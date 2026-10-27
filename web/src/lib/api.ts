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
};

export type TransactionList = {
  transactions: Transaction[];
  summary: { count: number; pending: number; inCents: number; outCents: number };
  truncated: boolean;
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
