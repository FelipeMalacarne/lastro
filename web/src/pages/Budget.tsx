import { useEffect, useMemo, useState, type ReactNode } from "react";
import { ArrowRight, Check, CircleAlert, Repeat, Sparkles, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { send, useResource, type BudgetCategory, type BudgetMonth, type BudgetSpend, type Category, type MonthSpend, type TransactionList } from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, formatDay, formatMonth, inputBRL, parseBRL, titleCase } from "@/lib/format";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";

type Filter = "all" | "attention" | "rollover";

/** Budget (canvas: "Budget", "Budget, phone"). Limits per category, pay yourself first and rollover (ADR-0006). */
export function Budget() {
  const { month, isCurrent } = useMonth();
  const res = useResource<BudgetMonth>(`/api/v1/budget?month=${month}`);
  const { byId, live } = useCategories();
  const [filter, setFilter] = useState<Filter>("all");
  const [selected, setSelected] = useState<string | null>(null);
  const wide = useWide();
  const v = res.data;
  // On a phone the detail sits under the table: choosing a row brings it into view.
  const select = (id: string) => {
    setSelected(id);
    if (!wide) requestAnimationFrame(() => document.getElementById("detail-title")?.scrollIntoView({ behavior: "smooth", block: "start" }));
  };

  // The first category that needs attention starts selected, so the detail is never empty.
  useEffect(() => {
    if (!v) return;
    const known = [...v.categories.map((c) => c.category), ...v.unlimited.map((u) => u.category ?? "")];
    if (!selected || !known.includes(selected)) setSelected(v.categories[0]?.category ?? v.unlimited.find((u) => u.category)?.category ?? null);
  }, [v, selected]);

  if (res.error && !v) return <PageHeader title="Budget" caption={`Could not load the budget: ${res.error}.`} />;
  if (!v) return <PageHeader title="Budget" caption="Loading…" />;

  const attention = v.categories.filter((c) => c.pace !== "on_pace");
  const rolls = v.categories.filter((c) => c.rollover);
  const shown = filter === "attention" ? attention : filter === "rollover" ? rolls : v.categories;
  const gone = Math.round(v.elapsedBp / 100);
  const caption = [
    isCurrent ? `Day ${v.days - v.daysLeft + 1} of ${v.days} · ${v.daysLeft} ${v.daysLeft === 1 ? "day" : "days"} left, today included` : `${formatMonth(v.month)} is over`,
    rolls.length ? `${rolls.length} ${rolls.length === 1 ? "category rolls" : "categories roll"} over` : null,
    v.hasBudget && !v.saved && v.from ? `limits as set in ${formatMonth(v.from)}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  if (!v.hasBudget) {
    return (
      <>
        <PageHeader title="Budget" caption={caption} />
        <StartBudget v={v} categories={live} />
      </>
    );
  }

  const sel = selected ? v.categories.find((c) => c.category === selected) : undefined;
  const selUnlimited = !sel && selected ? v.unlimited.find((u) => u.category === selected) : undefined;

  return (
    <>
      <PageHeader title="Budget" caption={caption} />
      <Hero v={v} isCurrent={isCurrent} gone={gone} />
      {!wide && attention.length > 0 && isCurrent && <Attention v={v} rows={attention} byId={byId} onSelect={select} />}

      <div className="flex flex-wrap items-start gap-4 lg:gap-6">
        <Panel
          className="min-w-0 flex-[2_1_560px]"
          bodyClassName="-mx-5"
          footer="Limits, spending and what is left per category. The tick on each bar is today: a fill to its left, or up to 10 points past it, is on pace. Recurring charges already paid, like rent, do not count against the pace."
        >
          <div className="flex flex-col gap-3 px-5 pb-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-col gap-0.5">
                <h2 className="m-0 type-heading">Categories</h2>
                <span className="type-caption text-ink-muted">What needs attention first, then by amount spent</span>
              </div>
              <SegmentedControl
                label="Show"
                value={filter}
                onChange={setFilter}
                options={[
                  { value: "all", label: `All ${v.categories.length}` },
                  { value: "attention", label: `Needs attention ${attention.length}` },
                  { value: "rollover", label: `Rolls over ${rolls.length}` },
                ]}
              />
            </div>
          </div>
          <CategoryTable
            v={v}
            rows={shown}
            byId={byId}
            selected={selected}
            onSelect={select}
            gone={v.elapsedBp / 100}
            total={filter === "all"}
          />
          {filter === "all" && v.unlimited.length > 0 && <Unlimited v={v} rows={v.unlimited} byId={byId} selected={selected} onSelect={select} />}
        </Panel>

        <aside aria-label="Details" className="flex min-w-0 flex-[1_1_340px] flex-col gap-4 lg:sticky lg:top-6 lg:max-w-[420px]">
          {wide && attention.length > 0 && isCurrent && <Attention v={v} rows={attention} byId={byId} onSelect={select} />}
          {sel && <Detail key={sel.category} v={v} c={sel} cat={byId.get(sel.category)} />}
          {selUnlimited && selUnlimited.category && (
            <Detail key={selUnlimited.category} v={v} spend={selUnlimited} cat={byId.get(selUnlimited.category)} />
          )}
        </aside>
      </div>
    </>
  );
}

/** Left to spend, how the month is going, and where the month's money goes. */
function Hero({ v, isCurrent, gone }: { v: BudgetMonth; isCurrent: boolean; gone: number }) {
  const left = v.leftCents - v.upcomingCents;
  const used = v.availableCents > 0 ? Math.round((v.spentCents / v.availableCents) * 100) : 0;
  const fixed = v.categories.reduce((s, c) => s + c.fixedCents, 0);
  const variableUsed = v.availableCents - fixed > 0 ? Math.round(((v.spentCents - fixed) / (v.availableCents - fixed)) * 100) : 0;
  const perDay = isCurrent && v.daysLeft > 0 && left > 0 ? left / v.daysLeft : 0;
  const pyf = v.payYourselfFirst;

  let pace: ReactNode;
  if (!isCurrent) pace = used > 100 ? `The limits were overrun by ${formatBRL(-v.leftCents)}.` : `${formatBRL(v.leftCents)} of the limits went unspent.`;
  else if (used <= gone + 10) pace = "On pace: spending is in step with the month.";
  else if (fixed > 0 && variableUsed <= gone + 10)
    pace = `Ahead of the calendar only because ${formatBRL(fixed)} in fixed charges were already paid. Without them you are on pace.`;
  else pace = `Ahead of pace: ${used}% of the limits are used with ${gone}% of the month gone.`;

  return (
    <Panel className="lg:p-6">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,440px)] lg:gap-10">
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-1">
            <span className="type-label text-ink-muted">{isCurrent ? "Left to spend" : "Left in the limits"}</span>
            <Amount cents={isCurrent ? left : v.leftCents} size="hero" sign="negative" tone={left < 0 ? "danger" : "ink"} />
            <span className="type-caption text-ink-muted">
              {perDay > 0 && `About ${formatBRL(perDay)} a day for the ${v.daysLeft} ${v.daysLeft === 1 ? "day" : "days"} left`}
              {isCurrent && v.upcomingCents > 0 && `${perDay > 0 ? ", after " : "After "}${formatBRL(v.upcomingCents)} in fixed charges still to come`}
              {!isCurrent && `${formatBRL(v.spentCents)} spent of ${formatBRL(v.availableCents)}`}
            </span>
          </div>
          <div className="flex flex-col gap-2">
            <div
              role="meter"
              aria-label={`${used}% of category limits used, ${gone}% of the month gone`}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.min(used, 100)}
              className="relative h-3 overflow-hidden rounded-xs bg-paper-sunken"
            >
              <div className={cn("h-full", used > 100 ? "bg-danger" : used > gone + 10 ? "bg-warning" : "bg-ink")} style={{ width: `${Math.min(used, 100)}%` }} />
              {isCurrent && <div className="absolute inset-y-[-2px] w-0.5 bg-tinta" style={{ left: `${gone}%` }} aria-hidden />}
            </div>
            <div className="flex flex-wrap justify-between gap-2 type-caption text-ink-muted">
              <span>
                <strong className="font-figure font-medium text-ink">{used}%</strong> of limits used
              </span>
              {isCurrent && (
                <span className="inline-flex items-center gap-1.5">
                  <span className="h-3 w-0.5 bg-tinta" aria-hidden />
                  <strong className="font-figure font-medium text-ink">{gone}%</strong> of the month gone
                </span>
              )}
            </div>
            <p className="m-0 type-caption text-ink-muted">{pace}</p>
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <span className="type-label text-ink-muted">{formatMonth(v.month).split(" ")[0]}'s money</span>
          <div role="table" aria-label={`Where ${formatMonth(v.month)}'s money goes`} className="flex flex-col">
            <MoneyRow label="Inflows" hint={v.incomeCount ? `${v.incomeCount} ${v.incomeCount === 1 ? "income" : "incomes"}` : "Nothing came in yet"}>
              <Amount cents={v.incomeCents} sign="always" />
            </MoneyRow>
            <MoneyRow
              label="Pay yourself first"
              hint={
                pyf.plannedCents === 0 ? (
                  <Link to="/settings#budget">Set an amount in Settings</Link>
                ) : pyf.doneOn ? (
                  <Badge tone="inflow" icon={Check}>
                    Done {formatDay(pyf.doneOn)}
                  </Badge>
                ) : pyf.movedCents > 0 ? (
                  `${formatBRL(pyf.movedCents)} moved so far`
                ) : (
                  `Due ${formatDay(pyf.due)}`
                )
              }
            >
              <Amount cents={-pyf.plannedCents} />
            </MoneyRow>
            <MoneyRow
              label="Category limits"
              hint={`${v.categories.length} ${v.categories.length === 1 ? "category" : "categories"}${v.carriedCents ? `, plus ${formatBRL(v.carriedCents)} rolled over` : ""}`}
            >
              <Amount cents={-v.limitsCents} />
            </MoneyRow>
            <div role="row" className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t-2 border-ink pt-2.5">
              <span role="cell" className="flex flex-col">
                <span className="text-[14px] leading-5 font-semibold">Not assigned</span>
                <span className={cn("type-caption", v.notAssignedCents < 0 ? "text-danger" : "text-ink-muted")}>
                  {v.notAssignedCents < 0 ? "The limits promise more than came in" : "No limit holds this money yet"}
                </span>
              </span>
              <span role="cell">
                <Amount cents={v.notAssignedCents} size="lg" tone={v.notAssignedCents < 0 ? "danger" : "ink"} />
              </span>
            </div>
          </div>
        </div>
      </div>
    </Panel>
  );
}

