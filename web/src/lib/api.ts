import { useEffect, useSyncExternalStore } from "react";

// The API's shapes (see modules/ledger/views.go and modules/ingest/http.go).

export type AccountType = "checking" | "savings" | "credit_card";

export type Account = {
  id: string;
  name: string;
  institution: string;
  type: AccountType;
  number: string;
  currency: string;
  balanceCents: number;
  balanceAt: string;
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
};

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
  counterpartTransaction: Transaction | null;
  purchase: Purchase | null;
  history: { count: number; totalCents: number; months: { month: string; cents: number; count: number }[] };
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
};

export const typeName: Record<AccountType, string> = {
  checking: "Checking",
  savings: "Savings",
  credit_card: "Credit card",
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
  if (!r.ok) throw new Error(out.message || `HTTP ${r.status}`);
  refreshAll();
  return out as T;
}

// categorize (see modules/categorize/http.go).

export type NoteColor = "note-1" | "note-2" | "note-5" | "note-10" | "note-20" | "note-50" | "note-100" | "note-200";

export type Category = { id: string; name: string; color: NoteColor; budgeted: boolean; archived: boolean };

export type PatternKind = "contains" | "exact";
export type Pattern = { kind: PatternKind; text: string };

export type ReviewGroup = {
  pattern: Pattern | null;
  count: number;
  totalCents: number;
  latest: string;
  transactions: { id: string; date: string; description: string; amountCents: number; accountId: string }[];
};

export type Review = { waiting: number; groups: ReviewGroup[] };
