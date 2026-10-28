import { useEffect, useSyncExternalStore } from "react";

// The API's shapes (see modules/ledger/views.go and modules/ingest/http.go).

export type AccountType = "checking" | "savings" | "credit_card" | "cash" | "asset";

export type Account = {
  id: string;
  name: string;
  institution: string;
  type: AccountType;
  number: string;
  currency: string;
  balanceCents: number;
  balanceAt: string;
  /** Typed in by me, outside Open Finance: a wallet ("cash") or an asset. */
  manual: boolean;
  /** A card's limit and bill, each part null when the bank does not report it. */
  card: { limitCents: number | null; availableCents: number | null; dueDate: string | null; closeDate: string | null } | null;
};

export type Health = "ok" | "syncing" | "stale" | "reauth";

export type Connection = {
  itemId: string;
  institution: string;
  health: Health;
  detail: string;
  short: string;
  dataUpdatedAt: string | null;
  consentExpiresAt: string | null;
  expiresSoon: boolean;
  readFailing: boolean;
  accountIds: string[];
};

export type AccountRef = { id: string; name: string; institution: string; type: AccountType };

export type Transaction = {
  id: string;
  date: string;
  description: string;
  amountCents: number;
  currency: string;
  kind: "expense" | "income" | "transfer" | "bill_payment";
  status: "pending" | "posted";
  installment: { number: number; total: number } | null;
  category: { id: string; source: string } | null;
  account: AccountRef;
  counterpart: AccountRef | null;
  counterpartId: string | null;
  note: string;
  tags: string[];
  /** A split's categories, largest part first; empty when not split. */
  splitCategories: string[];
  /** A transfer I marked: to an account lastro does not have. */
  markedTransfer: boolean;
  /** Added by me: a wallet entry, or one the bank never sent. */
  manual: boolean;
};

export type TagView = { tag: string; count: number; outCents: number; inCents: number; last: string };

export type TransactionList = {
  transactions: Transaction[];
  summary: { count: number; pending: number; inCents: number; outCents: number };
  truncated: boolean;
};

export type BalanceHistory = {
  currency: string;
  asOf: string;
  points: { date: string; balanceCents: number }[];
};

export type Flow = {
  kind: Transaction["kind"];
  direction: "in" | "out";
  counterpart: AccountRef | null;
  cents: number;
  count: number;
};

export type MonthView = {
  month: string;
  future: boolean;
  inCents: number;
  outCents: number;
  spentCents: number;
  refundsCents: number;
  purchases: number;
  flows: Flow[];
};

export type Purchase = {
  description: string;
  total: number;
  amountCents: number;
  current: number;
  remainingCount: number;
  remainingCents: number;
  firstDate: string;
  lastDate: string;
  installments: { id: string; number: number; date: string; status: "pending" | "posted" }[];
};

export type TransactionDetail = {
  transaction: Transaction;
  split: { category: string; amountCents: number }[];
  counterpartTransaction: Transaction | null;
  purchase: Purchase | null;
  history: { count: number; totalCents: number; months: { month: string; cents: number; count: number }[] };
  /** What it may be the same movement as: for one I added, the synced ones; for a synced one, mine. */
  duplicates: Transaction[];
};

export type DayView = { date: string; spentCents: number; purchases: number; inCents: number; movedCents: number };

export type DaysView = {
  month: string;
  days: DayView[];
  previous: DayView[];
  summary: { spentCents: number; inCents: number; movedCents: number; previousToDateCents: number };
};

type Entry<T> = { data?: T; error?: string; loading: boolean; at: number };

// One cache entry per URL, shared by every component that reads it, so the
// sidebar and the screen never fetch the accounts twice.
const cache = new Map<string, Entry<unknown>>();
const listeners = new Set<() => void>();
const inflight = new Map<string, AbortController>();
const notify = () => listeners.forEach((l) => l());
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

/**
 * When the session is gone, the API answers 401 with where to sign in
 * (ADR-0008). The page goes there and comes back to where it was.
 */
function signInIfAsked(r: Response, body: { login?: string }): boolean {
  if (r.status !== 401 || !body.login || !body.login.startsWith("/")) return false;
  window.location.assign(`${body.login}?next=${encodeURIComponent(window.location.pathname + window.location.search)}`);
  return true;
}

async function load(url: string) {
  inflight.get(url)?.abort();
  const ctrl = new AbortController();
  inflight.set(url, ctrl);
  const prev = cache.get(url);
  cache.set(url, { ...prev, loading: true, at: prev?.at ?? 0 });
  notify();
  try {
    const r = await fetch(url, { signal: ctrl.signal, headers: { Accept: "application/json" } });
    const body = await r.json().catch(() => ({}));
    if (signInIfAsked(r, body)) return;
    if (!r.ok) throw new Error(body.message || `HTTP ${r.status}`);
    cache.set(url, { data: body, loading: false, at: Date.now() });
  } catch (e) {
    if (ctrl.signal.aborted) return;
    cache.set(url, { ...cache.get(url), error: (e as Error).message, loading: false, at: Date.now() });
  } finally {
    if (inflight.get(url) === ctrl) inflight.delete(url);
    notify();
  }
}