function MoneyRow({ label, hint, children }: { label: string; hint: ReactNode; children: ReactNode }) {
  return (
    <div role="row" className="grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-rule py-2">
      <span role="cell" className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
        <span className="text-[14px] leading-5">{label}</span>
        <span className="type-caption text-ink-muted">{hint}</span>
      </span>
      <span role="cell">{children}</span>
    </div>
  );
}

const cols = "grid-cols-[minmax(0,1fr)_auto] sm:grid-cols-[minmax(0,1.3fr)_minmax(80px,1fr)_96px_96px_104px]";

function status(c: BudgetCategory): { text: string; tone: string; icon?: typeof CircleAlert } {
  if (c.pace === "over") return { text: `Over by ${formatBRL(-c.leftCents)}`, tone: "text-danger font-semibold", icon: CircleAlert };
  if (c.pace === "ahead") return { text: "Ahead of pace", tone: "text-warning font-semibold", icon: TriangleAlert };
  if (c.fixedCents > 0 && c.fixedCents >= c.spentCents * 0.8) return { text: "Fixed charges paid", tone: "text-ink-muted" };
  if (c.spentCents === 0) return { text: "Nothing spent yet", tone: "text-ink-muted" };
  return { text: "On pace", tone: "text-ink-muted" };
}

