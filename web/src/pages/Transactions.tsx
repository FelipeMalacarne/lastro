import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ChevronDown, Search, X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { CategoryTag } from "@/components/CategoryTag";
import { DayBars, dayValue, type Measure } from "@/components/charts/DayBars";
import { Kbd } from "@/components/Kbd";
import { SyncNotice } from "@/components/SyncNotice";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { Stat } from "@/components/Stat";
import { TextField } from "@/components/TextField";
import { DetailSteps, TransactionDetail } from "@/components/TransactionDetail";
import { TransactionRow } from "@/components/TransactionRow";
import { Button } from "@/components/ui/button";
import { Skeleton, SkeletonRows, SkeletonStat } from "@/components/ui/skeleton";
import { Sheet } from "@/components/ui/sheet";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { typeLabel, useResource, type Category, type DaysView, type TagView, type Transaction, type TransactionList } from "@/lib/api";
import { useCategories, useWaiting } from "@/lib/categories";
import { addMonths, dayLabel, formatBRL, formatDay, formatMonth, formatWeekday, todayISO } from "@/lib/format";
import { useWide } from "@/lib/media";
import { useMonth } from "@/lib/month";
import { Link, hrefWith, navigate, setQuery, useLocation } from "@/lib/router";
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

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

/** The Transactions screen (canvas: "Transactions: month chart and detail", the phone boards and the detail sheet). */
export function Transactions() {
  const { month, current } = useMonth();
  const { query } = useLocation();
  const { banks } = useBanks();
  const wide = useWide();
  const flow = (["in", "out", "transfer"].includes(query.get("flow") ?? "") ? query.get("flow") : "all") as Flow;
  const account = query.get("account") ?? "";
  const category = query.get("category") ?? "";
  const cats = useCategories();
  const waiting = useWaiting();
  const q = query.get("q") ?? "";
  const tag = query.get("tag") ?? "";
  const everyMonth = Boolean(q || tag) && query.get("scope") === "all";
  const tags = useResource<{ tags: TagView[] }>("/api/v1/tags").data?.tags ?? [];
  const asked = query.get("day");
  const day = asked && DAY_RE.test(asked) && asked.startsWith(month + "-") ? asked : null;
  const tx = query.get("tx");

  // Typing updates the field at once and the URL (and the request) a beat later.
  const [search, setSearch] = useState(q);
  useEffect(() => setSearch(q), [q]);
  useEffect(() => {
    if (search.trim() === q) return;
    const t = setTimeout(() => setQuery("q", search.trim() || null), 250);
    return () => clearTimeout(t);
  }, [search, q]);

  // The list and the month's chart read the same filters; only the list narrows to a day.
  const filters = useMemo(() => {
    const p = new URLSearchParams({ month });
    if (account) p.set("account", account);
    if (category) p.set("category", category);
    if (flow !== "all") p.set("flow", flow);
    if (q) p.set("q", q);
    if (tag) p.set("tag", tag);
    return p.toString();
  }, [month, account, category, flow, q, tag]);
  const res = useResource<TransactionList>(`/api/v1/transactions?${filters}${day ? `&day=${day}` : ""}${everyMonth ? "&scope=all" : ""}`);
  const days = useResource<DaysView>(`/api/v1/transactions/days?${filters}`);

  // While a new filter loads, keep showing the last list and month instead of flashing empty.
  const last = useRef<TransactionList | undefined>(undefined);
  if (res.data) last.current = res.data;
  const list = res.data ?? last.current;
  const lastDays = useRef<DaysView | undefined>(undefined);
  if (days.data) lastDays.current = days.data;
  const daysData = days.data ?? (lastDays.current?.month === month ? lastDays.current : undefined);

  // The detail moves through the list: newer above, older below.
  const ids = list?.transactions.map((t) => t.id) ?? [];
  const at = tx ? ids.indexOf(tx) : -1;
  const prev = at > 0 ? ids[at - 1] : undefined;
  const next = at >= 0 && at < ids.length - 1 ? ids[at + 1] : undefined;
  const close = () => setQuery("tx", null);
  useEffect(() => {
    if (!tx) return;
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      // Esc in the phone sheet is the sheet's: it has already closed it.
      if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || t.closest("input, textarea, select, [contenteditable], [role=menu]")) return;
      const to = e.key === "ArrowUp" || e.key === "k" ? prev : e.key === "ArrowDown" || e.key === "j" ? next : undefined;
      if (e.key === "Escape") {
        e.preventDefault();
        setQuery("tx", null);
      } else if (to) {
        e.preventDefault();
        navigate(hrefWith({ tx: to }), { replace: true });
        document.getElementById(`tx-${to}`)?.scrollIntoView({ block: "nearest" });
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [tx, prev, next]);

  // The phone sheet keeps the last transaction while it slides away.
  const inSheet = useRef<string | null>(null);
  if (tx) inSheet.current = tx;

  const byFilters = Boolean(account || category || flow !== "all" || q || tag);
  const filtered = byFilters || Boolean(day);
  const currentCategory = category === "none" ? null : cats.byId.get(category);
  const monthName = formatMonth(month).split(" ")[0];
  const caption = list
    ? everyMonth
      ? `${list.summary.count} ${list.summary.count === 1 ? "match" : "matches"} for ${q ? `“${q}”` : `#${tag}`} in every month · ${formatBRL(-list.summary.outCents)} out` +
        (list.summary.inCents ? ` · ${formatBRL(list.summary.inCents)} in` : "")
      : `${list.summary.count} ${filtered ? "found" : ""} in ${day ? formatDay(day) : monthName}`.replace("  ", " ") +
        (list.summary.pending ? ` · ${list.summary.pending} pending` : "")
    : undefined;

  const accounts = banks.flatMap((b) => b.accounts);
  const currentAccount = accounts.find((a) => a.id === account);
  const noAccounts = !res.loading && accounts.length === 0 && banks.length === 0;
  const following: Following | null = byFilters
    ? {
        category: category === "none" ? "none" : (currentCategory ?? undefined),
        text: [
          currentAccount && `${currentAccount.institution} ${typeLabel[currentAccount.type]}`,
          q && `“${q}”`,
          tag && `#${tag}`,
          flow !== "all" && flowsWide.find((f) => f.value === flow)?.label,
        ]
          .filter(Boolean)
          .join(" · "),
      }
    : null;
  const clearFilters = () => navigate(hrefWith({ q: null, flow: null, account: null, category: null, tag: null, scope: null }), { replace: true });

  return (
    <>
      <PageHeader
        title="Transactions"
        caption={caption}
        monthless={everyMonth}
        actions={
          waiting ? (
            <Button asChild variant="primary">
              <Link to="/review">
                Review {waiting} <Kbd>R</Kbd>
              </Link>
            </Button>
          ) : undefined
        }
      />

      <SyncNotice banks={banks} />

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
        {(q || tag) && (
          <SegmentedControl
            label="Search in"
            value={everyMonth ? "all" : "month"}
            onChange={(v) => setQuery("scope", v === "all" ? "all" : null)}
            options={[
              { value: "month", label: `In ${monthName}` },
              { value: "all", label: "Every month" },
            ]}
          />
        )}
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
              <Button>
                {currentAccount ? `${currentAccount.institution} ${typeLabel[currentAccount.type]}` : "All accounts"}
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
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button>
                {category === "none" ? "To review" : currentCategory ? currentCategory.name : "All categories"}
                <ChevronDown aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuRadioGroup value={category} onValueChange={(v) => setQuery("category", v || null)}>
                <DropdownMenuRadioItem value="">All categories</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="none">To review</DropdownMenuRadioItem>
                {cats.live.map((c) => (
                  <DropdownMenuRadioItem key={c.id} value={c.id}>
                    <CategoryTag name={c.name} color={c.color} />
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          {(tags.length > 0 || tag) && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button>
                  {tag ? `Tag: ${tag}` : "Any tag"}
                  <ChevronDown aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuRadioGroup value={tag} onValueChange={(v) => setQuery("tag", v || null)}>
                  <DropdownMenuRadioItem value="">Any tag</DropdownMenuRadioItem>
                  {tags.map((t) => (
                    <DropdownMenuRadioItem key={t.tag} value={t.tag}>
                      <span className="font-figure text-[13px]">#{t.tag}</span>
                      <span className="ml-auto type-figure-sm text-ink-muted">{t.count}</span>
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      {tag && <TagSummary tag={tag} info={tags.find((t) => t.tag === tag)} byId={cats.byId} />}

      {!everyMonth && (
        <MonthOverview
          month={month}
          current={current}
          data={daysData}
          error={days.error}
          day={day}
          flow={flow}
          following={following}
          color={currentCategory ? `var(--${currentCategory.color})` : undefined}
          onClear={clearFilters}
        />
      )}

      {day && (
        <div className="flex flex-wrap items-center gap-2 text-[14px] leading-5">
          <span className="text-ink-muted">Showing</span>
          <strong className="font-semibold">{formatWeekday(day)}</strong>
          <Button variant="ghost" size="sm" onClick={() => setQuery("day", null)}>
            <X aria-hidden />
            Clear
          </Button>
        </div>
      )}

      <div className="flex flex-wrap items-start gap-4 lg:gap-6">
        <Panel
          title={wide ? (day ? formatWeekday(day) : "All transactions") : undefined}
          className="min-w-0 flex-[999_1_520px]"
          bodyClassName={wide ? "-mx-5 border-t border-rule" : "-mx-5 -mt-5"}
          footer={
            <>
              {list?.truncated && `Showing the newest ${list.transactions.length.toLocaleString("pt-BR")}. `}
              Transfers and card bill payments are listed but never counted as spending. Select a transaction to open it.
            </>
          }
        >
          {res.error && !list && <Empty>Could not load transactions: {res.error}.</Empty>}
          {!list && !res.error && <SkeletonRows count={8} />}
          {list && list.transactions.length === 0 && (
            <Empty>
              {noAccounts
                ? "No accounts yet. They appear after the first sync, which runs within six hours."
                : q
                  ? everyMonth
                    ? `Nothing matches “${q}” in any month.`
                    : `Nothing matches “${q}” in ${day ? formatDay(day) : monthName}.`
                  : day
                    ? `Nothing on ${formatDay(day)}.`
                    : filtered
                      ? `Nothing like this in ${monthName}.`
                      : `Nothing in ${monthName} yet.`}
            </Empty>
          )}
          {list && <Days transactions={list.transactions} selected={tx} categories={cats.byId} filter={category} />}
        </Panel>

        {tx && wide && (
          <aside
            aria-label="Transaction detail"
            className="sticky top-6 max-h-[calc(100dvh-48px)] min-w-0 flex-[1_1_360px] self-start overflow-y-auto rounded-md border border-rule bg-paper-raised px-5 pt-4 pb-5 lg:max-w-[440px]"
          >
            <TransactionDetail id={tx} onClose={close} prev={prev} next={next} />
          </aside>
        )}
      </div>

      {!wide && inSheet.current && (
        <Sheet
          open={Boolean(tx)}
          onOpenChange={(o) => !o && close()}
          label="Transaction"
          actions={<DetailSteps prev={prev} next={next} size="icon" />}
          onCloseAutoFocus={(e) => {
            // Back to the row it came from, which a tap may never have focused.
            e.preventDefault();
            document.querySelector<HTMLElement>(`#tx-${CSS.escape(inSheet.current ?? "")} > a`)?.focus({ preventScroll: true });
          }}
        >
          <TransactionDetail id={inSheet.current} onClose={close} prev={prev} next={next} inSheet />
        </Sheet>
      )}
    </>
  );
}

/** A tag over every month: its total, by category (canvas: "Split, note and tags"). */
function TagSummary({ tag, info, byId }: { tag: string; info?: TagView; byId: Map<string, Category> }) {
  const res = useResource<{ categories: { category: string; outCents: number; inCents: number; count: number }[] }>(`/api/v1/tags/${encodeURIComponent(tag)}`);
  const rows = (res.data?.categories ?? []).filter((c) => c.outCents > 0);
  const max = Math.max(1, ...rows.map((c) => c.outCents));
  const title = info ? `#${tag} · ${formatBRL(info.outCents)} out in ${info.count} ${info.count === 1 ? "transaction" : "transactions"}` : `#${tag}`;
  return (
    <Panel title={title} footer="Every month, a split counted by its parts. Choose “Every month” above to list them all.">
      {rows.length === 0 ? (
        res.data ? <p className="m-0 type-body text-ink-muted">Nothing spent with this tag.</p> : <Skeleton className="h-16" />
      ) : (
        <div className="flex flex-col gap-2">
          {rows.map((c) => {
            const cat = byId.get(c.category);
            return (
              <div key={c.category || "none"} className="grid grid-cols-[minmax(0,140px)_minmax(0,1fr)_auto] items-center gap-3">
                <CategoryTag name={cat?.name} color={cat?.color} />
                <span className="h-2.5 rounded-xs" style={{ width: `${(c.outCents / max) * 100}%`, background: cat ? `var(--${cat.color})` : "var(--rule-strong)" }} />
                <Amount cents={c.outCents} sign="none" />
              </div>
            );
          })}
        </div>
      )}
    </Panel>
  );
}

/** What the list and the month follow, said back above the figures. */
type Following = { category?: Category | "none"; text: string };

const measureOf: Record<Flow, Measure> = { all: "spent", out: "spent", in: "in", transfer: "moved" };
const measureLead: Record<Measure, string> = {
  spent: "Spending only: transfers between your accounts and card bills never count.",
  in: "Money in that counts: income and refunds. Transfers from your own accounts never count.",
  moved: "Moved, not counted: transfers out of your accounts and card bills paid.",
};
const measureColor: Record<Measure, string> = { spent: "var(--ink)", in: "var(--inflow)", moved: "var(--ink-muted)" };

/**
 * The month at a glance: what counts, day by day, against the month before,
 * both under the list's filters (canvas: "Transactions: the month chart and
 * its figures follow the filters"). The type picks the figure the chart
 * draws; with a filter on, a figure at zero in both months is left out.
 */
function MonthOverview({
  month,
  current,
  data,
  error,
  day,
  flow,
  following,
  color,
  onClear,
}: {
  month: string;
  current: string;
  data?: DaysView;
  error?: string;
  day: string | null;
  flow: Flow;
  following: Following | null;
  color?: string;
  onClear: () => void;
}) {
  const monthName = formatMonth(month).split(" ")[0];
  const prevName = formatMonth(addMonths(month, -1)).split(" ")[0];
  const measure = measureOf[flow];
  const s = data?.summary;
  const by = month === current ? `than by ${Number(todayISO().slice(8))} ${prevName.slice(0, 3)}` : `than ${prevName}`;
  function delta(now: number, before: number, m: Measure): { text: string; tone: "inflow" | "warning" | "muted" } | undefined {
    if (before <= 0) return undefined;
    const diff = before - now;
    if (diff === 0) return { text: `The same as ${prevName}`, tone: "muted" };
    const tone = m === "spent" ? (diff > 0 ? "inflow" : "warning") : m === "in" && diff < 0 ? "inflow" : "muted";
    return { text: `${formatBRL(Math.abs(diff))} ${diff > 0 ? "less" : "more"} ${by}`, tone };
  }
  const total = (m: Measure) => (data?.previous ?? []).reduce((t, d) => t + dayValue(d, m), 0);
  const purchases = (data?.days ?? []).reduce((t, d) => t + d.purchases, 0);
  const lead = flow === "all" ? "spent" : measure;
  const figures = s
    ? [
        {
          m: "spent" as const,
          label: "Spent",
          now: s.spentCents,
          cents: -s.spentCents,
          toDate: s.previousToDateCents,
          sign: "negative" as const,
          tone: "ink" as const,
          caption: following ? `${purchases} ${purchases === 1 ? "purchase" : "purchases"}` : undefined,
        },
        { m: "in" as const, label: "Came in", now: s.inCents, cents: s.inCents, toDate: s.previousInToDateCents, sign: "always" as const, tone: "inflow" as const, caption: "Income and refunds" },
        {
          m: "moved" as const,
          label: "Moved, not counted",
          now: s.movedCents,
          cents: s.movedCents,
          toDate: s.previousMovedToDateCents,
          sign: "none" as const,
          tone: "muted" as const,
          caption: "Transfers out and card bills paid",
        },
      ].filter((f) => (flow === "all" ? !following || f.now !== 0 || total(f.m) !== 0 : f.m === measure))
    : [];
  const nothing = following && data && [...data.days, ...data.previous].every((d) => !d.spentCents && !d.inCents && !d.movedCents);
  return (
    <Panel
      title={month === current ? `${monthName} so far` : formatMonth(month)}
      footer={`${measureLead[measure]}${following ? ` ${prevName} follows the same filters.` : ""} A day far above the rest is cut at the dotted line, with its figure on top. Select a day to list it.`}
    >
      {error && !data && <p className="m-0 type-body text-ink-muted">Could not load the month: {error}.</p>}
      {!data && !error && (
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap gap-x-10 gap-y-4">
            <SkeletonStat />
            <SkeletonStat />
            <SkeletonStat />
          </div>
          <Skeleton className="h-[168px] max-lg:h-24" />
        </div>
      )}
      {data && s && (
        <div className="flex flex-col gap-5">
          {following && (
            <div className="-mt-1 flex flex-wrap items-center gap-2 text-[14px] leading-5">
              <span className="text-ink-muted">Following the filters:</span>
              {following.category === "none" ? <CategoryTag review /> : following.category && <CategoryTag name={following.category.name} color={following.category.color} />}
              {following.text && (
                <strong className="font-semibold">
                  {following.category && "· "}
                  {following.text}
                </strong>
              )}
              <Button variant="ghost" size="sm" onClick={onClear}>
                <X aria-hidden />
                Clear filters
              </Button>
            </div>
          )}
          {nothing ? (
            <p className="m-0 type-body text-ink-muted">
              Nothing like this in {monthName} or {prevName}.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap gap-x-10 gap-y-4">
                {figures.map((f) => (
                  <Stat key={f.m} label={f.label} cents={f.cents} sign={f.sign} tone={f.tone} delta={f.m === lead ? delta(f.now, f.toDate, f.m) : undefined} caption={f.caption} />
                ))}
              </div>
              <DayBars
                month={month}
                days={data.days}
                previous={data.previous}
                today={todayISO()}
                selected={day}
                onSelect={(d) => navigate(hrefWith({ day: d, tx: null }), { replace: true })}
                measure={measure}
                color={color ?? measureColor[measure]}
              />
            </>
          )}
        </div>
      )}
    </Panel>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="m-0 px-5 py-6 type-body text-ink-muted">{children}</p>;
}

/**
 * The list, grouped by day, each day with what it added up to. Under a
 * category filter a split shows its part in that category, as the figures
 * count it, and names that category.
 */
function Days({
  transactions,
  selected,
  categories,
  filter,
}: {
  transactions: Transaction[];
  selected: string | null;
  categories: Map<string, Category>;
  /** The category filter: an id, "none" or "". */
  filter: string;
}) {
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
            <div key={t.id} id={`tx-${t.id}`}>
              <TransactionRow
                to={hrefWith({ tx: t.id })}
                selected={t.id === selected}
                date={formatDay(t.date)}
                description={t.description}
                meta={meta(t)}
                kind={t.kind}
                amountCents={t.countedCents}
                pending={t.status === "pending"}
                installment={t.installment ? `${t.installment.number} of ${t.installment.total}` : undefined}
                category={categoryOf(t, categories, filter)}
                moreCategories={Math.max(0, t.splitCategories.length - 1)}
                tags={t.tags}
                noted={Boolean(t.note)}
              />
            </div>
          ))}
        </div>
      ))}
    </>
  );
}

/** "R$ 72,00 out · R$ 480,00 in": only what counts, a split's filtered part included. */
function dayTotal(rows: Transaction[]): string {
  let out = 0;
  let inn = 0;
  for (const t of rows) {
    if (t.kind === "transfer" || t.kind === "bill_payment") continue;
    if (t.countedCents < 0) out -= t.countedCents;
    else inn += t.countedCents;
  }
  const parts = [];
  if (out) parts.push(`${formatBRL(out)} out`);
  if (inn) parts.push(`${formatBRL(inn)} in`);
  return parts.join(" · ") || "Nothing counted";
}

/** "Nubank · credit card", or for a pair "Nubank checking → Itaú checking". */
function meta(t: Transaction): string {
  const here = `${t.account.institution} ${typeLabel[t.account.type]}`;
  // One I added to a synced account says so, so it never passes for one the bank sent.
  const by = t.manual && t.account.type !== "cash" ? " · added by you" : "";
  if (t.counterpart) {
    const there = `${t.counterpart.institution} ${typeLabel[t.counterpart.type]}`;
    return (t.amountCents < 0 ? `${here} → ${there}` : `${here} ← ${there}`) + by;
  }
  return `${t.account.institution} · ${typeLabel[t.account.type]}${by}`;
}

/** The row's category: its name and colour, or null while it waits for one. A split under a category filter names the filtered one. */
function categoryOf(t: Transaction, categories: Map<string, Category>, filter: string) {
  const id = t.splitCategories.includes(filter) ? filter : t.category?.id;
  const c = id ? categories.get(id) : undefined;
  return c ? { name: c.name, color: c.color } : null;
}
