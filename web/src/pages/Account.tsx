import { useMemo, useRef, useState, type ReactNode } from "react";
import { ArrowLeftRight, CreditCard, List, TrendingUp, Wallet, type LucideIcon } from "lucide-react";

import { Amount } from "@/components/Amount";
import { BalanceChart, type BalancePoint } from "@/components/charts/BalanceChart";
import { CardMonthBars } from "@/components/charts/CardMonthBars";
import { MonthBars } from "@/components/charts/MonthBars";
import { InstallmentProgress } from "@/components/InstallmentProgress";
import { LimitBar } from "@/components/LimitBar";
import { BillDetail, BillsList, billName } from "@/components/CardBills";
import { AddTransaction, ManualAccountView } from "@/components/ManualAccount";
import { Notice } from "@/components/Notice";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TransactionRow } from "@/components/TransactionRow";
import { Button } from "@/components/ui/button";
import { Sheet } from "@/components/ui/sheet";
import { Skeleton, SkeletonRows, SkeletonStat } from "@/components/ui/skeleton";
import {
  typeLabel,
  typeName,
  useResource,
  type Account,
  type BalanceHistory,
  type CardBill,
  type Flow,
  type MonthView,
  type Purchase,
  type TransactionList,
} from "@/lib/api";
import { addMonths, formatBRL, formatDate, formatDay, formatMonth, todayISO } from "@/lib/format";
import { useCategories } from "@/lib/categories";
import { focusBack } from "@/lib/focus";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { pickNotice } from "@/lib/notice";
import { Link, navigate } from "@/lib/router";
import { useBanks, type Bank } from "@/shell/Accounts";
import { PageHeader } from "@/shell/AppShell";

/** The account page (canvas: "Account (checking and card)" and "Account, phone"). */
export function AccountPage({ id }: { id: string }) {
  const { banks, loading } = useBanks();
  const { month } = useMonth();
  const all = banks.flatMap((b) => b.accounts.map((a) => ({ a, bank: b })));
  const found = all.find((x) => x.a.id === id);
  if (!found) {
    if (loading)
      return (
        <>
          <Skeleton className="h-8 w-56" />
          <Panel>
            <div className="flex flex-col gap-4">
              <SkeletonStat hero />
              <Skeleton className="h-40" />
            </div>
          </Panel>
        </>
      );
    return <PageHeader title="No such account" caption="It may have been removed from Meu Pluggy." />;
  }
  const { a, bank } = found;
  if (a.manual) {
    return (
      <>
        <PageHeader
          title={a.name}
          caption={`${typeName[a.type]} · outside Open Finance, typed in by you`}
          monthless={a.type === "asset"}
          actions={
            a.type === "cash" ? (
              <Button asChild variant="secondary">
                <Link to={`/transactions?month=${month}&account=${a.id}`}>
                  <List aria-hidden />
                  See its transactions
                </Link>
              </Button>
            ) : undefined
          }
        />
        <ManualAccountView a={a} />
        {a.type === "cash" && <AccountTransactions a={a} />}
      </>
    );
  }
  const notice = pickNotice([bank]);
  const synced = bank.connection ? bank.connection.detail : "Not synced yet";
  return (
    <>
      <PageHeader
        title={`${a.institution} ${typeLabel[a.type]}`}
        caption={[typeName[a.type], a.number && `•••• ${a.number}`, synced].filter(Boolean).join(" · ")}
        actions={
          <Button asChild variant="secondary">
            <Link to={`/transactions?month=${month}&account=${a.id}`}>
              <List aria-hidden />
              See its transactions
            </Link>
          </Button>
        }
      />
      {all.length > 1 && (
        // Many accounts scroll sideways instead of widening the page.
        <div
          className="-mx-4 overflow-x-auto px-4 lg:hidden"
          ref={(el) => el?.querySelector<HTMLElement>('[data-state="on"], [aria-checked="true"]')?.scrollIntoView({ block: "nearest", inline: "center" })}
        >
          <SegmentedControl
            label="Account"
            className="w-max [&_button]:whitespace-nowrap"
            options={all.map((x) => ({ value: x.a.id, label: shortName(x.a, all.map((y) => y.a)) }))}
            value={a.id}
            onChange={(v) => navigate(`/accounts/${v}${location.search}`)}
          />
        </div>
      )}
      {notice && (
        <Notice tone={notice.tone} title={notice.title}>
          {notice.text}
        </Notice>
      )}
      {a.type === "credit_card" ? <CardView key={a.id} a={a} bank={bank} /> : <CheckingView a={a} bank={bank} />}
      <AddTransaction a={a} missing />
      <AccountTransactions a={a} />
    </>
  );
}