function CategoryTable({
  v,
  rows,
  byId,
  selected,
  onSelect,
  gone,
  total,
}: {
  v: BudgetMonth;
  rows: BudgetCategory[];
  byId: Map<string, Category>;
  selected: string | null;
  onSelect: (id: string) => void;
  gone: number;
  total: boolean;
}) {
  return (
    <div role="table" aria-label={`Category limits for ${formatMonth(v.month)}`}>
      <div role="row" className={cn("grid h-8 items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted", cols)}>
        <span role="columnheader">Category</span>
        <span role="columnheader" className="hidden sm:block">
          Pace
        </span>
        <span role="columnheader" className="hidden text-right sm:block">
          Spent
        </span>
        <span role="columnheader" className="hidden text-right sm:block">
          Limit
        </span>
        <span role="columnheader" className="text-right">
          Left
        </span>
      </div>
      {rows.length === 0 && <p className="m-0 border-b border-rule px-5 py-4 type-body text-ink-muted">Nothing here this month.</p>}
      {rows.map((c) => {
        const cat = byId.get(c.category);
        const st = status(c);
        const on = c.category === selected;
        const fill = Math.min(c.availableCents > 0 ? (c.spentCents / c.availableCents) * 100 : c.spentCents > 0 ? 100 : 0, 100);
        return (
          <button
            key={c.category}
            type="button"
            role="row"
            aria-selected={on}
            onClick={() => onSelect(c.category)}
            className={cn(
              "grid min-h-16 w-full cursor-pointer items-center gap-x-4 border-0 border-b border-rule px-5 py-2.5 text-left focus-visible:focus-ring",
              cols,
              on ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken",
            )}
          >
            <span role="cell" className="flex min-w-0 flex-col gap-1">
              <span className="flex items-center gap-2">
                <CategoryTag name={cat?.name ?? "Archived"} color={cat?.color} />
                {c.rollover && <Repeat className="size-3.5 text-ink-muted" aria-label="rolls over" />}
              </span>
              <span className={cn("inline-flex items-center gap-1 type-caption", st.tone)}>
                {st.icon && <st.icon className="size-3.5" aria-hidden />}
                {st.text}
              </span>
              {/* On a phone the bar sits under the name. */}
              <PaceBar className="sm:hidden" fill={fill} over={c.pace === "over"} color={cat?.color} gone={gone} label={`${cat?.name}: ${formatBRL(c.spentCents)} of ${formatBRL(c.availableCents)}`} />
            </span>
            <span role="cell" className="hidden sm:block">
              <PaceBar fill={fill} over={c.pace === "over"} color={cat?.color} gone={gone} label={`${cat?.name}: ${formatBRL(c.spentCents)} of ${formatBRL(c.availableCents)}, ${Math.round(gone)}% of the month gone`} />
            </span>
            <span role="cell" className="hidden text-right sm:block">
              <Amount cents={c.spentCents} sign="none" />
            </span>
            <span role="cell" className="hidden flex-col items-end sm:flex">
              <Amount cents={c.availableCents} sign="none" tone="muted" />
              {c.carriedCents > 0 && <span className="type-caption text-ink-muted">+{formatBRL(c.carriedCents)} rolled</span>}
            </span>
            <span role="cell" className="flex flex-col items-end">
              <Amount cents={c.leftCents} tone={c.leftCents < 0 ? "danger" : "ink"} />
              <span className="type-caption text-ink-muted sm:hidden">of {formatBRL(c.availableCents)}</span>
            </span>
          </button>
        );
      })}
      {total && rows.length > 0 && (
        <div role="row" className={cn("grid min-h-11 items-center gap-x-4 border-b border-rule px-5 font-semibold", cols)}>
          <span role="cell" className="text-[14px]">
            Total
          </span>
          <span role="cell" className="hidden sm:block" />
          <span role="cell" className="hidden text-right sm:block">
            <Amount cents={v.spentCents} sign="none" />
          </span>
          <span role="cell" className="hidden text-right sm:block">
            <Amount cents={v.availableCents} sign="none" tone="muted" />
          </span>
          <span role="cell" className="flex flex-col items-end">
            <Amount cents={v.leftCents} tone={v.leftCents < 0 ? "danger" : "ink"} />
            <span className="type-caption font-normal text-ink-muted sm:hidden">of {formatBRL(v.availableCents)}</span>
          </span>
        </div>
      )}
    </div>
  );
}

