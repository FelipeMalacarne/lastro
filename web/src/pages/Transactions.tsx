import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ChevronDown, Search } from "lucide-react";

import { Notice } from "@/components/Notice";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { TransactionRow } from "@/components/TransactionRow";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { typeLabel, useResource, type Transaction, type TransactionList } from "@/lib/api";
import { dayLabel, formatBRL, formatDay, formatMonth, todayISO } from "@/lib/format";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { pickNotice } from "@/lib/notice";
import { setQuery, useLocation } from "@/lib/router";
import { AccountsPanel, useBanks } from "@/shell/Accounts";
import { PageHeader } from "@/shell/AppShell";

type Flow = "all" | "in" | "out" | "transfer";

const flowsWide: { value: Flow; label: string }[] = [
  { value: "all", label: "All" },
  { value: "in", label: "Inflows" },
  { value: "out", label: "Outflows" },
  { value: "transfer", label: "Transfers" },
];
const flowsNarrow = flowsWide.map((f) => ({ ...f, label: f.value === "in" ? "In" : f.value === "out" ? "Out" : f.label }));

/** The Transactions screen (canvas: "Transactions, M1" and "Transactions, phone"). */
export function Transactions() {
  const { month } = useMonth();
  const { query } = useLocation();
  const { banks } = useBanks();
  const wide = useWide();
  const flow = (["in", "out", "transfer"].includes(query.get("flow") ?? "") ? query.get("flow") : "all") as Flow;
  const account = query.get("account") ?? "";
  const q = query.get("q") ?? "";

  // Typing updates the field at once and the URL (and the request) a beat later.
  const [search, setSearch] = useState(q);
  useEffect(() => setSearch(q), [q]);
  useEffect(() => {
    if (search.trim() === q) return;
    const t = setTimeout(() => setQuery("q", search.trim() || null), 250);
    return () => clearTimeout(t);
  }, [search, q]);

  const url = useMemo(() => {
    const p = new URLSearchParams({ month });
    if (account) p.set("account", account);
    if (flow !== "all") p.set("flow", flow);
    if (q) p.set("q", q);
    return "/api/v1/transactions?" + p.toString();
  }, [month, account, flow, q]);
  const res = useResource<TransactionList>(url);

  // While a new filter loads, keep showing the last list instead of flashing empty.
  const last = useRef<TransactionList | undefined>(undefined);
  if (res.data) last.current = res.data;
  const list = res.data ?? last.current;

  const filtered = Boolean(account || flow !== "all" || q);
  const monthName = formatMonth(month).split(" ")[0];
  const caption = list
    ? `${list.summary.count} ${filtered ? "found" : ""} in ${monthName}`.replace("  ", " ") +
      (list.summary.pending ? ` · ${list.summary.pending} pending` : "")
    : undefined;

  const notice = pickNotice(banks);
  const accounts = banks.flatMap((b) => b.accounts);
  const current = accounts.find((a) => a.id === account);
  const noAccounts = !res.loading && accounts.length === 0 && banks.length === 0;

  return (
    <>
      <PageHeader title="Transactions" caption={caption} />

      {notice && (
        <Notice
          tone={notice.tone}
          title={notice.title}
          action={
            notice.action && (
              <Button asChild variant="ghost" size="sm" className="max-lg:hidden">
                <a href={notice.action.href} target="_blank" rel="noreferrer">
                  {notice.action.label}
                </a>
              </Button>
            )
          }
        >
          {notice.text}{" "}
          {notice.action && (
            <a href={notice.action.href} target="_blank" rel="noreferrer" className="font-semibold lg:hidden">
              {notice.action.label}
            </a>
          )}
        </Notice>
      )}

      <AccountsPanel banks={banks} />

      <div className="flex flex-col gap-3 lg:flex-row lg:flex-wrap lg:items-end">
        <TextField
          label="Search"
          placeholder="Description, e.g. iFood"
          prefix={<Search className="size-4" aria-hidden />}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => e.key === "Escape" && setSearch("")}
          data-search
          type="search"
          className="lg:min-w-0 lg:flex-[1_1_300px]"
        />
        <div className="flex flex-wrap items-center gap-2 lg:items-end lg:gap-3">
          <div className="hidden flex-col gap-1.5 lg:flex">
            <span className="text-[13px] leading-[18px] font-semibold">Type</span>
            <SegmentedControl label="Type" options={flowsWide} value={flow} onChange={(v) => setQuery("flow", v === "all" ? null : v)} />
          </div>
          <SegmentedControl
            label="Type"
            options={flowsNarrow}
            value={flow}
            onChange={(v) => setQuery("flow", v === "all" ? null : v)}
            className="lg:hidden"
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button className="max-lg:h-7 max-lg:px-3 max-lg:text-[13px]">
                {current ? `${current.institution} ${typeLabel[current.type]}` : "All accounts"}
                <ChevronDown aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuRadioGroup value={account} onValueChange={(v) => setQuery("account", v || null)}>
                <DropdownMenuRadioItem value="">All accounts</DropdownMenuRadioItem>
                {accounts.map((a) => (
                  <DropdownMenuRadioItem key={a.id} value={a.id}>
                    {a.institution} {typeLabel[a.type]}
                    {a.number && <span className="ml-auto type-figure-sm text-ink-muted">{a.number}</span>}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <Panel
        title={wide ? "All transactions" : undefined}
        bodyClassName={wide ? "-mx-5 border-t border-rule" : "-mx-5 -mt-5"}
        footer={
          <>
            {list?.truncated && `Showing the newest ${list.transactions.length.toLocaleString("pt-BR")}. `}
            Transfers and card bill payments are listed but never counted as spending.
            <span className="max-lg:hidden"> Categories arrive with the review queue.</span>
          </>
        }
      >
        {res.error && !list && <Empty>Could not load transactions: {res.error}.</Empty>}
        {!list && !res.error && <Empty>Loading transactions…</Empty>}
        {list && list.transactions.length === 0 && (
          <Empty>
            {noAccounts
              ? "No accounts yet. They appear after the first sync, which runs within six hours."
              : q
                ? `Nothing matches “${q}” in ${monthName}.`
                : filtered
                  ? `Nothing like this in ${monthName}.`
                  : `Nothing in ${monthName} yet.`}
          </Empty>
        )}
        {list && <Days transactions={list.transactions} />}
      </Panel>
    </>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="m-0 px-5 py-6 type-body text-ink-muted">{children}</p>;
}

/** The list, grouped by day, each day with what it added up to. */
function Days({ transactions }: { transactions: Transaction[] }) {
  const today = todayISO();
  const days: { date: string; rows: Transaction[] }[] = [];
  for (const t of transactions) {
    const last = days[days.length - 1];
    if (last && last.date === t.date) last.rows.push(t);
    else days.push({ date: t.date, rows: [t] });
  }
  return (
    <>
      {days.map((d) => (
        <div key={d.date}>
          <div className="flex h-8 items-center justify-between gap-3 border-b border-rule bg-paper-sunken px-4 sm:px-5">
            <span className="type-label text-ink-muted">{dayLabel(d.date, today)}</span>
            <span className="type-figure-sm text-ink-muted">{dayTotal(d.rows)}</span>
          </div>
          {d.rows.map((t) => (
            <TransactionRow
              key={t.id}
              date={formatDay(t.date)}
              description={t.description}
              meta={meta(t)}
              kind={t.kind}
              amountCents={t.amountCents}
              pending={t.status === "pending"}
              installment={t.installment ? `${t.installment.number} of ${t.installment.total}` : undefined}
              categorized={false}
            />
          ))}
        </div>
      ))}
    </>
  );
}

/** "R$ 72,00 out · R$ 480,00 in": only what counts. */
function dayTotal(rows: Transaction[]): string {
  let out = 0;
  let inn = 0;
  for (const t of rows) {
    if (t.kind === "transfer" || t.kind === "bill_payment") continue;
    if (t.amountCents < 0) out -= t.amountCents;
    else inn += t.amountCents;
  }
  const parts = [];
  if (out) parts.push(`${formatBRL(out)} out`);
  if (inn) parts.push(`${formatBRL(inn)} in`);
  return parts.join(" · ") || "Nothing counted";
}

/** "Nubank · credit card", or for a pair "Nubank checking → Itaú checking". */
function meta(t: Transaction): string {
  const here = `${t.account.institution} ${typeLabel[t.account.type]}`;
  if (t.counterpart) {
    const there = `${t.counterpart.institution} ${typeLabel[t.counterpart.type]}`;
    return t.amountCents < 0 ? `${here} → ${there}` : `${here} ← ${there}`;
  }
  return `${t.account.institution} · ${typeLabel[t.account.type]}`;
}