/** "Checking", "Card", or with the bank when there is more than one. */
function shortName(a: Account, all: Account[]): string {
  if (a.manual) return a.name;
  const kind = a.type === "credit_card" ? "Card" : typeName[a.type];
  // With more than one bank, every account says which.
  return new Set(all.filter((x) => !x.manual).map((x) => x.institution)).size > 1 ? `${a.institution} ${kind.toLowerCase()}` : kind;
}

type Range = "3m" | "6m" | "12m" | "all";
const ranges: { value: Range; label: string }[] = [
  { value: "3m", label: "3M" },
  { value: "6m", label: "6M" },
  { value: "12m", label: "12M" },
  { value: "all", label: "All" },
];
const rangeName: Record<Range, string> = { "3m": "3 months", "6m": "6 months", "12m": "12 months", all: "" };

function CheckingView({ a, bank }: { a: Account; bank: Bank }) {
  const history = useResource<BalanceHistory>(`/api/v1/accounts/${a.id}/balance`);
  const months = useResource<{ months: MonthView[] }>(`/api/v1/accounts/${a.id}/months`);
  const { month, setMonth } = useMonth();
  const [range, setRange] = useState<Range>("12m");
  const [hover, setHover] = useState<BalancePoint | null>(null);

  const all = history.data?.points ?? [];
  const since = all[0]?.date;
  const points = useMemo(() => {
    const pts = all.map((p) => ({ date: p.date, cents: p.balanceCents }));
    if (range === "all" || pts.length === 0) return pts;
    const days = { "3m": 91, "6m": 182, "12m": 365 }[range];
    return pts.slice(Math.max(0, pts.length - days));
  }, [all, range]);
  // A checking account here cannot stay below zero: a rebuilt line that does
  // means Pluggy is missing a movement, so it is read as an estimate.
  const impossible = a.type !== "credit_card" && all.some((p) => p.balanceCents < 0);
  const first = points[0]?.cents ?? 0;
  const last = points[points.length - 1]?.cents ?? a.balanceCents;
  const over = range === "all" ? (since ? `since ${formatDate(since)}` : "") : `over ${rangeName[range]}`;
  const diff = last - first;

  const past = (months.data?.months ?? []).filter((m) => !m.future).slice(-12);
  const sel = past.find((m) => m.month === month) ?? past[past.length - 1];

  return (
    <>
      <div className="flex flex-wrap items-stretch gap-4 lg:gap-6">
        <Panel
          title="Balance"
          className="flex-[8_1_560px]"
          footer={
            impossible
              ? `Rebuilt from today’s balance and every transaction${since ? ` since ${formatDate(since)}` : ""}. It goes below zero, which this account cannot: Pluggy is missing a movement, so read the line as an estimate. Compare it with the bank’s statement.`
              : `Rebuilt from today’s balance and every transaction${since ? ` since ${formatDate(since)}` : ""}. Hover the line, or focus it and use the arrow keys, for a day.`
          }
        >
          <div className="flex flex-col gap-5">
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div className="flex flex-col gap-1">
                <Amount cents={hover?.cents ?? a.balanceCents} size="hero" estimate={Boolean(hover) && impossible} />
                <span className="type-caption text-ink-muted">{hover ? `End of ${formatDate(hover.date)}` : "Today, as the bank reported it"}</span>
              </div>
              <SegmentedControl label="Range" options={ranges} value={range} onChange={setRange} />
            </div>
            {points.length > 1 && (
              <p className="m-0 type-body">
                {diff === 0 ? "No change" : `${diff > 0 ? "Up" : "Down"} ${formatBRL(Math.abs(diff))}`} {over}.
              </p>
            )}
            {history.error && <p className="m-0 type-body text-ink-muted">Could not load the balance history: {history.error}.</p>}
            {points.length > 0 && (
              <BalanceChart points={points} label={`Balance ${over}, from ${formatBRL(first)} to ${formatBRL(last)}`} onHover={setHover} />
            )}
          </div>
        </Panel>
        <Facts a={a} bank={bank} since={since} className="flex-[3_1_260px]" />
      </div>

      <div className="flex flex-wrap items-stretch gap-4 lg:gap-6">
        <Panel
          title="Money in and out, by month"
          className="flex-[7_1_520px]"
          footer="Transfers between your accounts and card bills are included: this is the account, not your spending. Select a month for its numbers."
        >
          {past.length === 0 ? (
            months.loading ? <Skeleton className="h-40" /> : <p className="m-0 type-body text-ink-muted">No months yet.</p>
          ) : (
            <div className="flex flex-col gap-4">
              <MonthBars months={past} selected={sel?.month ?? ""} onSelect={setMonth} />
              {sel && (
                <div className="flex flex-wrap items-baseline gap-x-5 gap-y-1.5 border-t border-rule pt-3 text-[14px] leading-5">
                  <strong className="font-semibold">{formatMonth(sel.month)}</strong>
                  <span className="inline-flex items-baseline gap-1.5 text-ink-muted">
                    In <Amount cents={sel.inCents} sign="always" />
                  </span>
                  <span className="inline-flex items-baseline gap-1.5 text-ink-muted">
                    Out <Amount cents={sel.outCents} />
                  </span>
                  <span className="inline-flex items-baseline gap-1.5 text-ink-muted">
                    Net <Amount cents={sel.inCents + sel.outCents} sign="always" />
                  </span>
                  <Link to={`/transactions?month=${sel.month}&account=${a.id}`} className="font-semibold lg:ml-auto">
                    Open {formatMonth(sel.month).split(" ")[0]} in Transactions
                  </Link>
                </div>
              )}
              <div className="flex flex-wrap gap-4 type-caption text-ink-muted">
                <Swatch className="bg-inflow">In</Swatch>
                <Swatch className="bg-ink">Out</Swatch>
              </div>
            </div>
          )}
        </Panel>
        <Panel title={sel ? `Where ${formatMonth(sel.month).split(" ")[0]} went` : "Where the money went"} className="flex-[5_1_340px]" footer="Only spending counts toward the budget.">
          {sel ? <Flows month={sel} /> : <p className="m-0 type-body text-ink-muted">Nothing yet.</p>}
        </Panel>
      </div>
    </>
  );
}