function PaceBar({ fill, over, color, gone, label, className }: { fill: number; over: boolean; color?: string; gone: number; label: string; className?: string }) {
  return (
    <span role="img" aria-label={label} className={cn("relative block h-2 rounded-xs bg-paper-sunken", className)}>
      <span className="absolute inset-y-0 left-0 rounded-xs" style={{ width: `${fill}%`, background: over ? "var(--danger)" : `var(--${color ?? "note-200"})` }} />
      {gone > 0 && gone < 100 && <span className="absolute -inset-y-1 w-0.5 bg-ink" style={{ left: `${gone}%` }} aria-hidden />}
    </span>
  );
}

/** Spending in categories without a limit, and what still waits for a category. */
function Unlimited({ v, rows, byId, selected, onSelect }: { v: BudgetMonth; rows: BudgetSpend[]; byId: Map<string, Category>; selected: string | null; onSelect: (id: string) => void }) {
  const total = rows.reduce((s, r) => s + r.spentCents, 0);
  return (
    <div role="table" aria-label="Spending without a limit" className="mt-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 px-5 pb-2">
        <h3 className="m-0 text-[14px] leading-5 font-semibold">Without a limit</h3>
        <span className="type-caption text-ink-muted">{formatBRL(total)} this month, outside every limit</span>
      </div>
      <div role="row" className="grid h-8 grid-cols-[minmax(0,1fr)_96px_96px] items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted">
        <span role="columnheader">Category</span>
        <span role="columnheader" className="text-right">
          6-mo avg
        </span>
        <span role="columnheader" className="text-right">
          {formatMonth(v.month).split(" ")[0].slice(0, 3)}
        </span>
      </div>
      {rows.map((r) => {
        const cat = r.category ? byId.get(r.category) : undefined;
        const on = r.category !== null && r.category === selected;
        const inner = (
          <>
            <span role="cell" className="flex min-w-0 flex-col gap-0.5">
              {r.category ? (
                <CategoryTag name={cat?.name ?? "Archived"} color={cat?.color} />
              ) : (
                <span className="inline-flex items-center gap-1.5 text-[14px] leading-5 font-semibold text-warning">
                  <TriangleAlert className="size-3.5" aria-hidden /> To review
                </span>
              )}
              <span className="type-caption text-ink-muted">
                {r.category ? "Set a limit" : `${r.count} ${r.count === 1 ? "expense waits" : "expenses wait"} for a category`}
              </span>
            </span>
            <span role="cell" className="text-right">
              <Amount cents={r.averageCents} sign="none" tone="muted" />
            </span>
            <span role="cell" className="text-right">
              <Amount cents={r.spentCents} sign="none" />
            </span>
          </>
        );
        const row = "grid min-h-14 w-full grid-cols-[minmax(0,1fr)_96px_96px] items-center gap-x-4 border-0 border-b border-rule px-5 py-2 text-left";
        return r.category ? (
          <button
            key={r.category}
            type="button"
            role="row"
            aria-selected={on}
            onClick={() => onSelect(r.category!)}
            className={cn(row, "cursor-pointer focus-visible:focus-ring", on ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken")}
          >
            {inner}
          </button>
        ) : (
          <Link key="none" to="/review" role="row" className={cn(row, "bg-paper-raised text-ink no-underline hover:bg-paper-sunken hover:text-ink")}>
            {inner}
          </Link>
        );
      })}
    </div>
  );
}

function Attention({ v, rows, byId, onSelect }: { v: BudgetMonth; rows: BudgetCategory[]; byId: Map<string, Category>; onSelect: (id: string) => void }) {
  const spare = v.notAssignedCents;
  return (
    <section aria-labelledby="attention-title" className="flex flex-col gap-3 rounded-md border border-rule bg-paper-raised p-5">
      <h2 id="attention-title" className="m-0 type-heading">
        Needs attention · {rows.length}
      </h2>
      {rows.map((c) => {
        const name = byId.get(c.category)?.name ?? "A category";
        const over = c.pace === "over";
        const daily = v.daysLeft > 0 ? Math.max(c.leftCents, 0) / v.daysLeft : 0;
        return (
          <div key={c.category} className="flex flex-col gap-2 border-t border-rule pt-3 first-of-type:border-0 first-of-type:pt-0">
            <span className="flex gap-2 text-[14px] leading-5">
              {over ? <CircleAlert className="mt-0.5 size-4 flex-none text-danger" aria-hidden /> : <TriangleAlert className="mt-0.5 size-4 flex-none text-warning" aria-hidden />}
              <span>
                {over ? (
                  <>
                    <strong>
                      {name} is over by {formatBRL(-c.leftCents)}.
                    </strong>{" "}
                    {spare >= -c.leftCents ? `Not assigned can cover it and still keep ${formatBRL(spare + c.leftCents)}.` : "Not assigned cannot cover it."}
                  </>
                ) : (
                  <>
                    <strong>
                      {name} has {formatBRL(c.leftCents)} left
                    </strong>{" "}
                    for {v.daysLeft} {v.daysLeft === 1 ? "day" : "days"}, about {formatBRL(daily)} a day.
                  </>
                )}
              </span>
            </span>
            <div className="flex flex-wrap gap-2 pl-6">
              {over && spare >= -c.leftCents && (
                <CoverButton month={v.month} c={c} />
              )}
              <Button variant="ghost" size="sm" onClick={() => onSelect(c.category)}>
                Details <ArrowRight className="size-3.5" aria-hidden />
              </Button>
            </div>
          </div>
        );
      })}
    </section>
  );
}

/** Raises an overspent limit by what it is over, this month on, taking it from Not assigned. */
function CoverButton({ month, c }: { month: string; c: BudgetCategory }) {
  const [busy, setBusy] = useState(false);
  return (
    <Button
      variant="primary"
      size="sm"
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        try {
          await send("PUT", `/api/v1/budget/${month}/limits/${c.category}`, { limitCents: c.limitCents - c.leftCents, rollover: c.rollover });
        } finally {
          setBusy(false);
        }
      }}
    >
      Cover with Not assigned
    </Button>
  );
}

