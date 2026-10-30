import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowRight, Check, CircleAlert, PiggyBank, Repeat, Sparkles, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { FundDetail } from "@/components/FundDetail";
import { LineEditor } from "@/components/LineEditor";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { Skeleton, SkeletonRows, SkeletonStat } from "@/components/ui/skeleton";
import { Checkbox } from "@/components/ui/checkbox";
import { Sheet, SheetActions } from "@/components/ui/sheet";
import { send, useResource, type BudgetLine, type BudgetMonth, type BudgetSpend, type Category, type MonthSpend, type NoteColor, type TransactionList } from "@/lib/api";
import { isShared, lineBody, lineColors, lineName, monthName, restore, saveLine, snapshot, type LineBody } from "@/lib/budget";
import { useCategories } from "@/lib/categories";
import { addMonths, formatBRL, formatDay, formatMonth, inputBRL, parseBRL, titleCase } from "@/lib/format";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { Link } from "@/lib/router";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";

type Filter = "all" | "attention" | "shared" | "rollover";

/** What is open beside the list, or in the sheet on a phone: a line, a category without one, or a line's editor (a new line when null). */
type Open = { line: string } | { category: string } | { edit: string | null };

/**
 * Budget (canvas: "Budget", "Budget, phone", "Budget: shared lines and funds",
 * "Budget, phone: a fund in a sheet"). Lines of one or more categories, each a
 * limit or a fund (ADR-0011, ADR-0012), pay yourself first and rollover
 * (ADR-0006).
 */