/** Reads url as JSON, refetching when it changes and when the tab comes back after a minute. */
export function useResource<T>(url: string | null): Entry<T> {
  const entry = useSyncExternalStore(subscribe, () => (url ? cache.get(url) : undefined)) as Entry<T> | undefined;
  useEffect(() => {
    if (!url) return;
    const e = cache.get(url);
    if (!e || (!e.loading && Date.now() - e.at > 60_000)) void load(url);
    const onVisible = () => {
      const cur = cache.get(url);
      if (document.visibilityState === "visible" && cur && !cur.loading && Date.now() - cur.at > 60_000) void load(url);
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [url]);
  return entry ?? { loading: Boolean(url), at: 0 };
}

export const typeLabel: Record<AccountType, string> = {
  checking: "checking",
  savings: "savings",
  credit_card: "credit card",
  cash: "wallet",
  asset: "asset",
};

export const typeName: Record<AccountType, string> = {
  checking: "Checking",
  savings: "Savings",
  credit_card: "Credit card",
  cash: "Wallet",
  asset: "Asset",
};

/** Refetches every resource read so far: after a change, whatever shows it is fresh. */
export function refreshAll() {
  for (const url of cache.keys()) void load(url);
}

/** Sends a change and refreshes what the screens show. Throws the API's message. */
export async function send<T = unknown>(method: "POST" | "PUT" | "PATCH" | "DELETE", url: string, body?: unknown): Promise<T> {
  const r = await fetch(url, {
    method,
    headers: { Accept: "application/json", ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const out = await r.json().catch(() => ({}));
  if (signInIfAsked(r, out)) throw new Error("Signing in again…");
  if (!r.ok) throw new Error(out.message || `HTTP ${r.status}`);
  refreshAll();
  return out as T;
}

// categorize (see modules/categorize/http.go).

export type NoteColor = "note-1" | "note-2" | "note-5" | "note-10" | "note-20" | "note-50" | "note-100" | "note-200";

export type Category = { id: string; name: string; color: NoteColor; budgeted: boolean; archived: boolean };

export type PatternKind = "contains" | "exact";
export type Pattern = { kind: PatternKind; text: string };

/** What puts a choice back: a choice answers with it and POST /api/v1/undo takes it. */
export type Undo = { rule?: string; wasRule?: string; before: { transaction: string; category?: string; source?: string }[] };
export type Change = { categorized: number; undo: Undo };

/** Two undos as one, for a choice made transaction by transaction. */
export function joinUndo(a: Undo | null, b: Undo): Undo {
  return a ? { ...b, rule: b.rule ?? a.rule, wasRule: b.wasRule ?? a.wasRule, before: [...a.before, ...b.before] } : b;
}

export type ReviewGroup = {
  pattern: Pattern | null;
  count: number;
  totalCents: number;
  latest: string;
  transactions: { id: string; date: string; description: string; amountCents: number; accountId: string }[];
  /** The local model's answer for the merchant, when it named a category. */
  suggestion: { category: string; confidence: number } | null;
};

export type Review = { waiting: number; groups: ReviewGroup[] };

export type RuleOrigin = "correction" | "hand";

export type Rule = {
  id: string;
  order: number;
  origin: RuleOrigin;
  pattern: Pattern;
  account: string | null;
  category: string;
  meets: number;
  decides: number;
  last: string | null;
  createdAt: string;
  updatedAt: string;
};

export type CategoryTotal = { category: string; outCents: number; inCents: number; count: number };

// budget (see modules/budget/http.go).

export type Pace = "on_pace" | "ahead" | "over";

export type MonthSpend = { month: string; spentCents: number };

/** One of a line's categories: its spending this month and, alone, its six-month average. */
export type BudgetLinePart = { category: string; spentCents: number; count: number; averageCents: number };

/** A budget line (ADR-0012): one or more categories under one limit. `category` is its only one, or a shared line's first. */
export type BudgetCategory = {
  id: string;
  name: string | null;
  category: string;
  byCategory: BudgetLinePart[];
  limitCents: number;
  carriedCents: number;
  availableCents: number;
  spentCents: number;
  leftCents: number;
  rollover: boolean;
  /** A fund (ADR-0011) carries its balance, debt included: available and left are its balance before and after spending. */
  fund: boolean;
  adjustedCents: number;
  usedBp: number;
  pace: Pace;
  count: number;
  fixedCents: number;
  upcomingCents: number;
  history: MonthSpend[];
  averageCents: number;
};

export type BudgetSpend = { category: string | null; spentCents: number; count: number; history: MonthSpend[]; averageCents: number };

export type BudgetMonth = {
  month: string;
  hasBudget: boolean;
  saved: boolean;
  from: string | null;
  prevDiffers: boolean;
  rolloverNew: boolean;
  days: number;
  daysLeft: number;
  elapsedBp: number;
  payday: number;
  payYourselfFirst: { plannedCents: number; movedCents: number; due: string; doneOn: string | null };
  incomeCents: number;
  incomeCount: number;
  limitsCents: number;
  carriedCents: number;
  availableCents: number;
  spentCents: number;
  leftCents: number;
  notAssignedCents: number;
  upcomingCents: number;
  /** The funds together; carried, available, spent, left and upcoming above are the limits'. */
  funds: { count: number; amountCents: number; carriedCents: number; adjustedCents: number; spentCents: number; balanceCents: number; keptCents: number };
  categories: BudgetCategory[];
  unlimited: BudgetSpend[];
  summary: { incomeCents: number; spentCents: number; contributionCents: number; savingsRateBp: number };
};

export type RecurringCharge = {
  name: string;
  category: string | null;
  steadiness: "fixed" | "varies";
  monthlyCents: number;
  months: number;
  first: string;
  last: string;
  next: string;
  paidThisMonth: boolean;
  new: boolean;
  change: { fromCents: number; toCents: number; since: string } | null;
};

export type Recurring = { monthlyCents: number; paidCents: number; upcomingCents: number; new: number; changed: number; charges: RecurringCharge[] };

// portfolio (see modules/portfolio/http.go).

export type AssetClass = "cdi" | "ipca" | "br_stocks" | "global";
export type BandStatus = "in_band" | "below" | "above";

export const assetClasses: { id: AssetClass; name: string; color: NoteColor }[] = [
  { id: "cdi", name: "CDI", color: "note-2" },
  { id: "ipca", name: "IPCA+", color: "note-20" },
  { id: "br_stocks", name: "Brazilian stocks", color: "note-1" },
  { id: "global", name: "Global stocks", color: "note-100" },
];

export type NetWorthPoint = {
  date: string;
  cashCents: number;
  cardsCents: number;
  investedCents: number;
  emergencyCents: number;
  /** Assets outside Open Finance, typed in. */
  otherCents: number;
  basisCents: number;
  totalCents: number;
};

/** One lot of a group: a single purchase or contribution, as the bank reports it. */
export type Lot = {
  id: string;
  quantity: string;
  valueCents: number;
  basisCents: number;
  returnBp: number | null;
  maturity: string | null;
  asOf: string;
  priced: boolean;
};

export type Reserve = "none" | "amount" | "all";

/** A product held, in one or more lots (each Nubank Caixinha contribution is a CDB of its own). */
export type PositionGroup = {
  id: string;
  name: string;
  institution: string;
  kind: string;
  ticker: string | null;
  class: AssetClass;
  autoClass: AssetClass;
  indexer: "" | "CDI" | "SELIC" | "IPCA" | "PRE";
  rateBp: number;
  /** How much of it is the emergency fund: none, reserveCents, or all of it. */
  reserve: Reserve;
  reserveCents: number;
  /** What of it is the emergency fund today, and what it lacks to cover reserveCents. */
  emergencyCents: number;
  shortCents: number;
  valueCents: number;
  basisCents: number;
  returnBp: number | null;
  firstMaturity: string | null;
  lastMaturity: string | null;
  asOf: string;
  priced: boolean;
  lots: Lot[];
};

export type Thesis = {
  ticker: string;
  thesis: string;
  breaks: string;
  nextReport: string | null;
  reportInDays: number | null;
  held: boolean;
  valueCents: number;
};

export type PortfolioOverview = {
  asOf: string | null;
  positions: number;
  where: string[] | null;
  investedCents: number;
  basisCents: number;
  monthDeltaCents: number | null;
  monthFrom: string | null;
  putIn12mCents: number | null;
  returns12mCents: number | null;
  return12mBp: number | null;
  since: string | null;
  netWorth: NetWorthPoint;
  targets: Record<AssetClass, number>;
  bandBp: number;
  classes: {
    class: AssetClass;
    valueCents: number;
    basisCents: number;
    nowBp: number;
    targetBp: number;
    status: BandStatus;
    return12mBp: number | null;
    holdings: number;
  }[];
  growth: { date: string; valueCents: number; basisCents: number }[];
  benchmarks: { from: string | null; cdiBp: number | null; ipcaBp: number | null; cdiUntil: string | null; ipcaUntil: string | null };
  groups: PositionGroup[];
  theses: Thesis[];
};

export type ContributionWhy = "below_band" | "gap" | "target" | "above_band" | "at_target";

export type ContributionPlan = {
  amountCents: number;
  totalCents: number;
  bandBp: number;
  splits: {
    class: AssetClass;
    amountCents: number;
    why: ContributionWhy;
    valueCents: number;
    beforeBp: number;
    afterBp: number;
    targetBp: number;
    buy: string | null;
  }[];
};

export type PlannedPurchase = {
  id: string;
  plannedOn: string;
  what: string;
  class: AssetClass;
  amountCents: number;
  condition: string;
  status: "planned" | "done" | "cancelled";
  note: string;
  decidedAt: string | null;
};

export type MarketIndices = {
  CDI: { last12mBp: number; last: string | null; latestBp: number };
  IPCA: { last12mBp: number; last: string | null; latestBp: number };
  bonds: { title: string; name: string; maturity: string; date: string; buyRateBp: number; priceCents: number }[];
};