/** One category: where its limit comes from, its last six months, where the money went, and its limit. */
function Detail({ v, c, spend, cat }: { v: BudgetMonth; c?: BudgetCategory; spend?: BudgetSpend; cat?: Category }) {
  const id = c?.category ?? spend?.category ?? "";
  const history = c?.history ?? spend?.history ?? [];
  const average = c?.averageCents ?? spend?.averageCents ?? 0;
  const spent = c?.spentCents ?? spend?.spentCents ?? 0;
  const count = c?.count ?? spend?.count ?? 0;
  const list = useResource<TransactionList>(`/api/v1/transactions?month=${v.month}&category=${id}&flow=out`);
  const merchants = useMemo(() => byMerchant(list.data?.transactions.map((t) => ({ d: t.description, c: -t.amountCents })) ?? []), [list.data]);

  return (
    <section aria-labelledby="detail-title" className="flex flex-col gap-4 rounded-md border border-rule bg-paper-raised p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="detail-title" className="m-0 flex scroll-mt-20 items-center gap-2 type-heading">
          <span className="size-3 rounded-xs" style={{ background: `var(--${cat?.color ?? "note-200"})` }} aria-hidden />
          {cat?.name ?? "Archived category"}
        </h2>
        {c?.pace === "over" && (
          <Badge tone="danger" icon={CircleAlert}>
            Over
          </Badge>
        )}
        {c?.pace === "ahead" && (
          <Badge tone="warning" icon={TriangleAlert}>
            Ahead of pace
          </Badge>
        )}
        {!c && <Badge>No limit</Badge>}
      </div>

      {c ? (
        <>
          <div className="flex flex-col gap-0.5">
            <span className="flex flex-wrap items-baseline gap-2">
              <Amount cents={c.leftCents} size="lg" tone={c.leftCents < 0 ? "danger" : "ink"} />
              <span className="type-caption text-ink-muted">left of {formatBRL(c.availableCents)}</span>
            </span>
            <span className="type-caption text-ink-muted">
              {formatBRL(spent)} spent on {count} {count === 1 ? "transaction" : "transactions"}
              {c.fixedCents > 0 && `, ${formatBRL(c.fixedCents)} of it in fixed charges`}
              {c.upcomingCents > 0 && `; ${formatBRL(c.upcomingCents)} more still to come`}
            </span>
          </div>
          <div role="table" aria-label="Where the limit comes from" className="flex flex-col type-caption">
            <div role="row" className="flex justify-between border-t border-rule py-1.5">
              <span role="cell" className="text-ink-muted">
                Monthly limit
              </span>
              <Amount cents={c.limitCents} sign="none" size="sm" />
            </div>
            <div role="row" className="flex justify-between border-t border-rule py-1.5">
              <span role="cell" className="text-ink-muted">
                Rolled over from {formatMonth(addMonth(v.month, -1)).split(" ")[0]}
              </span>
              <Amount cents={c.carriedCents} sign="always" tone="ink" size="sm" />
            </div>
            <div role="row" className="flex justify-between border-t border-ink py-1.5 font-semibold">
              <span role="cell">Available in {formatMonth(v.month).split(" ")[0]}</span>
              <Amount cents={c.availableCents} sign="none" size="sm" />
            </div>
          </div>
        </>
      ) : (
        <span className="type-caption text-ink-muted">
          {formatBRL(spent)} spent this month on {count} {count === 1 ? "transaction" : "transactions"}, with no limit holding it.
        </span>
      )}

      <History history={history} now={{ month: v.month, spentCents: spent }} average={average} limit={c?.limitCents} name={cat?.name ?? "This category"} color={cat?.color} />

      {merchants.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="type-label text-ink-muted">Where it went</span>
          {merchants.map((m) => (
            <div key={m.name} className="flex items-center justify-between gap-3 text-[14px] leading-5">
              <span className="min-w-0 truncate">
                {titleCase(m.name)} <span className="type-figure-sm text-ink-muted">{m.count}×</span>
              </span>
              <Amount cents={m.cents} sign="none" size="sm" />
            </div>
          ))}
        </div>
      )}

      <LimitEditor key={id + (c?.limitCents ?? 0)} month={v.month} category={id} name={cat?.name ?? ""} limit={c} suggested={average} />

      <Link to={`/transactions?month=${v.month}&category=${id}`} className="self-start text-[14px] font-semibold">
        See the {count} {count === 1 ? "transaction" : "transactions"}
      </Link>
    </section>
  );
}