export function Budget() {
  const { month, isCurrent } = useMonth();
  const res = useResource<BudgetMonth>(`/api/v1/budget?month=${month}`);
  const { byId, live } = useCategories();
  const [filter, setFilter] = useState<Filter>("all");
  const [open, setOpen] = useState<Open | null>(null);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const wide = useWide();
  const v = res.data;
  // On a phone what is open sits in a sheet, which keeps it while it slides away.
  const inSheet = useRef<Open | null>(null);
  if (open) inSheet.current = open;
  const startEditing = () => {
    setEditing(true);
    requestAnimationFrame(() => document.getElementById("limits-editor")?.scrollIntoView({ behavior: "smooth", block: "start" }));
  };

  // On a wide screen the first line that needs attention starts open, so the detail is never empty.
  useEffect(() => {
    if (!v) return;
    const first: Open | null = v.lines[0] ? { line: v.lines[0].id } : v.unlimited.find((u) => u.category) ? { category: v.unlimited.find((u) => u.category)!.category! } : null;
    const gone =
      open &&
      (("line" in open && !v.lines.some((l) => l.id === open.line)) ||
        ("category" in open && !v.unlimited.some((u) => u.category === open.category)) ||
        ("edit" in open && open.edit !== null && !v.lines.some((l) => l.id === open.edit)));
    if (gone) setOpen(wide ? first : null);
    else if (!open && wide) setOpen(first);
  }, [v, open, wide]);

  useEffect(() => setEditing(false), [month]);

  if (res.error && !v) return <PageHeader title="Budget" caption={`Could not load the budget: ${res.error}.`} />;
  if (!v)
    return (
      <>
        <PageHeader title="Budget" />
        <Panel className="lg:p-6">
          <div className="flex flex-col gap-4">
            <SkeletonStat hero />
            <Skeleton className="h-3.5" />
          </div>
        </Panel>
        <Panel title="Limits" bodyClassName="-mx-5">
          <SkeletonRows count={6} className="border-t border-rule" />
        </Panel>
      </>
    );

  const limits = v.lines.filter((l) => !l.fund);
  const funds = v.lines.filter((l) => l.fund);
  const attention = limits.filter((l) => l.pace !== "on_pace");
  const shared = limits.filter(isShared);
  const rolls = limits.filter((l) => l.rollover);
  const shown = filter === "attention" ? attention : filter === "shared" ? shared : filter === "rollover" ? rolls : limits;
  const gone = Math.round(v.elapsedBp / 100);
  const caption = [
    isCurrent ? `Day ${v.days - v.daysLeft + 1} of ${v.days} · ${v.daysLeft} ${v.daysLeft === 1 ? "day" : "days"} left, today included` : `${formatMonth(v.month)} is over`,
    rolls.length ? `${rolls.length} ${rolls.length === 1 ? "line rolls" : "lines roll"} over` : null,
    funds.length ? `${funds.length} ${funds.length === 1 ? "fund" : "funds"}` : null,
    v.hasBudget && !v.saved && v.from ? `lines as set in ${formatMonth(v.from)}` : null,
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

  const shownOpen = wide ? open : inSheet.current;
  const prev = addMonths(v.month, -1);
  const prevName = monthName(prev);

  async function copy() {
    if (!v) return;
    const before = snapshot(v);
    const into = v.month;
    setBusy(true);
    try {
      const r = await send<{ copied: number }>("POST", `/api/v1/budget/${into}/copy`, { from: prev });
      toast({
        id: "budget",
        text: `Copied ${prevName}'s ${r.copied} ${r.copied === 1 ? "line" : "lines"} into ${monthName(into)}.`,
        undo: () => restore(into, before),
      });
    } catch (e) {
      toast({ id: "budget", tone: "error", text: `Could not copy: ${(e as Error).message}.` });
    } finally {
      setBusy(false);
    }
  }

  const actions = editing ? undefined : (
    <>
      {v.prevDiffers && (
        <Button variant="ghost" onClick={() => void copy()} disabled={busy}>
          Copy {prevName}
        </Button>
      )}
      <Button variant="ghost" onClick={() => setOpen({ edit: null })}>
        Add a line
      </Button>
      <Button onClick={startEditing}>Edit all limits</Button>
    </>
  );

  let detail: ReactNode = null;
  let sheetLabel = "";
  if (shownOpen && "line" in shownOpen) {
    const l = v.lines.find((x) => x.id === shownOpen.line);
    if (l) {
      const edit = () => setOpen({ edit: l.id });
      detail = l.fund ? (
        <FundDetail key={l.id} v={v} l={l} byId={byId} inSheet={!wide} onEdit={edit} />
      ) : (
        <LineDetail key={l.id} v={v} l={l} byId={byId} inSheet={!wide} onEdit={edit} />
      );
      sheetLabel = `${lineName(l, byId)} · ${monthName(v.month)}`;
    }
  } else if (shownOpen && "category" in shownOpen) {
    const s = v.unlimited.find((u) => u.category === shownOpen.category);
    if (s) {
      detail = <LineDetail key={shownOpen.category} v={v} spend={s} byId={byId} inSheet={!wide} />;
      sheetLabel = `${byId.get(shownOpen.category)?.name ?? "Archived category"} · ${monthName(v.month)}`;
    }
  } else if (shownOpen && "edit" in shownOpen) {
    const l = shownOpen.edit ? v.lines.find((x) => x.id === shownOpen.edit) : undefined;
    detail = (
      <LineEditor
        key={shownOpen.edit ?? "new"}
        v={v}
        line={l}
        categories={live}
        byId={byId}
        inSheet={!wide}
        onDone={() => setOpen(l && v.lines.some((x) => x.id === l.id) ? { line: l.id } : null)}
      />
    );
    sheetLabel = l ? `Edit ${lineName(l, byId)}` : "A new line";
  }

  const isOpen = (o: Open) => JSON.stringify(o) === JSON.stringify(open);
  const selectLine = (id: string) => setOpen({ line: id });
  const selectCategory = (id: string) => setOpen({ category: id });

  return (
    <>
      <PageHeader title="Budget" caption={caption} actions={actions} />
      {wide ? <Hero v={v} limits={limits} funds={funds} byId={byId} isCurrent={isCurrent} gone={gone} onAssign={editing ? undefined : startEditing} /> : <PhoneHero v={v} limits={limits} isCurrent={isCurrent} byId={byId} />}
      {!wide && <PayFirst v={v} />}

      {editing ? (
        <LimitsEditor v={v} categories={live} byId={byId} onDone={() => setEditing(false)} />
      ) : !wide ? (
        <>
          {attention.length > 0 && <PhoneGroup title="Needs attention" v={v} rows={attention} byId={byId} onSelect={selectLine} isOpen={isOpen} cover />}
          {limits.length > attention.length && (
            <PhoneGroup title={isCurrent ? "On pace" : "Within the limit"} v={v} rows={limits.filter((l) => l.pace === "on_pace")} byId={byId} onSelect={selectLine} isOpen={isOpen} />
          )}
          {funds.length > 0 && <PhoneFunds v={v} rows={funds} byId={byId} onSelect={selectLine} isOpen={isOpen} />}
          {v.unlimited.length > 0 && (
            <section className="overflow-hidden rounded-md border border-rule bg-paper-raised pt-4">
              <Unlimited v={v} rows={v.unlimited} byId={byId} isOpen={isOpen} onSelect={selectCategory} className="-mb-px" />
            </section>
          )}
          {inSheet.current && (
            <Sheet open={Boolean(open)} onOpenChange={(o) => !o && setOpen(null)} label={sheetLabel}>
              {detail}
            </Sheet>
          )}
        </>
      ) : (
        <div className="flex flex-wrap items-start gap-4 lg:gap-6">
          <div className="flex min-w-0 flex-[2_1_560px] flex-col gap-4 lg:gap-6">
            <Panel
              bodyClassName="-mx-5"
              footer="Limits, spending and what is left per line. A shared line has one limit for its categories together; its bar shows each one's share in its colour. The tick on each bar is today: a fill to its left, or up to 10 points past it, is on pace. Recurring charges already paid, like rent, do not count against the pace."
            >
              <div className="flex flex-col gap-3 px-5 pb-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex flex-col gap-0.5">
                    <h2 className="m-0 type-heading">Limits</h2>
                    <span className="type-caption text-ink-muted">What needs attention first, then by amount spent</span>
                  </div>
                  <SegmentedControl
                    label="Show"
                    value={filter}
                    onChange={setFilter}
                    options={[
                      { value: "all", label: `All ${limits.length}` },
                      { value: "attention", label: `Needs attention ${attention.length}` },
                      ...(shared.length ? [{ value: "shared" as const, label: `Shared ${shared.length}` }] : []),
                      ...(rolls.length ? [{ value: "rollover" as const, label: `Rolls over ${rolls.length}` }] : []),
                    ]}
                  />
                </div>
              </div>
              <LineTable v={v} rows={shown} byId={byId} isOpen={isOpen} onSelect={selectLine} gone={v.elapsedBp / 100} total={filter === "all"} />
              {filter === "all" && v.unlimited.length > 0 && <Unlimited v={v} rows={v.unlimited} byId={byId} isOpen={isOpen} onSelect={selectCategory} />}
            </Panel>
            {funds.length > 0 && <FundsPanel v={v} rows={funds} byId={byId} isOpen={isOpen} onSelect={selectLine} />}
          </div>

          <aside aria-label="Details" className="flex min-w-0 flex-[1_1_340px] flex-col gap-4 lg:sticky lg:top-6 lg:max-w-[420px]">
            {attention.length > 0 && isCurrent && !(open && "edit" in open) && <Attention v={v} rows={attention} byId={byId} onSelect={selectLine} />}
            {detail}
          </aside>
        </div>
      )}
    </>
  );
}

/** Left to spend, how the month is going, and where the month's money goes. */
function Hero({
  v,
  limits,
  funds,
  byId,
  isCurrent,
  gone,
  onAssign,
}: {
  v: BudgetMonth;
  limits: BudgetLine[];
  funds: BudgetLine[];
  byId: Map<string, Category>;
  isCurrent: boolean;
  gone: number;
  onAssign?: () => void;
}) {
  const left = v.leftCents - v.upcomingCents;
  const used = v.availableCents > 0 ? Math.round((v.spentCents / v.availableCents) * 100) : 0;
  const fixed = limits.reduce((s, c) => s + c.fixedCents, 0);
  const variableUsed = v.availableCents - fixed > 0 ? Math.round(((v.spentCents - fixed) / (v.availableCents - fixed)) * 100) : 0;
  const perDay = isCurrent && v.daysLeft > 0 && left > 0 ? left / v.daysLeft : 0;
  const pyf = v.payYourselfFirst;
  const sharedCount = limits.filter(isShared).length;

  let pace: ReactNode;
  if (!isCurrent) pace = used > 100 ? `The limits were overrun by ${formatBRL(-v.leftCents)}.` : `${formatBRL(v.leftCents)} of the limits went unspent.`;
  else if (used <= gone + 10) pace = "On pace: spending is in step with the month.";
  else if (fixed > 0 && variableUsed <= gone + 10)
    pace = `Ahead of the calendar only because ${formatBRL(fixed)} in fixed charges were already paid. Without them you are on pace.`;
  else pace = `Ahead of pace: ${used}% of the limits are used with ${gone}% of the month gone.`;
  if (funds.length) pace = <>{pace} The funds are apart.</>;

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
              aria-label={`${used}% of the limits used, ${gone}% of the month gone`}
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
                <strong className="font-figure font-medium text-ink">{used}%</strong> of the limits used
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
          <span className="type-label text-ink-muted">{monthName(v.month)}'s money</span>
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
              label="Limits"
              hint={`${limits.length} ${limits.length === 1 ? "line" : "lines"}${sharedCount ? `, ${sharedCount} shared` : ""}${v.carriedCents ? `, plus ${formatBRL(v.carriedCents)} rolled over` : ""}`}
            >
              <Amount cents={-(v.limitsCents - v.funds.amountCents)} />
            </MoneyRow>
            {funds.length > 0 && (
              <MoneyRow label="Into the funds" hint={`${funds.map((f) => lineName(f, byId)).join(" and ")}, kept invested`}>
                <Amount cents={-v.funds.amountCents} />
              </MoneyRow>
            )}
            <div role="row" className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 border-t-2 border-ink pt-2.5">
              <span role="cell" className="flex flex-col">
                <span className="text-[14px] leading-5 font-semibold">Not assigned</span>
                <span className={cn("type-caption", v.notAssignedCents < 0 ? "text-danger" : "text-ink-muted")}>
                  {v.notAssignedCents < 0 ? "The lines promise more than came in" : "No line holds this money yet"}
                </span>
              </span>
              <span role="cell">
                {onAssign && (
                  <Button variant="ghost" size="sm" onClick={onAssign}>
                    {v.notAssignedCents < 0 ? "Adjust" : "Assign"}
                  </Button>
                )}
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

function status(c: BudgetLine): { text: string; tone: string; icon?: typeof CircleAlert } {
  if (c.pace === "over") return { text: `Over by ${formatBRL(-c.leftCents)}`, tone: "text-danger font-semibold", icon: CircleAlert };
  if (c.pace === "ahead") return { text: "Ahead of pace", tone: "text-warning font-semibold", icon: TriangleAlert };
  if (c.fixedCents > 0 && c.fixedCents >= c.spentCents * 0.8) return { text: "Fixed charges paid", tone: "text-ink-muted" };
  if (c.spentCents === 0) return { text: "Nothing spent yet", tone: "text-ink-muted" };
  return { text: "On pace", tone: "text-ink-muted" };
}

/** A line's name: a shared line's swatches and name, or its one category's tag. */
function LineName({ l, byId }: { l: BudgetLine; byId: Map<string, Category> }) {
  if (!isShared(l)) {
    const cat = byId.get(l.byCategory[0]?.category ?? "");
    return <CategoryTag name={cat?.name ?? "Archived"} color={cat?.color} />;
  }
  return (
    <span className="inline-flex min-w-0 items-center gap-2 text-[14px] leading-5 font-semibold">
      <span className="inline-flex flex-none gap-0.5" aria-hidden>
        {lineColors(l, byId).map((c, i) => (
          <span key={i} className="h-3 w-1.5 rounded-[1px]" style={{ background: `var(--${c})` }} />
        ))}
      </span>
      <span className="truncate">{lineName(l, byId)}</span>
    </span>
  );
}

/** A shared line's categories, as the caption under its name. */
const members = (l: BudgetLine, byId: Map<string, Category>) => l.byCategory.map((p) => byId.get(p.category)?.name ?? "Archived").join(", ");

function LineTable({
  v,
  rows,
  byId,
  isOpen,
  onSelect,
  gone,
  total,
}: {
  v: BudgetMonth;
  rows: BudgetLine[];
  byId: Map<string, Category>;
  isOpen: (o: Open) => boolean;
  onSelect: (id: string) => void;
  gone: number;
  total: boolean;
}) {
  return (
    <div role="table" aria-label={`Limits for ${formatMonth(v.month)}`}>
      <div role="row" className={cn("grid h-8 items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted", cols)}>
        <span role="columnheader">Line</span>
        <span role="columnheader" className="hidden sm:block">
          Pace
        </span>
        <span role="columnheader" className="hidden text-right sm:block">
          Spent
        </span>
        <span role="columnheader" className="hidden text-right sm:block">
          Available
        </span>
        <span role="columnheader" className="text-right">
          Left
        </span>
      </div>
      {rows.length === 0 && <p className="m-0 border-b border-rule px-5 py-4 type-body text-ink-muted">Nothing here this month.</p>}
      {rows.map((c) => {
        const name = lineName(c, byId);
        const st = status(c);
        const on = isOpen({ line: c.id });
        const fill = Math.min(c.availableCents > 0 ? (c.spentCents / c.availableCents) * 100 : c.spentCents > 0 ? 100 : 0, 100);
        const parts = paceParts(c, byId);
        return (
          <button
            key={c.id}
            type="button"
            role="row"
            aria-selected={on}
            onClick={() => onSelect(c.id)}
            className={cn(
              "grid min-h-16 w-full cursor-pointer items-center gap-x-4 border-0 border-b border-rule px-5 py-2.5 text-left focus-visible:focus-ring",
              cols,
              on ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken",
            )}
          >
            <span role="cell" className="flex min-w-0 flex-col gap-1">
              <span className="flex min-w-0 items-center gap-2">
                <LineName l={c} byId={byId} />
                {c.rollover && <Repeat className="size-3.5 flex-none text-ink-muted" aria-label="rolls over" />}
              </span>
              {isShared(c) && <span className="truncate type-caption text-ink-muted">{members(c, byId)}</span>}
              <span className={cn("inline-flex items-center gap-1 type-caption", st.tone)}>
                {st.icon && <st.icon className="size-3.5" aria-hidden />}
                {st.text}
              </span>
              {/* On a phone the bar sits under the name. */}
              <PaceBar className="sm:hidden" fill={fill} over={c.pace === "over"} parts={parts} gone={gone} label={`${name}: ${formatBRL(c.spentCents)} of ${formatBRL(c.availableCents)}`} />
            </span>
            <span role="cell" className="hidden sm:block">
              <PaceBar
                fill={fill}
                over={c.pace === "over"}
                parts={parts}
                gone={gone}
                label={`${name}: ${formatBRL(c.spentCents)} of ${formatBRL(c.availableCents)}${isShared(c) ? `, ${c.byCategory.map((p) => `${byId.get(p.category)?.name ?? "Archived"} ${formatBRL(p.spentCents)}`).join(", ")}` : ""}, ${Math.round(gone)}% of the month gone`}
              />
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

/** A line's spending by category, each in its colour, for its bar. */
function paceParts(l: BudgetLine, byId: Map<string, Category>): { color: NoteColor; cents: number }[] {
  return l.byCategory.filter((p) => p.spentCents > 0).map((p) => ({ color: byId.get(p.category)?.color ?? "note-200", cents: p.spentCents }));
}

function PaceBar({
  fill,
  over,
  parts,
  gone,
  label,
  className,
}: {
  fill: number;
  over: boolean;
  parts: { color: NoteColor; cents: number }[];
  gone: number;
  label: string;
  className?: string;
}) {
  return (
    <span role="img" aria-label={label} className={cn("relative block h-2 rounded-xs bg-paper-sunken", className)}>
      <span className="absolute inset-y-0 left-0 flex gap-px overflow-hidden rounded-xs" style={{ width: `${fill}%` }}>
        {over || parts.length === 0 ? (
          <span className="flex-1" style={{ background: over ? "var(--danger)" : `var(--${parts[0]?.color ?? "note-200"})` }} />
        ) : (
          parts.map((p, i) => <span key={i} className="min-w-px" style={{ flex: `${p.cents} 1 0`, background: `var(--${p.color})` }} />)
        )}
      </span>
      {gone > 0 && gone < 100 && <span className="absolute -inset-y-1 w-0.5 bg-ink" style={{ left: `${gone}%` }} aria-hidden />}
    </span>
  );
}

/** The funds (ADR-0011): what each brought, put in this month, paid, and its balance. */
function FundsPanel({
  v,
  rows,
  byId,
  isOpen,
  onSelect,
}: {
  v: BudgetMonth;
  rows: BudgetLine[];
  byId: Map<string, Category>;
  isOpen: (o: Open) => boolean;
  onSelect: (id: string) => void;
}) {
  const f = v.funds;
  const fcols = "grid-cols-[minmax(0,1fr)_auto] sm:grid-cols-[minmax(0,1.3fr)_96px_104px_96px_112px]";
  return (
    <Panel
      bodyClassName="-mx-5"
      footer="A fund saves its amount up month by month, kept invested, and carries its balance, debt included: what goes past it is paid back by the months after. Its spending comes in lumps, so it has no pace. Kept is the amounts minus the spending; pay yourself first leaves it out."
    >
      <div className="flex flex-wrap items-end justify-between gap-3 px-5 pb-3">
        <div className="flex flex-col gap-0.5">
          <h2 className="m-0 flex items-center gap-2 type-heading">
            <PiggyBank className="size-4 text-ink-muted" aria-hidden />
            Funds
          </h2>
          <span className="type-caption text-ink-muted">Kept invested, apart from what is left to spend</span>
        </div>
        <span className="flex flex-col items-end">
          <Amount cents={f.balanceCents} size="lg" sign="negative" tone={f.balanceCents < 0 ? "warning" : "ink"} />
          <span className="type-caption text-ink-muted">saved, {rows.length === 1 ? "in the one fund" : `the ${rows.length} funds together`}</span>
        </span>
      </div>
      <div role="table" aria-label={`Funds in ${formatMonth(v.month)}`}>
        <div role="row" className={cn("grid h-8 items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted", fcols)}>
          <span role="columnheader">Fund</span>
          <span role="columnheader" className="hidden text-right sm:block">
            Brought
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            In this month
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            Spent
          </span>
          <span role="columnheader" className="text-right">
            Balance
          </span>
        </div>
        {rows.map((l) => {
          const on = isOpen({ line: l.id });
          const red = l.leftCents < 0;
          return (
            <button
              key={l.id}
              type="button"
              role="row"
              aria-selected={on}
              onClick={() => onSelect(l.id)}
              className={cn(
                "grid min-h-16 w-full cursor-pointer items-center gap-x-4 border-0 border-b border-rule px-5 py-2.5 text-left focus-visible:focus-ring",
                fcols,
                on ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken",
              )}
            >
              <span role="cell" className="flex min-w-0 flex-col gap-1">
                <LineName l={l} byId={byId} />
                {isShared(l) && <span className="truncate type-caption text-ink-muted">{members(l, byId)}</span>}
                <span className={cn("inline-flex items-center gap-1 type-caption", red ? "font-semibold text-warning" : "text-ink-muted")}>
                  {red && <TriangleAlert className="size-3.5" aria-hidden />}
                  {fundStatus(l, v.month)}
                </span>
              </span>
              <span role="cell" className="hidden text-right sm:block">
                <Amount cents={l.carriedCents} tone="muted" />
              </span>
              <span role="cell" className="hidden text-right sm:block">
                <Amount cents={l.limitCents + l.adjustedCents} sign="always" tone="ink" />
              </span>
              <span role="cell" className="hidden text-right sm:block">
                <Amount cents={l.spentCents} sign="none" />
              </span>
              <span role="cell" className="text-right">
                <Amount cents={l.leftCents} tone={red ? "warning" : "ink"} />
              </span>
            </button>
          );
        })}
        <div role="row" className={cn("grid min-h-11 items-center gap-x-4 border-b border-rule px-5 py-2", fcols)}>
          <span role="cell" className="flex flex-col">
            <span className="text-[14px] font-semibold">Total</span>
            <span className="type-caption text-ink-muted">
              Kept this month <strong className="font-figure font-medium text-ink">{formatBRL(f.keptCents)}</strong>
              {f.keptCents < 0 && ": they paid more than the month put in"}
            </span>
          </span>
          <span role="cell" className="hidden text-right sm:block">
            <Amount cents={f.carriedCents} tone="muted" />
          </span>
          <span role="cell" className="hidden text-right sm:block">
            <Amount cents={f.amountCents + f.adjustedCents} sign="always" tone="ink" />
          </span>
          <span role="cell" className="hidden text-right sm:block">
            <Amount cents={f.spentCents} sign="none" />
          </span>
          <span role="cell" className="text-right">
            <Amount cents={f.balanceCents} tone={f.balanceCents < 0 ? "warning" : "ink"} />
          </span>
        </div>
      </div>
    </Panel>
  );
}

/** How a fund stands, in a few words. */
function fundStatus(l: BudgetLine, month: string): string {
  if (l.leftCents < 0) {
    if (l.limitCents <= 0) return "In the red";
    return `In the red · back above zero in ${monthName(addMonths(month, Math.ceil(-l.leftCents / l.limitCents)))}`;
  }
  if (l.spentCents > 0) return `Paid ${formatBRL(l.spentCents)} this month`;
  return "Saving up";
}

/** Spending in categories no line covers, and what still waits for a category. */
function Unlimited({
  v,
  rows,
  byId,
  isOpen,
  onSelect,
  className = "mt-5",
}: {
  v: BudgetMonth;
  rows: BudgetSpend[];
  byId: Map<string, Category>;
  isOpen: (o: Open) => boolean;
  onSelect: (id: string) => void;
  className?: string;
}) {
  const total = rows.reduce((s, r) => s + r.spentCents, 0);
  return (
    <div role="table" aria-label="Spending without a limit" className={className}>
      <div className="flex flex-wrap items-baseline justify-between gap-2 px-5 pb-2">
        <h3 className="m-0 text-[14px] leading-5 font-semibold">Without a limit</h3>
        <span className="type-caption text-ink-muted">{formatBRL(total)} this month, outside every line</span>
      </div>
      <div role="row" className="grid h-8 grid-cols-[minmax(0,1fr)_96px_96px] items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted">
        <span role="columnheader">Category</span>
        <span role="columnheader" className="text-right">
          6-mo avg
        </span>
        <span role="columnheader" className="text-right">
          {monthName(v.month).slice(0, 3)}
        </span>
      </div>
      {rows.map((r) => {
        const cat = r.category ? byId.get(r.category) : undefined;
        const on = r.category !== null && isOpen({ category: r.category });
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

function Attention({ v, rows, byId, onSelect }: { v: BudgetMonth; rows: BudgetLine[]; byId: Map<string, Category>; onSelect: (id: string) => void }) {
  const spare = v.notAssignedCents;
  return (
    <section aria-labelledby="attention-title" className="flex flex-col gap-3 rounded-md border border-rule bg-paper-raised p-5">
      <h2 id="attention-title" className="m-0 type-heading">
        Needs attention · {rows.length}
      </h2>
      {rows.map((c) => {
        const name = lineName(c, byId);
        const over = c.pace === "over";
        const daily = v.daysLeft > 0 ? Math.max(c.leftCents, 0) / v.daysLeft : 0;
        return (
          <div key={c.id} className="flex flex-col gap-2 border-t border-rule pt-3 first-of-type:border-0 first-of-type:pt-0">
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
              {over && spare >= -c.leftCents && <CoverButton v={v} l={c} byId={byId} />}
              <Button variant="ghost" size="sm" onClick={() => onSelect(c.id)}>
                Details <ArrowRight className="size-3.5" aria-hidden />
              </Button>
            </div>
          </div>
        );
      })}
    </section>
  );
}

/** Raises an overspent limit by what it is over, this month on, taking it from Not assigned; Undo puts the lines back. */
function CoverButton({ v, l, byId, label = "Cover with Not assigned" }: { v: BudgetMonth; l: BudgetLine; byId: Map<string, Category>; label?: string }) {
  const [busy, setBusy] = useState(false);
  return (
    <Button
      variant="primary"
      size="sm"
      disabled={busy}
      onClick={async () => {
        const before = snapshot(v);
        setBusy(true);
        try {
          await saveLine(v.month, l.id, lineBody(l, { limitCents: l.limitCents - l.leftCents }));
          toast({
            id: "budget",
            text: (
              <>
                <strong>{lineName(l, byId)}</strong> covered with {formatBRL(-l.leftCents)} from Not assigned.
              </>
            ),
            undo: () => restore(v.month, before),
          });
        } catch (e) {
          toast({ id: "budget", tone: "error", text: `Could not cover it: ${(e as Error).message}.` });
        } finally {
          setBusy(false);
        }
      }}
    >
      {label}
    </Button>
  );
}

/**
 * A limit, or a category without one: where its limit comes from, its last
 * six months, where the money went (by category for a shared line), and its
 * limit. In a phone sheet its head says the name.
 */
function LineDetail({
  v,
  l,
  spend,
  byId,
  inSheet,
  onEdit,
}: {
  v: BudgetMonth;
  l?: BudgetLine;
  spend?: BudgetSpend;
  byId: Map<string, Category>;
  inSheet?: boolean;
  onEdit?: () => void;
}) {
  const single = l ? (isShared(l) ? undefined : l.byCategory[0]?.category) : (spend?.category ?? undefined);
  const cat = single ? byId.get(single) : undefined;
  const name = l ? lineName(l, byId) : (cat?.name ?? "Archived category");
  const history = l?.history ?? spend?.history ?? [];
  const average = l?.averageCents ?? spend?.averageCents ?? 0;
  const spent = l?.spentCents ?? spend?.spentCents ?? 0;
  const count = l?.count ?? spend?.count ?? 0;
  const list = useResource<TransactionList>(single ? `/api/v1/transactions?month=${v.month}&category=${single}&flow=out` : null);
  const merchants = useMemo(() => byMerchant(list.data?.transactions.map((t) => ({ d: t.description, c: -t.amountCents })) ?? []), [list.data]);
  const colors = l ? lineColors(l, byId) : [cat?.color ?? "note-200"];

  return (
    <section aria-labelledby={inSheet ? undefined : "detail-title"} className={cn("flex flex-col gap-4", !inSheet && "rounded-md border border-rule bg-paper-raised p-5")}>
      <div className={cn("flex flex-wrap items-center justify-between gap-2", inSheet && "empty:hidden")}>
        {!inSheet && (
          <h2 id="detail-title" className="m-0 flex items-center gap-2 type-heading">
            <span className="inline-flex gap-0.5" aria-hidden>
              {colors.map((c, i) => (
                <span key={i} className={cn("h-3 rounded-xs", colors.length > 1 ? "w-1.5" : "w-3")} style={{ background: `var(--${c})` }} />
              ))}
            </span>
            {name}
          </h2>
        )}
        {l?.pace === "over" && (
          <Badge tone="danger" icon={CircleAlert}>
            Over
          </Badge>
        )}
        {l?.pace === "ahead" && (
          <Badge tone="warning" icon={TriangleAlert}>
            Ahead of pace
          </Badge>
        )}
        {!l && <Badge>No limit</Badge>}
      </div>

      {l ? (
        <>
          <div className="flex flex-col gap-0.5">
            <span className="flex flex-wrap items-baseline gap-2">
              <Amount cents={l.leftCents} size="lg" tone={l.leftCents < 0 ? "danger" : "ink"} />
              <span className="type-caption text-ink-muted">left of {formatBRL(l.availableCents)}</span>
            </span>
            <span className="type-caption text-ink-muted">
              {formatBRL(spent)} spent on {count} {count === 1 ? "transaction" : "transactions"}
              {l.fixedCents > 0 && `, ${formatBRL(l.fixedCents)} of it in fixed charges`}
              {l.upcomingCents > 0 && `; ${formatBRL(l.upcomingCents)} more still to come`}
            </span>
          </div>
          <div role="table" aria-label="Where the limit comes from" className="flex flex-col type-caption">
            <div role="row" className="flex justify-between border-t border-rule py-1.5">
              <span role="cell" className="text-ink-muted">
                Monthly limit
              </span>
              <span role="cell">
                <Amount cents={l.limitCents} sign="none" size="sm" />
              </span>
            </div>
            <div role="row" className="flex justify-between border-t border-rule py-1.5">
              <span role="cell" className="text-ink-muted">
                Rolled over from {monthName(addMonths(v.month, -1))}
              </span>
              <span role="cell">
                <Amount cents={l.carriedCents} sign="always" tone="ink" size="sm" />
              </span>
            </div>
            <div role="row" className="flex justify-between border-t border-ink py-1.5 font-semibold">
              <span role="cell">Available in {monthName(v.month)}</span>
              <span role="cell">
                <Amount cents={l.availableCents} sign="none" size="sm" />
              </span>
            </div>
          </div>
        </>
      ) : (
        <span className="type-caption text-ink-muted">
          {formatBRL(spent)} spent this month on {count} {count === 1 ? "transaction" : "transactions"}, with no line holding it.
        </span>
      )}

      <History history={history} now={{ month: v.month, spentCents: spent }} average={average} limit={l?.limitCents} name={name} color={colors.length === 1 ? colors[0] : undefined} />

      {l && isShared(l) ? (
        <div className="flex flex-col gap-1.5">
          <span className="type-label text-ink-muted">By category</span>
          {l.byCategory.map((p) => {
            const c = byId.get(p.category);
            return (
              <Link
                key={p.category}
                to={`/transactions?month=${v.month}&category=${p.category}`}
                className="flex items-center justify-between gap-3 text-[14px] leading-5 text-ink no-underline hover:text-ink"
              >
                <span className="flex min-w-0 flex-col">
                  <CategoryTag name={c?.name ?? "Archived"} color={c?.color} />
                  <span className="pl-5 type-caption text-ink-muted">
                    {p.count} {p.count === 1 ? "transaction" : "transactions"} · avg {formatBRL(p.averageCents)}
                  </span>
                </span>
                <Amount cents={p.spentCents} sign="none" size="sm" />
              </Link>
            );
          })}
        </div>
      ) : (
        merchants.length > 0 && (
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
        )
      )}

      <LimitEditor key={(l?.id ?? single ?? "") + (l?.limitCents ?? 0)} v={v} line={l} category={single} name={name} suggested={average} onEdit={onEdit} />

      {single && (
        <Link to={`/transactions?month=${v.month}&category=${single}`} className="self-start text-[14px] font-semibold">
          See the {count} {count === 1 ? "transaction" : "transactions"}
        </Link>
      )}
    </section>
  );
}

// Words a bank adds around the merchant's name.
const noise = new Set(["PAG", "PG", "PIX", "ENVIADO", "RECEBIDO", "COMPRA", "DEBITO", "CREDITO", "ESTORNO", "PENDING"]);

/** "PAG*IFOOD SP", "IFOOD *IFOOD" and "PG *IFOOD" are all "IFOOD". */
export function merchantName(description: string): string {
  const words = description
    .toUpperCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
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

/**
 * A limit's amount and rollover, saved with its button; a category without
 * a line sets one here. Edit line opens the editor for its categories and
 * kind.
 */
function LimitEditor({
  v,
  line,
  category,
  name,
  suggested,
  onEdit,
}: {
  v: BudgetMonth;
  line?: BudgetLine;
  category?: string;
  name: string;
  suggested: number;
  onEdit?: () => void;
}) {
  const formId = useId();
  const start = line?.limitCents ?? Math.ceil(suggested / 5000) * 5000;
  const [text, setText] = useState(start ? inputBRL(start) : "");
  const [rollover, setRollover] = useState(line?.rollover ?? v.rolloverNew);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cents = parseBRL(text);
  const changed = !line || cents !== line.limitCents || rollover !== line.rollover;
  const month = monthName(v.month);

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

  async function remove() {
    if (!line) return;
    const before = snapshot(v);
    await run(async () => {
      await send("DELETE", `/api/v1/budget/${v.month}/lines/${line.id}`);
      toast({
        id: "budget",
        text: (
          <>
            <strong>{name}</strong> has no limit from {month} on.
          </>
        ),
        undo: () => restore(v.month, before),
      });
    });
  }

  async function save(cents: number) {
    const before = snapshot(v);
    if (line) await saveLine(v.month, line.id, lineBody(line, { limitCents: cents, rollover }));
    else await send("PUT", `/api/v1/budget/${v.month}/limits/${category}`, { limitCents: cents, rollover });
    toast({
      id: "budget",
      text: (
        <>
          <strong>{name}</strong>: a limit of {formatBRL(cents)} from {month} on.
        </>
      ),
      undo: () => restore(v.month, before),
    });
  }

  return (
    <form
      id={formId}
      className="flex flex-col gap-3 border-t border-rule pt-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (cents === null) return;
        void run(() => save(cents));
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
        hint={line ? `From ${month} on; earlier months keep theirs.` : suggested ? `Suggested from the 6-month average of ${name}.` : `From ${month} on.`}
      />
      <label className="flex cursor-pointer items-start gap-2.5">
        <Checkbox checked={rollover} onCheckedChange={(on) => setRollover(on === true)} className="mt-px" />
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
      <SheetActions className="flex flex-wrap items-center gap-2">
        <Button type="submit" form={formId} variant={line ? "secondary" : "primary"} size="sm" disabled={busy || cents === null || !changed}>
          {line ? "Save limit" : "Set limit"}
        </Button>
        {line && onEdit && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={onEdit}>
            Edit line
          </Button>
        )}
        {line && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void remove()}>
            Remove limit
          </Button>
        )}
      </SheetActions>
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
      await send("PUT", `/api/v1/budget/${v.month}/limits`, { limits: proposal.map((p) => ({ category: p.id, limitCents: p.cents, rollover: v.rolloverNew })) });
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

/** Where the month's money goes, as one bar (canvas: "Budget, phone"). */
function PhoneHero({ v, limits, isCurrent, byId }: { v: BudgetMonth; limits: BudgetLine[]; isCurrent: boolean; byId: Map<string, Category> }) {
  const left = v.leftCents - v.upcomingCents;
  const perDay = isCurrent && v.daysLeft > 0 && left > 0 ? left / v.daysLeft : 0;
  const pyf = v.payYourselfFirst.plannedCents;
  const funds = v.funds.amountCents;
  const inLimits = Math.max(0, v.leftCents);
  const spare = Math.max(0, v.notAssignedCents);
  const spent = limits
    .flatMap((l) => l.byCategory)
    .filter((p) => p.spentCents > 0)
    .sort((a, b) => b.spentCents - a.spentCents);
  const total = pyf + funds + v.spentCents + inLimits + spare;
  // Today counts as gone, as in the header's "Day 30 of 31" and in elapsedBp.
  const gone = v.days - v.daysLeft + 1;
  const legend: { label: string; cents: number; swatch: ReactNode }[] = [
    { label: "Pay yourself first", cents: pyf, swatch: <span className="size-2.5 rounded-xs bg-ink" /> },
    ...(funds > 0 ? [{ label: "Into the funds", cents: funds, swatch: <span className="size-2.5 rounded-xs bg-rule-strong" /> }] : []),
    {
      label: "Spent",
      cents: v.spentCents,
      swatch: (
        <span className="inline-flex h-2.5 w-[15px] gap-px">
          {spent.slice(0, 3).map((p) => (
            <span key={p.category} className="flex-1" style={{ background: `var(--${byId.get(p.category)?.color ?? "note-200"})` }} />
          ))}
        </span>
      ),
    },
    { label: "Left in limits", cents: v.leftCents, swatch: <span className="box-border size-2.5 rounded-xs border border-dashed border-rule-strong" /> },
    { label: "Not assigned", cents: v.notAssignedCents, swatch: <span className="box-border size-2.5 rounded-xs border border-rule bg-paper-sunken" /> },
  ];
  return (
    <Panel footer={isCurrent ? `${gone} of ${v.days} days gone (${Math.round(v.elapsedBp / 100)}%).` : `${formatMonth(v.month)} is over.`}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <span className="type-label text-ink-muted">{isCurrent ? "Left to spend" : "Left in the limits"}</span>
          <Amount cents={isCurrent ? left : v.leftCents} size="hero" sign="negative" tone={left < 0 ? "danger" : "ink"} />
          <span className="type-caption text-ink-muted">
            {isCurrent
              ? `${v.daysLeft} ${v.daysLeft === 1 ? "day" : "days"} left${perDay > 0 ? `, about ${formatBRL(perDay)} a day` : ""}${v.upcomingCents > 0 ? `, after ${formatBRL(v.upcomingCents)} in fixed charges to come` : ""}`
              : `${formatBRL(v.spentCents)} spent of ${formatBRL(v.availableCents)}`}
          </span>
        </div>
        {total > 0 && (
          <div
            role="img"
            aria-label={`Of ${formatBRL(total)}: pay yourself first ${formatBRL(pyf)},${funds > 0 ? ` into the funds ${formatBRL(funds)},` : ""} spent ${formatBRL(v.spentCents)}, left in limits ${formatBRL(inLimits)}, not assigned ${formatBRL(spare)}.`}
            className="flex h-4 gap-0.5"
          >
            {pyf > 0 && <span className="min-w-0 rounded-xs bg-ink" style={{ flex: `${pyf} 1 0` }} />}
            {funds > 0 && <span className="min-w-0 rounded-xs bg-rule-strong" style={{ flex: `${funds} 1 0` }} />}
            {v.spentCents > 0 && (
              <span className="flex min-w-0 gap-px" style={{ flex: `${v.spentCents} 1 0` }}>
                {spent.map((p) => (
                  <span key={p.category} className="min-w-0" style={{ flex: `${p.spentCents} 1 0`, background: `var(--${byId.get(p.category)?.color ?? "note-200"})` }} />
                ))}
              </span>
            )}
            {inLimits > 0 && <span className="box-border min-w-0 rounded-xs border border-dashed border-rule-strong" style={{ flex: `${inLimits} 1 0` }} />}
            {spare > 0 && <span className="min-w-0 rounded-xs bg-paper-sunken shadow-[inset_0_0_0_1px_var(--rule)]" style={{ flex: `${spare} 1 0` }} />}
          </div>
        )}
        <div className="grid grid-cols-2 gap-x-4 gap-y-2 type-caption text-ink-muted">
          {legend.map((l) => (
            <span key={l.label} className="flex flex-col">
              <span className="inline-flex items-center gap-1.5">
                <span aria-hidden className="inline-flex">
                  {l.swatch}
                </span>
                {l.label}
              </span>
              <strong className={cn("font-figure font-medium", l.cents < 0 ? "text-danger" : "text-ink")}>{formatBRL(l.cents)}</strong>
            </span>
          ))}
        </div>
      </div>
    </Panel>
  );
}

/** Pay yourself first on its own: set aside before any limit, never spending. */
function PayFirst({ v }: { v: BudgetMonth }) {
  const p = v.payYourselfFirst;
  return (
    <Panel>
      <div className="flex items-center gap-3">
        <span className="inline-flex size-9 flex-none items-center justify-center rounded-sm bg-tinta-soft text-tinta" aria-hidden>
          <PiggyBank className="size-5" />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="text-[15px] leading-[22px] font-semibold">Pay yourself first</span>
          <span className="type-caption text-ink-muted">
            {p.plannedCents === 0 ? <Link to="/settings#budget">Set an amount in Settings</Link> : `Set aside on ${formatDay(p.due)} · never spending`}
          </span>
        </span>
        {p.plannedCents > 0 && (
          <span className="flex flex-col items-end gap-1">
            <Amount cents={p.plannedCents} sign="none" />
            {p.doneOn ? (
              <Badge tone="inflow" icon={Check}>
                Done
              </Badge>
            ) : p.movedCents > 0 ? (
              <span className="type-caption text-ink-muted">{formatBRL(p.movedCents)} moved</span>
            ) : (
              <span className="type-caption text-ink-muted">Due {formatDay(p.due)}</span>
            )}
          </span>
        )}
      </div>
    </Panel>
  );
}

/** A group of limits on a phone, each a bar; tapping one opens its detail. */
function PhoneGroup({
  title,
  v,
  rows,
  byId,
  onSelect,
  isOpen,
  cover,
}: {
  title: string;
  v: BudgetMonth;
  rows: BudgetLine[];
  byId: Map<string, Category>;
  onSelect: (id: string) => void;
  isOpen: (o: Open) => boolean;
  cover?: boolean;
}) {
  const gone = v.elapsedBp / 100;
  const coverable = cover ? rows.find((c) => c.pace === "over" && v.notAssignedCents >= -c.leftCents) : undefined;
  return (
    <Panel title={`${title} · ${rows.length}`}>
      <div className="flex flex-col gap-5">
        {rows.map((c) => {
          const name = lineName(c, byId);
          const st = status(c);
          const fill = Math.min(c.availableCents > 0 ? (c.spentCents / c.availableCents) * 100 : c.spentCents > 0 ? 100 : 0, 100);
          return (
            <button
              key={c.id}
              type="button"
              aria-pressed={isOpen({ line: c.id })}
              onClick={() => onSelect(c.id)}
              className="-m-1.5 flex cursor-pointer flex-col gap-1.5 rounded-sm border-0 bg-transparent p-1.5 text-left text-ink focus-visible:focus-ring aria-pressed:bg-tinta-soft"
            >
              <span className="flex items-center justify-between gap-3">
                <span className="flex min-w-0 items-center gap-2">
                  <LineName l={c} byId={byId} />
                  {c.rollover && <Repeat className="size-3.5 flex-none text-ink-muted" aria-label="rolls over" />}
                </span>
                <span className="type-figure-sm whitespace-nowrap">
                  {formatBRL(c.spentCents)} <span className="text-ink-muted">of {formatBRL(c.availableCents)}</span>
                </span>
              </span>
              {isShared(c) && <span className="-mt-1 truncate type-caption text-ink-muted">{members(c, byId)}</span>}
              <PaceBar fill={fill} over={c.pace === "over"} parts={paceParts(c, byId)} gone={gone} label={`${name}: ${formatBRL(c.spentCents)} of ${formatBRL(c.availableCents)}`} />
              <span className="flex items-center justify-between gap-3 type-caption">
                <span className={cn("inline-flex items-center gap-1", st.tone)}>
                  {st.icon && <st.icon className="size-3.5" aria-hidden />}
                  {st.text}
                </span>
                <span className="text-ink-muted">{c.leftCents >= 0 ? `${formatBRL(c.leftCents)} left` : ""}</span>
              </span>
            </button>
          );
        })}
        {coverable && <CoverButton v={v} l={coverable} byId={byId} label={`Cover ${lineName(coverable, byId)} from Not assigned`} />}
      </div>
    </Panel>
  );
}

/** The funds on a phone: each with its balance; tapping one opens it in the sheet (canvas: "Budget, phone: a fund in a sheet"). */
function PhoneFunds({
  v,
  rows,
  byId,
  onSelect,
  isOpen,
}: {
  v: BudgetMonth;
  rows: BudgetLine[];
  byId: Map<string, Category>;
  onSelect: (id: string) => void;
  isOpen: (o: Open) => boolean;
}) {
  return (
    <Panel title={`Funds · ${rows.length}`} footer={`${formatBRL(v.funds.balanceCents)} saved, kept invested and apart from what is left to spend.`}>
      <div className="flex flex-col">
        {rows.map((l) => {
          const red = l.leftCents < 0;
          return (
            <button
              key={l.id}
              type="button"
              aria-pressed={isOpen({ line: l.id })}
              onClick={() => onSelect(l.id)}
              className="-mx-1.5 flex min-h-14 cursor-pointer items-center justify-between gap-3 rounded-sm border-0 border-b border-rule bg-transparent px-1.5 text-left text-ink last:border-0 focus-visible:focus-ring aria-pressed:bg-tinta-soft"
            >
              <span className="flex min-w-0 flex-col gap-0.5">
                <LineName l={l} byId={byId} />
                <span className={cn("inline-flex items-center gap-1 type-caption", red ? "font-semibold text-warning" : "text-ink-muted")}>
                  {red && <TriangleAlert className="size-3.5" aria-hidden />}
                  {red ? "In the red" : fundStatus(l, v.month)}
                </span>
              </span>
              <Amount cents={l.leftCents} tone={red ? "warning" : "ink"} />
            </button>
          );
        })}
      </div>
    </Panel>
  );
}

type Row = { key: string; line?: BudgetLine; category?: Category; label: ReactNode; hint?: string; average: number };

/** Every line at once: what each gets, whether a limit rolls over, and what is left unassigned. A shared line is one row; a category on its own, another. */
function LimitsEditor({ v, categories, byId, onDone }: { v: BudgetMonth; categories: Category[]; byId: Map<string, Category>; onDone: () => void }) {
  const [rows] = useState<Row[]>(() => {
    const inShared = new Set(v.lines.filter(isShared).flatMap((l) => l.byCategory.map((p) => p.category)));
    const own = new Map(v.lines.filter((l) => !isShared(l)).map((l) => [l.byCategory[0].category, l]));
    const avg = new Map<string, number>(v.unlimited.filter((u) => u.category).map((u) => [u.category!, u.averageCents]));
    const lines: Row[] = v.lines.filter(isShared).map((l) => ({ key: l.id, line: l, label: lineName(l, byId), hint: members(l, byId), average: l.averageCents }));
    const singles: Row[] = categories
      .filter((c) => c.budgeted && !inShared.has(c.id))
      .map((c) => {
        const l = own.get(c.id);
        return { key: c.id, line: l, category: c, label: c.name, average: l?.averageCents ?? avg.get(c.id) ?? 0 };
      });
    return [...lines, ...singles].sort((a, b) => (b.line?.limitCents ?? -1) - (a.line?.limitCents ?? -1) || b.average - a.average);
  });
  const [vals, setVals] = useState<Record<string, { text: string; rollover: boolean }>>(() =>
    Object.fromEntries(rows.map((r) => [r.key, { text: r.line ? inputBRL(r.line.limitCents) : "", rollover: r.line?.rollover ?? v.rolloverNew }])),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cents = (key: string) => (vals[key].text.trim() ? parseBRL(vals[key].text) : null);
  const name = (r: Row) => (r.line ? lineName(r.line, byId) : (r.category?.name ?? ""));
  const bad = rows.filter((r) => vals[r.key].text.trim() && cents(r.key) === null);
  const total = rows.reduce((s, r) => s + (cents(r.key) ?? 0), 0);
  const spare = v.notAssignedCents + v.limitsCents - total;
  const set = (key: string, p: Partial<{ text: string; rollover: boolean }>) => setVals((x) => ({ ...x, [key]: { ...x[key], ...p } }));
  const grid = "grid-cols-[minmax(0,1fr)_128px_44px] sm:grid-cols-[minmax(0,1fr)_110px_150px_96px]";

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (bad.length) return;
    const before = snapshot(v);
    setBusy(true);
    setError(null);
    try {
      const limits: (LineBody | { category: string; limitCents: number; rollover: boolean })[] = rows
        .filter((r) => cents(r.key) !== null)
        .map((r) =>
          r.line
            ? { ...lineBody(r.line, { limitCents: cents(r.key)!, rollover: !r.line.fund && vals[r.key].rollover }), id: r.line.id }
            : { category: r.category!.id, limitCents: cents(r.key)!, rollover: vals[r.key].rollover },
        );
      await send("PUT", `/api/v1/budget/${v.month}/limits`, { limits });
      // Replaces any earlier budget toast, whose Undo would put back the lines from before this save too.
      toast({ id: "budget", text: `${monthName(v.month)}'s limits saved.`, undo: () => restore(v.month, before) });
      onDone();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form id="limits-editor" onSubmit={save} className="scroll-mt-20">
      <Panel
        title="Edit all limits"
        action={
          <span className="flex flex-col items-end">
            <span className="type-caption text-ink-muted">Not assigned after</span>
            <Amount cents={spare} tone={spare < 0 ? "danger" : "ink"} />
          </span>
        }
        bodyClassName="-mx-5"
        footer={`From ${monthName(v.month)} on; earlier months keep theirs. An empty limit removes it; a shared line's categories and a fund change in the line's editor. ${v.incomeCents ? `${formatBRL(v.incomeCents)} came in, ${formatBRL(v.payYourselfFirst.plannedCents)} of it set aside first.` : ""}`}
      >
        <div role="table" aria-label="Limits">
          <div role="row" className={cn("grid h-8 items-center gap-x-3 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted", grid)}>
            <span role="columnheader">Line</span>
            <span role="columnheader" className="hidden text-right sm:block">
              6-mo avg
            </span>
            <span role="columnheader">Limit</span>
            <span role="columnheader" className="text-center">
              <span className="sm:hidden" aria-hidden>
                <Repeat className="inline size-3.5" />
              </span>
              <span className="hidden sm:inline">Rolls over</span>
            </span>
          </div>
          {rows.map((r) => {
            const val = vals[r.key];
            const wrong = Boolean(val.text.trim()) && cents(r.key) === null;
            const fund = r.line?.fund ?? false;
            return (
              <div role="row" key={r.key} className={cn("grid min-h-14 items-center gap-x-3 border-b border-rule px-5 py-1.5", grid)}>
                <span role="cell" className="flex min-w-0 flex-col gap-0.5">
                  <span className="flex min-w-0 items-center gap-2">
                    {r.line && isShared(r.line) ? <LineName l={r.line} byId={byId} /> : <CategoryTag name={r.category?.name ?? name(r)} color={r.category?.color} />}
                    {fund && (
                      <Badge icon={PiggyBank} className="flex-none">
                        Fund
                      </Badge>
                    )}
                  </span>
                  {r.hint && <span className="truncate pl-5 type-caption text-ink-muted">{r.hint}</span>}
                  <span className="type-caption text-ink-muted sm:hidden">avg {formatBRL(r.average)}</span>
                  {wrong && (
                    <span id={`limit-${r.key}-error`} className="flex items-center gap-1 type-caption text-danger">
                      <CircleAlert className="size-3.5 flex-none" aria-hidden />
                      Type an amount, like 500,00
                    </span>
                  )}
                </span>
                <span role="cell" className="hidden text-right sm:block">
                  <Amount cents={r.average} sign="none" tone="muted" />
                </span>
                <span role="cell">
                  <label className={cn("flex h-11 items-center gap-1 rounded-sm border bg-paper-raised px-2.5 font-figure text-[14px] focus-within:focus-ring lg:h-9", wrong ? "border-danger" : "border-rule-strong")}>
                    <span className="text-ink-muted">R$</span>
                    <input
                      aria-label={`${fund ? "Each month for" : "Limit for"} ${name(r)}`}
                      aria-invalid={wrong}
                      aria-describedby={wrong ? `limit-${r.key}-error` : undefined}
                      inputMode="decimal"
                      placeholder="No limit"
                      value={val.text}
                      onChange={(e) => set(r.key, { text: e.target.value })}
                      className="w-full min-w-0 border-0 bg-transparent text-right text-ink outline-none placeholder:text-ink-muted"
                    />
                  </label>
                </span>
                <span role="cell" className="flex justify-center">
                  {fund ? (
                    <span className="type-caption text-ink-muted">carries</span>
                  ) : (
                    <Checkbox aria-label={`${name(r)} rolls over`} checked={val.rollover} onCheckedChange={(on) => set(r.key, { rollover: on === true })} />
                  )}
                </span>
              </div>
            );
          })}
          <div role="row" className={cn("grid min-h-11 items-center gap-x-3 px-5 font-semibold", grid)}>
            <span role="cell" className="text-[14px]">
              Lines, a month
            </span>
            <span role="cell" className="hidden sm:block" />
            <span role="cell" className="pr-2.5 text-right">
              <Amount cents={total} sign="none" />
            </span>
            <span role="cell" />
          </div>
        </div>
        <div className="flex flex-col gap-2 px-5 pt-3">
          {bad.length > 0 && <p className="m-0 type-caption text-danger">Type an amount, like 500,00, for {bad.map(name).join(", ")}.</p>}
          {error && (
            <p role="alert" className="m-0 type-caption text-danger">
              Could not save: {error}.
            </p>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" variant="primary" disabled={busy || bad.length > 0}>
              Save all limits
            </Button>
            <Button type="button" variant="ghost" onClick={onDone}>
              Cancel
            </Button>
          </div>
        </div>
      </Panel>
    </form>
  );
}
