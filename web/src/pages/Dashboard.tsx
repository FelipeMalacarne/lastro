import { Check, CircleAlert, Clock, Info, Sparkles, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { RunningTotal } from "@/components/charts/RunningTotal";
import { Kbd } from "@/components/Kbd";
import { Panel } from "@/components/Panel";
import { Sparkline } from "@/components/Sparkline";
import { Stat } from "@/components/Stat";
import { SyncNotice } from "@/components/SyncNotice";
import { TransactionRow } from "@/components/TransactionRow";
import { Button } from "@/components/ui/button";
import { SyncButton } from "@/lib/sync";
import {
  typeLabel,
  useResource,
  type BudgetMonth,
  type NetWorthPoint,
  type PortfolioOverview,
  type Category,
  type DaysView,
  type Recurring,
  type Review,
  type Transaction,
  type TransactionList,
} from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, formatDay, formatMonth, formatPercent, keepAmounts, titleCase, todayISO } from "@/lib/format";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";
import { useBanks } from "@/shell/Accounts";

/** The month at a glance (canvas: "Dashboard", "Dashboard, phone"). */
export function Dashboard() {
  const { month, isCurrent } = useMonth();
  const budget = useResource<BudgetMonth>(`/api/v1/budget?month=${month}`).data;
  const days = useResource<DaysView>(`/api/v1/transactions/days?month=${month}`).data;
  const list = useResource<TransactionList>(`/api/v1/transactions?month=${month}`).data;
  const { banks } = useBanks();
  const { byId } = useCategories();
  const today = todayISO();
  const monthName = formatMonth(month).split(" ")[0];

  const synced = banks
    .map((b) => b.connection?.dataUpdatedAt)
    .filter((d): d is string => Boolean(d))
    .sort()
    .at(-1);
  const caption = [
    budget && isCurrent ? `Day ${budget.days - budget.daysLeft + 1} of ${budget.days}` : budget ? `${formatMonth(month)} is over` : null,
    synced ? `synced ${ago(synced)}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <PageHeader
        title="Dashboard"
        caption={caption || undefined}
        actions={
          <>
            {(!isCurrent || new Date().getDate() <= 7) && (
              <Button asChild variant="secondary">
                <Link to={`/report?month=${isCurrent ? prevOf(month) : month}`}>{formatMonth(isCurrent ? prevOf(month) : month).split(" ")[0]} in review</Link>
              </Button>
            )}
            <SyncButton />
          </>
        }
      />
      <SyncNotice banks={banks} />
      {isCurrent && <Alerts />}
      <ReportReminders />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-6">
        <NetWorth />
        {budget && days ? <MonthSoFar v={budget} days={days} isCurrent={isCurrent} byId={byId} /> : <Panel title={`${monthName} so far`}>Loading…</Panel>}
      </div>

      <div className="grid gap-4 lg:grid-cols-2 lg:gap-6">
        {budget ? <ByCategory v={budget} byId={byId} isCurrent={isCurrent} /> : <Panel title="Spending by category">Loading…</Panel>}
        <Panel title="Spending through the month" footer={`Spending added up by day, with ${formatMonth(prevOf(month)).split(" ")[0]} for comparison.`}>
          {days ? <Through month={month} days={days} today={today} isCurrent={isCurrent} /> : "Loading…"}
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Recent list={list} byId={byId} month={month} />
        <ToReview list={list} byId={byId} monthName={monthName} />
      </div>

      <FixedCosts byId={byId} />
    </>
  );
}

type Alert = { key: string; kind: string; severity: "danger" | "warning" | "info"; title: string; detail: string; link: string };

/** What is worth knowing this month (F10): over limits, ahead of pace, unusual spending and changed charges. */
function Alerts() {
  const r = useResource<{ alerts: Alert[] }>("/api/v1/alerts").data;
  if (!r?.alerts.length) return null;
  const icon = { danger: CircleAlert, warning: TriangleAlert, info: Info };
  const tone = { danger: "text-danger", warning: "text-warning", info: "text-tinta" };
  return (
    <section aria-label="Alerts" className="flex flex-col overflow-hidden rounded-md border border-rule bg-paper-raised">
      {r.alerts.map((a) => {
        const Icon = icon[a.severity];
        return (
          <Link
            key={a.key}
            to={a.link}
            className="flex items-start gap-3 border-b border-rule px-4 py-2.5 text-ink no-underline last:border-b-0 hover:bg-paper-sunken hover:text-ink"
          >
            <Icon className={cn("mt-0.5 size-4 flex-none", tone[a.severity])} aria-hidden />
            <span className="flex min-w-0 flex-col">
              <span className="text-[14px] leading-5 font-semibold">{keepAmounts(a.title)}</span>
              <span className="type-caption text-ink-muted">{keepAmounts(a.detail)}</span>
            </span>
          </Link>
        );
      })}
    </section>
  );
}

/** Earnings reports this week for stocks held, with the thesis to read before them (F8). */
function ReportReminders() {
  const r = useResource<{ reminders: { ticker: string; date: string; inDays: number; thesis: string; breaks: string }[] }>("/api/v1/reminders").data;
  if (!r?.reminders.length) return null;
  return (
    <section aria-label="Earnings reports this week" className="flex flex-col gap-2 rounded-md border border-warning bg-warning-soft/50 px-4 py-3">
      {r.reminders.map((m) => (
        <div key={m.ticker} className="flex flex-col gap-0.5 text-[14px] leading-5">
          <span className="inline-flex flex-wrap items-center gap-x-2">
            <Clock className="size-4 text-warning" aria-hidden />
            <strong className="font-figure font-medium">{m.ticker}</strong>
            <span>reports {m.inDays === 0 ? "today" : m.inDays === 1 ? "tomorrow" : `in ${m.inDays} days, on ${formatDay(m.date)}`}.</span>
            <Link to="/portfolio" className="font-semibold">
              Read the thesis
            </Link>
          </span>
          <span className="pl-6 type-caption text-ink-muted">
            Thesis: {m.thesis} Breaks if: {m.breaks}
          </span>
        </div>
      ))}
    </section>
  );
}

function ago(iso: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
  if (mins < 60) return `${mins} min ago`;
  const h = Math.round(mins / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}

function prevOf(m: string): string {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(y, mo - 2, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

/** Net worth (F6): everything owned, its last twelve months, and what it is made of. */
function NetWorth() {
  const res = useResource<{ points: NetWorthPoint[] }>("/api/v1/net-worth?months=12");
  // The same twelve months as the Portfolio, so the two never disagree.
  const overview = useResource<PortfolioOverview>("/api/v1/portfolio").data;
  const points = res.data?.points ?? [];
  const now = points.at(-1);
  if (!now) return <Panel title="Net worth">{res.error ? `Could not load: ${res.error}.` : "Loading…"}</Panel>;
  const first = points[0];
  // Accounts typed in later than the first point would read as growth:
  // compare like with like and say what is left out.
  const late = first.otherCents === 0 && now.otherCents > 0;
  const counted = (p: NetWorthPoint) => p.totalCents - (late ? p.otherCents : 0);
  const delta = counted(now) - counted(first);
  const fromLabel = formatMonth(first.date.slice(0, 7), true);
  const rows = [
    { label: "Invested", cents: now.investedCents, to: "/portfolio" },
    { label: "Emergency fund", cents: now.emergencyCents, to: "/portfolio" },
    { label: "Cash after card bills", cents: now.cashCents + now.cardsCents, to: "/transactions" },
    ...(now.otherCents ? [{ label: "Outside Open Finance", cents: now.otherCents, to: "/settings#manual" }] : []),
  ];
  return (
    <Panel className="lg:p-6" footer="Investments at the latest prices; accounts as the banks last reported them.">
      <div className="flex flex-col gap-5">
        <div className="flex flex-col gap-1">
          <span className="type-label text-ink-muted">Net worth</span>
          <Amount cents={now.totalCents} size="hero" sign="none" />
          {points.length > 1 && (
            <span className={cn("type-caption", delta >= 0 ? "text-inflow" : "text-warning")}>
              {formatBRL(delta, { sign: "always" })} since {fromLabel}
              {late && <span className="text-ink-muted"> · not counting {formatBRL(now.otherCents)} typed in recently</span>}
            </span>
          )}
        </div>
        {points.length > 2 && (
          <div className="flex w-[300px] max-w-full flex-col gap-1">
            <Sparkline zero={false} values={points.map(counted)} width={300} height={56} label={`Net worth since ${fromLabel}, from ${formatBRL(first.totalCents)} to ${formatBRL(now.totalCents)}`} />
            <div className="flex justify-between type-figure-sm text-ink-muted">
              <span>{fromLabel}</span>
              <span>Today</span>
            </div>
          </div>
        )}
        {overview?.putIn12mCents != null && overview.since && (
          <div className="grid grid-cols-2 gap-4 border-t border-rule pt-3">
            <div className="flex flex-col gap-0.5">
              <span className="type-caption text-ink-muted">Invested since {formatDay(overview.since)} {overview.since.slice(0, 4)}</span>
              <Amount cents={overview.putIn12mCents} sign="none" />
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="type-caption text-ink-muted">Returns on it</span>
              <Amount cents={overview.returns12mCents ?? 0} sign="always" />
            </div>
          </div>
        )}
        <div className="flex flex-col">
          {rows.map((r) => (
            <Link
              key={r.label}
              to={r.to}
              className="-mx-2 flex min-h-10 items-center justify-between gap-3 rounded-sm border-t border-rule px-2 text-[14px] leading-5 text-ink no-underline hover:bg-paper-sunken hover:text-ink"
            >
              <span>{r.label}</span>
              <Amount cents={r.cents} size="sm" />
            </Link>
          ))}
        </div>
      </div>
    </Panel>
  );
}

/** The month in four numbers, and where its income went. */
function MonthSoFar({ v, days, isCurrent, byId }: { v: BudgetMonth; days: DaysView; isCurrent: boolean; byId: Map<string, Category> }) {
  const monthName = formatMonth(v.month).split(" ")[0];
  const spent = v.summary.spentCents;
  const prevToDate = days.summary.previousToDateCents;
  const diff = spent - prevToDate;
  const prevName = formatMonth(prevOf(v.month)).split(" ")[0].slice(0, 3);
  const pyf = v.payYourselfFirst;
  const left = v.leftCents - v.upcomingCents;

  // Where the income went: pay yourself first, spending by category, what the limits still hold, the rest.
  const segments: { key: string; label: string; cents: number; color: string }[] = [];
  if (pyf.movedCents > 0) segments.push({ key: "pyf", label: "Pay yourself first", cents: pyf.movedCents, color: "var(--ink)" });
  const spend = [...v.categories.map((c) => ({ id: c.category as string | null, cents: c.spentCents })), ...v.unlimited.map((u) => ({ id: u.category, cents: u.spentCents }))]
    .filter((s) => s.cents > 0)
    .sort((a, b) => b.cents - a.cents);
  for (const s of spend) {
    const c = s.id ? byId.get(s.id) : undefined;
    segments.push({ key: s.id ?? "none", label: c?.name ?? "To review", cents: s.cents, color: c ? `var(--${c.color})` : "var(--warning)" });
  }
  const inLimits = Math.max(v.leftCents, 0);
  if (inLimits > 0) segments.push({ key: "left", label: "Left in limits", cents: inLimits, color: "transparent" });
  const rest = v.incomeCents - pyf.movedCents - spent - inLimits;
  if (rest > 0) segments.push({ key: "rest", label: "Outside the limits", cents: rest, color: "transparent" });
  const total = Math.max(v.incomeCents, segments.reduce((s, x) => s + x.cents, 0), 1);

  return (
    <Panel title={isCurrent ? `${monthName} so far` : monthName} footer="Transfers between your accounts and card bill payments are never counted as spending.">
      <div className="flex flex-col gap-6">
        <div className="grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-4">
          <Stat label="Inflows" cents={v.incomeCents} sign="none" tone="inflow" caption={`${v.incomeCount} ${v.incomeCount === 1 ? "income" : "incomes"}`} />
          <Stat
            label="Spent"
            cents={spent}
            sign="none"
            delta={
              prevToDate > 0
                ? { text: `${formatBRL(Math.abs(diff))} ${diff <= 0 ? "less" : "more"} than ${isCurrent ? `by ${Math.min(Number(todayISO().slice(8)), monthDays(prevOf(v.month)))} ${prevName}` : `in ${prevName}`}`, tone: diff <= 0 ? "inflow" : "warning" }
                : undefined
            }
          />
          <Stat
            label="Pay yourself first"
            cents={pyf.movedCents}
            sign="none"
            caption={pyf.doneOn ? `Done ${formatDay(pyf.doneOn)}` : pyf.plannedCents ? `of ${formatBRL(pyf.plannedCents)}, due ${formatDay(pyf.due)}` : "Not set"}
          />
          <Stat
            label={isCurrent ? "Left to spend" : "Left in limits"}
            cents={isCurrent ? left : v.leftCents}
            sign="negative"
            tone={left < 0 ? "danger" : "ink"}
            caption={v.hasBudget ? `of ${formatBRL(v.availableCents)} in limits${isCurrent ? ` · ${v.daysLeft} ${v.daysLeft === 1 ? "day" : "days"} left` : ""}` : <Link to="/budget">Set a budget</Link>}
          />
        </div>

        {v.incomeCents > 0 && (
          <div className="flex flex-col gap-2.5">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <span className="text-[14px] leading-5 font-semibold">Where {monthName}'s {formatBRL(v.incomeCents)} went</span>
              <span className="type-caption text-ink-muted">
                Savings rate {isCurrent ? "so far " : ""}
                <strong className={cn("font-figure font-medium", v.summary.savingsRateBp >= 0 ? "text-inflow" : "text-danger")}>{formatPercent(v.summary.savingsRateBp)}</strong>
              </span>
            </div>
            <div
              role="img"
              aria-label={segments.map((s) => `${s.label} ${formatBRL(s.cents)}`).join(", ")}
              className="flex h-4 w-full gap-px overflow-hidden rounded-xs"
            >
              {segments.map((s) => (
                <span
                  key={s.key}
                  title={`${s.label}: ${formatBRL(s.cents)}`}
                  className={cn(
                    "h-full",
                    s.key === "left" && "box-border border-[1.5px] border-dashed border-ink-muted",
                    s.key === "rest" && "box-border border-[1.5px] border-rule-strong bg-[image:repeating-linear-gradient(135deg,var(--rule-strong)_0_1.5px,transparent_1.5px_6px)]",
                  )}
                  style={{ width: `${(s.cents / total) * 100}%`, background: s.color }}
                />
              ))}
            </div>
            <div className="flex flex-wrap gap-x-4 gap-y-1 type-caption text-ink-muted">
              {pyf.movedCents > 0 && <Legend color="var(--ink)">Pay yourself first {formatBRL(pyf.movedCents)}</Legend>}
              <span className="inline-flex items-center gap-1.5">
                <span className="flex h-2.5 w-4 overflow-hidden rounded-xs" aria-hidden>
                  <span className="flex-1 bg-note-100" />
                  <span className="flex-1 bg-note-10" />
                  <span className="flex-1 bg-note-1" />
                </span>
                Spent {formatBRL(spent)}, in each category's colour
              </span>
              {inLimits > 0 && <Legend dashed>Left in limits {formatBRL(inLimits)}</Legend>}
              {rest > 0 && (
                <span className="inline-flex items-center gap-1.5">
                  <span className="box-border size-2.5 rounded-xs border border-rule-strong bg-[image:repeating-linear-gradient(135deg,var(--rule-strong)_0_1px,transparent_1px_3px)]" aria-hidden />
                  Outside the limits {formatBRL(rest)}
                </span>
              )}
            </div>
          </div>
        )}
      </div>
    </Panel>
  );
}

function Legend({ color, dashed, children }: { color?: string; dashed?: boolean; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn("size-2.5 rounded-xs", dashed && "box-border border-[1.5px] border-dashed border-ink-muted")} style={color ? { background: color } : undefined} aria-hidden />
      {children}
    </span>
  );
}

/** Each category's spending against its 6-month average, with one sentence on what stands out. */
function ByCategory({ v, byId, isCurrent }: { v: BudgetMonth; byId: Map<string, Category>; isCurrent: boolean }) {
  const rows = [
    ...v.categories.map((c) => ({ id: c.category as string | null, spent: c.spentCents, avg: c.averageCents })),
    ...v.unlimited.map((u) => ({ id: u.category, spent: u.spentCents, avg: u.averageCents })),
  ]
    .filter((r) => r.spent > 0 || r.avg > 0)
    .sort((a, b) => b.spent - a.spent || b.avg - a.avg);
  const total = rows.reduce((s, r) => s + Math.max(r.spent, 0), 0);
  const max = Math.max(1, ...rows.map((r) => Math.max(r.spent, r.avg)));
  const nameOf = (id: string | null) => (id ? (byId.get(id)?.name ?? "Archived") : "To review");

  // What stands out: in a closed month, how far each category landed from its
  // average; in this one, only the categories already past their whole-month average.
  const named = rows.filter((r) => r.id && r.avg >= 5000);
  let insight: React.ReactNode;
  if (rows.length === 0) {
    insight = `No spending recorded in ${formatMonth(v.month)}.`;
  } else if (isCurrent) {
    // Past it by 10% and R$ 50 at least, so a rent paid to the cent is not news.
    const past = named.filter((r) => r.spent > r.avg * 1.1 && r.spent - r.avg >= 5000).sort((a, b) => b.spent / b.avg - a.spent / a.avg);
    insight = past.length ? (
      <>
        <strong>{past.map((r) => nameOf(r.id)).join(", ")}</strong> {past.length === 1 ? "has" : "have"} already passed {past.length === 1 ? "its" : "their"} 6-month average,
        with {v.daysLeft} {v.daysLeft === 1 ? "day" : "days"} to go.
      </>
    ) : (
      "No category is more than 10% past its 6-month average."
    );
  } else {
    const off = named.map((r) => ({ ...r, pct: Math.round(((r.spent - r.avg) / r.avg) * 100) })).filter((r) => Math.abs(r.pct) >= 10 && Math.abs(r.spent - r.avg) >= 5000);
    const up = off.filter((r) => r.pct > 0).sort((a, b) => b.pct - a.pct)[0];
    const down = off.filter((r) => r.pct < 0).sort((a, b) => a.pct - b.pct)[0];
    insight =
      up || down ? (
        <>
          {up && (
            <>
              <strong>{nameOf(up.id)}</strong> ended {up.pct}% above its 6-month average.{" "}
            </>
          )}
          {down && (
            <>
              <strong>{nameOf(down.id)}</strong> ended {-down.pct}% below.
            </>
          )}
        </>
      ) : (
        "Every category landed within 10% (or R$ 50) of its 6-month average."
      );
  }

  return (
    <Panel title="Spending by category" footer="Card purchases count in the month of each installment. Refunds come off their category.">
      <div className="flex flex-col gap-4">
        <p className="m-0 type-body">{insight}</p>
        <div role="list" className="flex flex-col gap-2.5">
          {rows.map((r) => {
            const c = r.id ? byId.get(r.id) : undefined;
            return (
              <Link
                key={r.id ?? "none"}
                role="listitem"
                to={`/transactions?month=${v.month}&category=${r.id ?? "none"}`}
                aria-label={`${nameOf(r.id)}: ${formatBRL(r.spent)}, 6-month average ${formatBRL(r.avg)}`}
                className="grid grid-cols-[112px_minmax(0,1fr)_96px_40px] items-center gap-3 text-ink no-underline hover:text-ink max-sm:grid-cols-[96px_minmax(0,1fr)_88px]"
              >
                {c ? (
                  <CategoryTag name={c.name} color={c.color} />
                ) : (
                  <span className="inline-flex items-center gap-1 text-[13px] font-semibold text-warning">
                    <TriangleAlert className="size-3.5" aria-hidden /> To review
                  </span>
                )}
                <span className="relative h-3 rounded-xs bg-paper-sunken">
                  <span
                    className="absolute inset-y-0 left-0 rounded-xs"
                    style={{
                      width: `${(Math.max(r.spent, 0) / max) * 100}%`,
                      // Waiting for a category: hatched, so it never reads as one.
                      background: c ? `var(--${c.color})` : "repeating-linear-gradient(135deg, var(--warning) 0 2px, transparent 2px 5px)",
                      boxShadow: c ? undefined : "inset 0 0 0 1px var(--warning)",
                    }}
                  />
                  {r.avg > 0 && <span title="6-month average" className="absolute -inset-y-1 w-0.5 bg-ink" style={{ left: `${(r.avg / max) * 100}%` }} />}
                </span>
                <Amount cents={r.spent} sign="none" size="sm" className="text-right" />
                <span className="text-right type-figure-sm text-ink-muted max-sm:hidden">{total > 0 ? `${Math.round((Math.max(r.spent, 0) / total) * 100)}%` : ""}</span>
              </Link>
            );
          })}
        </div>
        <div className="flex flex-wrap gap-x-4 type-caption text-ink-muted">
          <Legend color="var(--ink-muted)">{formatMonth(v.month).split(" ")[0]}{isCurrent ? ", to date" : ""}</Legend>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-3 w-0.5 bg-ink" aria-hidden />
            6-month average
          </span>
        </div>
      </div>
    </Panel>
  );
}

function Through({ month, days, today, isCurrent }: { month: string; days: DaysView; today: string; isCurrent: boolean }) {
  const wide = useWide();
  const prevName = formatMonth(prevOf(month)).split(" ")[0];
  const diff = days.summary.spentCents - days.summary.previousToDateCents;
  return (
    <div className="flex flex-col gap-3">
      <p className="m-0 type-body">
        {days.summary.spentCents === 0 && days.summary.previousToDateCents === 0 ? (
          "No spending in this month or the one before."
        ) : days.summary.previousToDateCents === 0 ? (
          "Nothing to compare with in the month before."
        ) : (
          <>
            You have spent{" "}
            <strong className={diff <= 0 ? "text-inflow" : "text-warning"}>
              {formatBRL(Math.abs(diff))} {diff <= 0 ? "less" : "more"}
            </strong>{" "}
            than {isCurrent ? "at this point in" : "in"} {prevName}.
          </>
        )}
      </p>
      {(days.summary.spentCents > 0 || days.previous.some((d) => d.spentCents > 0)) && (
        <RunningTotal month={month} days={days.days} previous={days.previous} today={today} width={wide ? 560 : 340} />
      )}
      <div className="flex flex-wrap gap-x-4 type-caption text-ink-muted">
        <span className="inline-flex items-center gap-1.5">
          <span className="h-0.5 w-4 bg-ink" aria-hidden />
          {formatMonth(month).split(" ")[0]}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="w-4 border-t-2 border-dashed border-rule-strong" aria-hidden />
          {prevName}
        </span>
      </div>
    </div>
  );
}

function Recent({ list, byId, month }: { list?: TransactionList; byId: Map<string, Category>; month: string }) {
  const rows = (list?.transactions ?? []).slice(0, 7);
  const pending = list?.summary.pending ?? 0;
  return (
    <Panel
      title="Recent transactions"
      bodyClassName="-mx-5"
      footer={pending ? `${pending} pending ${pending === 1 ? "transaction" : "transactions"} may still change on the next sync.` : undefined}
    >
      {!list && <p className="m-0 px-5 type-body text-ink-muted">Loading…</p>}
      {list && rows.length === 0 && <p className="m-0 px-5 type-body text-ink-muted">Nothing this month yet.</p>}
      <div className="border-t border-rule">
        {rows.map((t) => (
          <TransactionRow
            key={t.id}
            to={`/transactions?month=${month}&tx=${t.id}`}
            date={formatDay(t.date)}
            description={t.description}
            meta={meta(t)}
            kind={t.kind}
            amountCents={t.amountCents}
            pending={t.status === "pending"}
            installment={t.installment ? `${t.installment.number} of ${t.installment.total}` : undefined}
            category={t.category && byId.get(t.category.id) ? { name: byId.get(t.category.id)!.name, color: byId.get(t.category.id)!.color } : null}
          />
        ))}
      </div>
      {list && list.summary.count > rows.length && (
        <Link to={`/transactions?month=${month}`} className="mx-5 mt-3 inline-block text-[14px] font-semibold">
          See all {list.summary.count} transactions
        </Link>
      )}
    </Panel>
  );
}

function meta(t: Transaction): string {
  if (t.counterpart) return `${t.account.institution} ${typeLabel[t.account.type]} ${t.amountCents < 0 ? "→" : "←"} ${t.counterpart.institution} ${typeLabel[t.counterpart.type]}`;
  return `${t.account.institution} · ${typeLabel[t.account.type]}`;
}

function ToReview({ list, byId, monthName }: { list?: TransactionList; byId: Map<string, Category>; monthName: string }) {
  const review = useResource<Review>("/api/v1/review").data;
  const wide = useWide();
  const groups = review?.groups.slice(0, 4) ?? [];
  const suggested = review?.groups.filter((g) => g.suggestion).length ?? 0;

  const by = { rule: 0, llm: 0, manual: 0 };
  for (const t of list?.transactions ?? []) {
    const src = t.category?.source as keyof typeof by | undefined;
    if (src && src in by) by[src]++;
  }

  return (
    <Panel title="To review" footer={list ? `${monthName}: ${by.rule} by your rules, ${by.llm} by the local model, ${by.manual} by hand.` : undefined}>
      {!review ? (
        <p className="m-0 type-body text-ink-muted">Loading…</p>
      ) : review.waiting === 0 ? (
        <p className="m-0 flex items-center gap-2 type-body">
          <Check className="size-4 text-inflow" aria-hidden /> Every transaction has a category.
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="m-0 type-body">
            {review.waiting} {review.waiting === 1 ? "transaction needs" : "transactions need"} a category, from {review.groups.length}{" "}
            {review.groups.length === 1 ? "group" : "groups"}, every month.{suggested > 0 && ` The local model has a suggestion for ${suggested} of them.`}
          </p>
          <div className="flex flex-col">
            {groups.map((g) => {
              const s = g.suggestion ? byId.get(g.suggestion.category) : undefined;
              return (
                <div key={(g.pattern?.text ?? "") + g.transactions[0]?.id} className="flex items-center justify-between gap-3 border-t border-rule py-2">
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate font-figure text-[13px] leading-5 font-medium">
                      {g.pattern?.text ?? g.transactions[0]?.description}
                      {g.count > 1 && <span className="text-ink-muted"> · {g.count}×</span>}
                    </span>
                    <span className="inline-flex items-center gap-1 type-caption text-ink-muted">
                      {s ? (
                        <>
                          <Sparkles className="size-3" aria-hidden /> Suggested: {s.name}
                        </>
                      ) : (
                        `latest ${formatDay(g.latest)}`
                      )}
                    </span>
                  </span>
                  <Amount cents={g.totalCents} sign="always" size="sm" />
                </div>
              );
            })}
          </div>
          {review.groups.length > groups.length && (
            <p className="m-0 -mt-2 type-caption text-ink-muted">
              and {review.groups.length - groups.length} more {review.groups.length - groups.length === 1 ? "group" : "groups"} in the queue
            </p>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button asChild variant="primary">
              <Link to="/review">Start review {wide && <Kbd>R</Kbd>}</Link>
            </Button>
            <span className="type-caption text-ink-muted">One choice settles a whole merchant</span>
          </div>
        </div>
      )}
    </Panel>
  );
}

/** The charges that come back every month (F5), with what changed. */
function FixedCosts({ byId }: { byId: Map<string, Category> }) {
  const r = useResource<Recurring>("/api/v1/recurring").data;
  if (!r) return <Panel title="Fixed costs">Loading…</Panel>;
  const changed = r.charges.filter((c) => c.change);
  const fresh = r.charges.filter((c) => c.new);
  const news = [
    ...changed.map((c) => `${titleCase(c.name)} went ${c.change!.toCents > c.change!.fromCents ? "up" : "down"} ${formatBRL(Math.abs(c.change!.toCents - c.change!.fromCents))}`),
    ...fresh.map((c) => `${titleCase(c.name)} is new`),
  ];
  return (
    <Panel title="Fixed costs" footer="Found in charges from the same merchant, on about the same day each month, at a similar amount.">
      {r.charges.length === 0 ? (
        <p className="m-0 type-body text-ink-muted">No charge comes back every month yet. It takes three months of history.</p>
      ) : (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,280px)_minmax(0,1fr)] lg:gap-10">
          <div className="flex flex-col gap-2">
            <span className="flex items-baseline gap-2">
              <Amount cents={r.monthlyCents} size="lg" sign="none" />
              <span className="type-caption text-ink-muted">a month</span>
            </span>
            <p className="m-0 type-body text-ink-muted">
              {r.charges.length} recurring {r.charges.length === 1 ? "charge" : "charges"}.{news.length > 0 && ` ${sentence(news)}.`}
            </p>
            {r.upcomingCents > 0 && (
              <p className="m-0 type-caption text-ink-muted">{formatBRL(r.upcomingCents)} still to come this month.</p>
            )}
          </div>
          <div className="flex flex-col">
            {r.charges.map((c) => {
              const cat = c.category ? byId.get(c.category) : undefined;
              return (
                <div key={c.name} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-rule py-2 first:border-t-0">
                  <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
                    <span className="truncate text-[14px] leading-5 font-semibold">{titleCase(c.name)}</span>
                    <span className="type-caption text-ink-muted">
                      {c.paidThisMonth ? `paid, next ${formatDay(c.next)}` : `due ${formatDay(c.next)}`}
                      {c.steadiness === "varies" && " · varies"}
                    </span>
                    {cat && <CategoryTag name={cat.name} color={cat.color} />}
                    {c.change && (
                      <Badge tone={c.change.toCents > c.change.fromCents ? "warning" : "inflow"}>
                        {c.change.toCents > c.change.fromCents ? "+" : "−"}
                        {formatBRL(Math.abs(c.change.toCents - c.change.fromCents))} since {formatDay(c.change.since)}
                      </Badge>
                    )}
                    {c.new && <Badge tone="tinta">New</Badge>}
                  </span>
                  <Amount cents={c.monthlyCents} sign="none" estimate={c.steadiness === "varies"} />
                </div>
              );
            })}
          </div>
        </div>
      )}
    </Panel>
  );
}

function sentence(parts: string[]): string {
  if (parts.length <= 1) return parts.join("");
  return parts.slice(0, -1).join(", ") + " and " + parts.at(-1);
}

/** How many days a month ("2026-09") has. */
function monthDays(m: string): number {
  const [y, mo] = m.split("-").map(Number);
  return new Date(y, mo, 0).getDate();
}