function addMonth(m: string, n: number): string {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(y, mo - 1 + n, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

// Words a bank adds around the merchant's name.
const noise = new Set(["PAG", "PG", "PIX", "ENVIADO", "RECEBIDO", "COMPRA", "DEBITO", "CREDITO", "ESTORNO", "PENDING"]);

/** "PAG*IFOOD SP", "IFOOD *IFOOD" and "PG *IFOOD" are all "IFOOD". */
export function merchantName(description: string): string {
  const words = description
    .toUpperCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .split(/[^A-Z0-9]+/)
    .filter((w) => w.length > 2 && !/^\d+$/.test(w) && !noise.has(w));
  const name = [...new Set(words)].slice(0, 2).join(" ");
  return name || description.trim().toUpperCase();
}

/** The descriptions behind a month's spending, biggest first; past four, the rest as "Others". */
function byMerchant(txs: { d: string; c: number }[]): { name: string; count: number; cents: number }[] {
  const m = new Map<string, { name: string; count: number; cents: number }>();
  for (const t of txs) {
    const key = merchantName(t.d);
    const e = m.get(key) ?? { name: key, count: 0, cents: 0 };
    e.count++;
    e.cents += t.c;
    m.set(key, e);
  }
  const all = [...m.values()].sort((a, b) => b.cents - a.cents);
  if (all.length <= 4) return all;
  const rest = all.slice(3);
  return [...all.slice(0, 3), { name: "Others", count: rest.reduce((s, r) => s + r.count, 0), cents: rest.reduce((s, r) => s + r.cents, 0) }];
}

/** The last six months and this one as bars, with the average and the limit. */
function History({ history, now, average, limit, name, color }: { history: MonthSpend[]; now: MonthSpend; average: number; limit?: number; name: string; color?: string }) {
  const all = [...history, now];
  const max = Math.max(1, ...all.map((h) => h.spentCents), limit ?? 0, average);
  const label = `${name} spending: ${all.map((h) => `${formatMonth(h.month, true)} ${formatBRL(h.spentCents)}`).join(", ")}.${limit !== undefined ? ` Monthly limit ${formatBRL(limit)}.` : ""}`;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between gap-2">
        <span className="type-label text-ink-muted">Last 6 months</span>
        <span className="type-caption text-ink-muted">
          Average <strong className="font-figure font-medium text-ink">{formatBRL(average)}</strong>
        </span>
      </div>
      <div role="img" aria-label={label} className="relative grid h-[88px] items-end gap-1.5" style={{ gridTemplateColumns: `repeat(${all.length}, minmax(0, 1fr))` }}>
        {all.map((h, i) => (
          <span
            key={h.month}
            className={cn("rounded-t-xs", i === all.length - 1 ? (color ? "" : "bg-tinta") : "bg-rule-strong")}
            style={{ background: i === all.length - 1 && color ? `var(--${color})` : undefined, height: `${Math.max((h.spentCents / max) * 100, h.spentCents > 0 ? 2 : 0)}%` }}
          />
        ))}
        {limit !== undefined && limit > 0 && (
          <span className="absolute inset-x-0 border-t-[1.5px] border-dashed border-ink" style={{ bottom: `${(limit / max) * 100}%` }} aria-hidden />
        )}
      </div>
      <div className="grid gap-1.5 type-figure-sm text-ink-muted" style={{ gridTemplateColumns: `repeat(${all.length}, minmax(0, 1fr))` }}>
        {all.map((h) => (
          <span key={h.month} className="text-center">
            {formatMonth(h.month, true).slice(0, 3)}
          </span>
        ))}
      </div>
      {limit !== undefined && (
        <span className="inline-flex items-center gap-1.5 type-caption text-ink-muted">
          <span className="w-4 border-t-[1.5px] border-dashed border-ink" aria-hidden />
          Monthly limit, {formatBRL(limit)}
        </span>
      )}
    </div>
  );
}

function LimitEditor({ month, category, name, limit, suggested }: { month: string; category: string; name: string; limit?: BudgetCategory; suggested: number }) {
  const start = limit?.limitCents ?? Math.ceil(suggested / 5000) * 5000;
  const [text, setText] = useState(start ? inputBRL(start) : "");
  const [rollover, setRollover] = useState(limit?.rollover ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cents = parseBRL(text);
  const changed = !limit || cents !== limit.limitCents || rollover !== limit.rollover;
  const monthName = formatMonth(month).split(" ")[0];

  async function run(f: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await f();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="flex flex-col gap-3 border-t border-rule pt-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (cents === null) return;
        void run(() => send("PUT", `/api/v1/budget/${month}/limits/${category}`, { limitCents: cents, rollover }));
      }}
    >
      <TextField
        label="Monthly limit"
        prefix="R$"
        numeric
        inputMode="decimal"
        value={text}
        onChange={(e) => setText(e.target.value)}
        error={text && cents === null ? "Type an amount, like 500,00" : undefined}
        hint={limit ? `From ${monthName} on; earlier months keep theirs.` : suggested ? `Suggested from the 6-month average of ${name}.` : `From ${monthName} on.`}
      />
      <label className="flex cursor-pointer items-start gap-2.5">
        <input type="checkbox" checked={rollover} onChange={(e) => setRollover(e.target.checked)} className="mt-0.5 size-4 accent-[var(--tinta)]" />
        <span className="flex flex-col">
          <span className="text-[14px] leading-5 font-semibold">Roll over what is left</span>
          <span className="type-caption text-ink-muted">Whatever remains at the end of a month is added to the next one's limit.</span>
        </span>
      </label>
      {error && (
        <p role="alert" className="m-0 type-caption text-danger">
          Could not save: {error}.
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" variant={limit ? "secondary" : "primary"} size="sm" disabled={busy || cents === null || !changed}>
          {limit ? "Save limit" : "Set limit"}
        </Button>
        {limit && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void run(() => send("DELETE", `/api/v1/budget/${month}/limits/${category}`))}>
            Remove limit
          </Button>
        )}
      </div>
    </form>
  );
}