function Swatch({ className, children }: { className: string; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`box-border size-2.5 rounded-xs ${className}`} aria-hidden />
      {children}
    </span>
  );
}

/** A month's flows, largest first, each with a bar against the largest. */
function Flows({ month }: { month: MonthView }) {
  const rows = month.flows
    .map((f) => ({ f, ...flowLabel(f) }))
    .sort((x, y) => Math.abs(y.f.cents) - Math.abs(x.f.cents));
  const max = Math.max(1, ...rows.map((r) => Math.abs(r.f.cents)));
  return (
    <div className="flex flex-col gap-3">
      {rows.map(({ f, label, icon: Icon, counts }) => (
        <div key={`${f.kind}-${f.direction}-${f.counterpart?.id ?? ""}`} className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-1">
          <span className="inline-flex min-w-0 items-center gap-2 text-[14px] leading-5">
            <Icon className="size-3.5 flex-none text-ink-muted" aria-hidden />
            <span className="truncate">{label}</span>
          </span>
          <Amount cents={f.cents} sign={counts ? "always" : "none"} tone={counts ? "auto" : "muted"} />
          <div className="col-span-2 h-1.5 rounded-xs bg-paper-sunken">
            <div
              className={`h-full rounded-xs ${!counts ? "bg-rule-strong" : f.direction === "in" ? "bg-inflow" : "bg-ink"}`}
              style={{ width: `${(Math.abs(f.cents) / max) * 100}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function flowLabel(f: Flow): { label: string; icon: LucideIcon; counts: boolean } {
  const other = f.counterpart ? `${f.counterpart.institution} ${typeLabel[f.counterpart.type]}` : "";
  switch (f.kind) {
    case "income":
      return f.direction === "in" ? { label: "Income", icon: TrendingUp, counts: true } : { label: "Income returned", icon: TrendingUp, counts: true };
    case "expense":
      return f.direction === "out" ? { label: "Spending", icon: Wallet, counts: true } : { label: "Refunds", icon: Wallet, counts: true };
    case "bill_payment":
      return f.direction === "out"
        ? { label: other ? `Card bill, ${other}` : "Card bill", icon: CreditCard, counts: false }
        : { label: other ? `Bill paid from ${other}` : "Bill paid", icon: CreditCard, counts: false };
    default:
      if (other) return { label: f.direction === "out" ? `To ${other}` : `From ${other}`, icon: ArrowLeftRight, counts: false };
      return { label: f.direction === "out" ? "To investments and other transfers" : "From investments and other transfers", icon: ArrowLeftRight, counts: false };
  }
}

function Facts({ a, bank, since, className }: { a: Account; bank: Bank; since?: string; className?: string }) {
  const at = new Date(a.balanceAt);
  const facts: [string, string][] = [
    ["Bank", a.institution],
    ["Type", typeName[a.type]],
  ];
  if (a.number) facts.push(["Number", `•••• ${a.number}`]);
  if (a.card?.limitCents != null) facts.push(["Limit", formatBRL(a.card.limitCents)]);
  if (a.card?.dueDate) facts.push([a.card.dueDate < todayISO() ? "Last bill due" : "Bill due", formatDate(a.card.dueDate)]);
  if (a.card?.closeDate) facts.push(["Bill closes", formatDate(a.card.closeDate)]);
  facts.push(["Balance as of", `${formatDay(todayISO(at))}, ${at.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}`]);
  if (since) facts.push(["History since", formatDate(since)]);
  facts.push(["Connection", bank.connection ? "Meu Pluggy" : "Not synced yet"]);
  if (bank.connection?.consentExpiresAt) facts.push(["Authorized until", formatDate(todayISO(new Date(bank.connection.consentExpiresAt)))]);
  return (
    <Panel title={a.type === "credit_card" ? "This card" : "This account"} className={className}>
      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2.5 text-[14px] leading-5">
        {facts.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-ink-muted">{k}</dt>
            <dd className="m-0 text-right font-semibold">{v}</dd>
          </div>
        ))}
      </dl>
    </Panel>
  );
}

function CardView({ a, bank }: { a: Account; bank: Bank }) {
  const months = useResource<{ months: MonthView[] }>(`/api/v1/accounts/${a.id}/months`);
  const purchases = useResource<{ purchases: Purchase[] }>(`/api/v1/accounts/${a.id}/installments`);
  const billsRes = useResource<{ bills: CardBill[] }>(`/api/v1/accounts/${a.id}/bills`);
  const { current } = useMonth();
  const { byId } = useCategories();
  const wide = useWide();
  const today = todayISO();
  // On a wide screen the open bill starts selected beside the list; on a
  // phone nothing is, until a bill opens in the sheet.
  const [picked, setPicked] = useState<string | null>(null);
  // The bill the sheet came from, kept while it slides away.
  const fromBill = useRef<string | null>(null);
  if (picked) fromBill.current = picked;

  const limit = a.card?.limitCents;
  const available = a.card?.availableCents;
  const all = months.data?.months ?? [];
  // Nubank counts future installments against the limit: what it reports as
  // the card's balance is the limit in use, not the open bill.
  const inUse = limit != null && available != null ? limit - available : -a.balanceCents;
  const live = purchases.data?.purchases ?? [];
  const left = live.reduce((s, p) => s + p.remainingCents, 0);
  // Installments not billed yet: the months ahead when the bank lists them,
  // else what the purchases in progress still have to bill.
  const futureMonths = all.filter((m) => m.future).reduce((s, m) => s + m.spentCents, 0);
  const owed = Math.max(futureMonths, left);
  // The bank's limit in use normally includes the installments to come; when
  // it is less than what they still owe, the bank is not counting them yet,
  // and the two figures are shown apart instead of one capping the other.
  const counted = owed <= inUse;
  const future = counted ? owed : 0;
  const shown = [...all.filter((m) => !m.future).slice(-12), ...all.filter((m) => m.future)];
  const past = all.filter((m) => !m.future && m.month < current);
  const avg = past.length ? Math.round(past.slice(-11).reduce((s, m) => s + m.spentCents, 0) / Math.min(11, past.length)) : 0;
  const now = all.find((m) => m.month === current);
  const next = all.find((m) => m.month === addMonths(current, 1));

  const bills = billsRes.data?.bills ?? [];
  const open = bills.find((b) => b.state === "open");
  const selectedId = picked ?? (wide ? "open" : null);
  const selected = bills.find((b) => b.id === selectedId);
  const detail = selected && <BillDetail key={selected.id} b={selected} a={a} byId={byId} today={today} />;

  return (
    <>
      <div className="flex flex-wrap items-stretch gap-4 lg:gap-6">
        <Panel
          title={open ? `${billName(open, today)}, open` : "In use on the card"}
          className="flex-[8_1_560px]"
          footer={
            open
              ? `As ${a.institution} reports it through Meu Pluggy. Pending purchases may still change until they post.`
              : "As the bank reports it through Meu Pluggy: unpaid bills, this bill’s purchases and the installments still to come."
          }
        >
          <div className="flex flex-col gap-5">
            {open ? (
              <>
                <div className="flex flex-col gap-1">
                  <Amount cents={open.totalCents} size="hero" sign="none" />
                  <span className="type-caption text-ink-muted">
                    {open.dueDate && `Due on ${formatDay(open.dueDate)} · `}
                    {open.transactions.length} {open.transactions.length === 1 ? "transaction" : "transactions"} so far. New purchases go into it until {a.institution} closes it.
                  </span>
                </div>
                <dl className="m-0 grid grid-cols-3 gap-x-6 gap-y-3">
                  {[
                    { label: `Purchases (${open.purchases.count})`, cents: -open.purchases.cents, sign: "none" as const },
                    { label: `Installments (${open.installments.count})`, cents: -open.installments.cents, sign: "none" as const },
                    { label: `Refunds (${open.refunds.count})`, cents: open.refunds.cents, sign: "always" as const },
                  ].map((p) => (
                    <div key={p.label} className="flex min-w-0 flex-col gap-0.5 border-t border-rule pt-2.5">
                      <dt className="type-caption text-ink-muted">{p.label}</dt>
                      <dd className="m-0">
                        <Amount cents={p.cents} sign={p.sign} />
                      </dd>
                    </div>
                  ))}
                </dl>
              </>
            ) : billsRes.loading ? (
              <SkeletonStat hero />
            ) : (
              <div className="flex flex-col gap-1">
                <Amount cents={a.balanceCents} size="hero" />
                <span className="type-caption text-ink-muted">{billDays(a, today)}</span>
              </div>
            )}
            {limit != null && available != null ? (
              <div className="flex flex-col gap-2">
                <p className="m-0 type-body">
                  <strong className="font-semibold">{formatBRL(inUse)}</strong> of your {formatBRL(limit)} limit is in use
                  {future > 0 ? `, ${formatBRL(future)} of it in installments not billed yet.` : "."}
                </p>
                {!counted && owed > 0 && (
                  <p className="m-0 type-caption text-ink-muted">
                    Installments still owe {formatBRL(owed)}, more than the limit in use: the bank takes them from the limit only as each one is billed.
                  </p>
                )}
                <LimitBar limitCents={limit} restCents={Math.max(0, inUse - future)} futureCents={future} availableCents={available} />
              </div>
            ) : (
              <p className="m-0 type-body text-ink-muted">The bank does not report this card’s limit.</p>
            )}
          </div>
        </Panel>
        <Facts a={a} bank={bank} className="flex-[3_1_260px]" />
      </div>

      <div className="flex flex-wrap items-start gap-4 lg:gap-6">
        <Panel
          title="Bills"
          className="flex-[7_1_520px]"
          bodyClassName="-mx-5"
          footer="Closed bills as the bank reports them, each payment paired with the account it came from. A bill is named for the month it is due. Select one for what is in it."
        >
          {billsRes.error && !billsRes.data ? (
            <p className="m-0 px-5 type-body text-ink-muted">Could not load the bills: {billsRes.error}.</p>
          ) : !billsRes.data ? (
            <SkeletonRows count={5} className="border-t border-rule" />
          ) : (
            <BillsList bills={bills} selected={selectedId} onSelect={setPicked} today={today} />
          )}
        </Panel>
        {wide && selected && (
          <Panel title={billName(selected, today)} className="flex-[5_1_340px]">
            {detail}
          </Panel>
        )}
        {!wide && (
          <Sheet
            open={Boolean(picked && selected)}
            onOpenChange={(o) => !o && setPicked(null)}
            label={selected ? billName(selected, today) : "Bill"}
            onCloseAutoFocus={(e) => {
              // Back to the bill it came from, which a tap may never have focused.
              e.preventDefault();
              focusBack(`[data-bill="${fromBill.current}"]`);
            }}
          >
            {detail}
          </Sheet>
        )}
      </div>

      <Panel
        title="Card spending by month"
        footer={`Each installment counts in the month it is billed.${futureMonths > 0 ? " Dashed bars are installments already committed to the coming months." : ""}`}
      >
        <div className="flex flex-col gap-3">
          {now && (
            <p className="m-0 type-body">
              {formatMonth(current).split(" ")[0]} so far is {formatBRL(now.spentCents)}
              {avg > 0 && `, against an average of ${formatBRL(avg)} a month`}.
              {next && next.spentCents > 0 && ` ${formatBRL(next.spentCents)} is already committed for ${formatMonth(next.month).split(" ")[0]} in installments.`}
            </p>
          )}
          {shown.length > 0 ? <CardMonthBars months={shown} current={current} /> : months.loading ? <Skeleton className="h-[200px]" /> : <p className="m-0 type-body text-ink-muted">No months yet.</p>}
        </div>
      </Panel>

      <Panel
        title="Installments in progress"
        bodyClassName="-mx-5"
        footer={live.length ? `${formatBRL(left)} still to pay in ${live.length} ${live.length === 1 ? "purchase" : "purchases"}. Pluggy has no id for a purchase: installments with the same description, count and amount are put together.` : undefined}
      >
        {live.length === 0 ? (
          purchases.loading ? <SkeletonRows count={3} className="border-t border-rule" /> : <p className="m-0 px-5 type-body text-ink-muted">No installments in progress.</p>
        ) : (
          <div>
            <div className="hidden grid-cols-[minmax(0,1fr)_150px_96px_104px_72px] gap-3 border-y border-rule bg-paper-sunken px-5 py-2 type-label text-ink-muted lg:grid">
              <span>Purchase</span>
              <span>Progress</span>
              <span className="text-right">Per month</span>
              <span className="text-right">Still to pay</span>
              <span className="text-right">Last</span>
            </div>
            {live.map((p) => (
              <Link
                key={p.installments[0].id}
                to={`/transactions?month=${p.installments.find((i) => i.number === p.current)?.date.slice(0, 7) ?? current}&account=${a.id}&tx=${(p.installments.find((i) => i.number === p.current) ?? p.installments[0]).id}`}
                className="grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 border-b border-rule px-5 py-2.5 text-ink no-underline hover:bg-paper-sunken hover:text-ink lg:grid-cols-[minmax(0,1fr)_150px_96px_104px_72px] lg:py-2"
              >
                <span className="flex min-w-0 flex-col">
                  <span className="truncate type-body-strong">{p.description}</span>
                  <span className="type-caption text-ink-muted">{formatBRL(p.amountCents * p.total)} in all</span>
                </span>
                <span className="flex flex-col gap-1 max-lg:col-span-2 max-lg:row-start-2">
                  <InstallmentProgress current={p.current} total={p.total} />
                  <span className="type-figure-sm text-ink-muted">
                    {p.current} of {p.total}
                    <span className="lg:hidden">
                      {" "}· {formatBRL(p.remainingCents)} still to pay · last in {formatMonth(p.lastDate.slice(0, 7), true)}
                    </span>
                  </span>
                </span>
                <Amount cents={p.amountCents} sign="none" className="text-right max-lg:row-start-1 max-lg:col-start-2" />
                <Amount cents={p.remainingCents} sign="none" className="text-right max-lg:hidden" />
                <span className="text-right type-figure-sm text-ink-muted max-lg:hidden">{formatMonth(p.lastDate.slice(0, 7), true)}</span>
              </Link>
            ))}
          </div>
        )}
      </Panel>
    </>
  );
}

/**
 * The bill's days as the bank reports them. A due day already past is the
 * last bill's: Nubank keeps reporting it until the next one closes.
 */
function billDays(a: Account, today: string): string {
  const close = a.card?.closeDate;
  const due = a.card?.dueDate;
  const out: string[] = [];
  if (close) out.push(close < today ? `The last bill closed on ${formatDay(close)}.` : `The bill closes on ${formatDay(close)}.`);
  if (due) out.push(due < today ? `The last bill was due on ${formatDay(due)}.` : `The bill is due on ${formatDay(due)}.`);
  return out.join(" ") || "The bank does not report the bill’s days.";
}

/** The month in view, in this account; a row opens its detail on Transactions. */
function AccountTransactions({ a }: { a: Account }) {
  const { month } = useMonth();
  const { byId } = useCategories();
  const res = useResource<TransactionList>(`/api/v1/transactions?month=${month}&account=${a.id}`);
  const list = res.data?.transactions ?? [];
  const shown = list.slice(0, 8);
  const name = a.manual ? a.name : `${a.institution} ${typeLabel[a.type]}`;
  return (
    <Panel
      title={`${formatMonth(month).split(" ")[0]} in ${a.type === "credit_card" ? `the ${name}` : name}`}
      bodyClassName="-mx-5 border-t border-rule"
      footer={
        list.length > shown.length ? (
          <Link to={`/transactions?month=${month}&account=${a.id}`} className="font-semibold">
            See all {list.length}
          </Link>
        ) : (
          "Select a transaction to open it."
        )
      }
    >
      {res.error && <p className="m-0 px-5 py-6 type-body text-ink-muted">Could not load transactions: {res.error}.</p>}
      {!res.data && !res.error && <SkeletonRows count={6} />}
      {res.data && list.length === 0 && <p className="m-0 px-5 py-6 type-body text-ink-muted">Nothing in {formatMonth(month).split(" ")[0]}.</p>}
      {shown.map((t) => (
        <TransactionRow
            key={t.id}
            to={`/transactions?month=${month}&account=${a.id}&tx=${t.id}`}
            date={formatDay(t.date)}
            description={t.description}
            meta={
              t.counterpart
                ? `${t.amountCents < 0 ? "To" : "From"} ${t.counterpart.institution} ${typeLabel[t.counterpart.type]}`
                : t.manual && t.account.type !== "cash"
                  ? "Added by you"
                  : undefined
            }
            kind={t.kind}
            amountCents={t.amountCents}
            pending={t.status === "pending"}
            installment={t.installment ? `${t.installment.number} of ${t.installment.total}` : undefined}
            category={t.category && byId.get(t.category.id) ? { name: byId.get(t.category.id)!.name, color: byId.get(t.category.id)!.color } : null}
          />
      ))}
    </Panel>
  );
}