/** No budget saved yet: one click turns the 6-month averages into limits. */
function StartBudget({ v, categories }: { v: BudgetMonth; categories: Category[] }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const budgeted = new Map(categories.filter((c) => c.budgeted).map((c) => [c.id, c]));
  const proposal = v.unlimited
    .filter((u) => u.category && budgeted.has(u.category) && u.averageCents > 0)
    .map((u) => ({ id: u.category!, name: budgeted.get(u.category!)!.name, color: budgeted.get(u.category!)!.color, cents: Math.ceil(u.averageCents / 5000) * 5000 }));
  const total = proposal.reduce((s, p) => s + p.cents, 0);

  async function start() {
    setBusy(true);
    setError(null);
    try {
      for (const p of proposal) await send("PUT", `/api/v1/budget/${v.month}/limits/${p.id}`, { limitCents: p.cents, rollover: false });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel title="No budget yet">
      <div className="flex flex-col gap-4">
        <p className="m-0 max-w-[62ch] type-body text-ink-muted">
          A budget is a limit per category, and pay yourself first set aside before any of them. Each month follows the one before it until you change it, so this
          is done once.
        </p>
        {proposal.length > 0 ? (
          <>
            <div className="flex flex-col">
              {proposal.map((p) => (
                <div key={p.id} className="flex items-center justify-between gap-3 border-t border-rule py-2">
                  <CategoryTag name={p.name} color={p.color} />
                  <Amount cents={p.cents} sign="none" />
                </div>
              ))}
              <div className="flex items-center justify-between gap-3 border-t-2 border-ink py-2 font-semibold">
                <span className="text-[14px]">Limits, a month</span>
                <Amount cents={total} sign="none" />
              </div>
            </div>
            {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
            <div className="flex flex-wrap items-center gap-3">
              <Button variant="primary" onClick={() => void start()} disabled={busy}>
                <Sparkles className="size-4" aria-hidden /> Start from my 6-month averages
              </Button>
              <span className="type-caption text-ink-muted">Rounded up to R$ 50. Change any of them afterwards.</span>
            </div>
          </>
        ) : (
          <p className="m-0 type-body">
            There is no categorized spending to start from yet. <Link to="/review">Review the queue</Link> first, then come back.
          </p>
        )}
      </div>
    </Panel>
  );
}

