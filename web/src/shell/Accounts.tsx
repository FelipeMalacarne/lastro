import { ChevronRight } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Panel } from "@/components/Panel";
import { SyncStatus } from "@/components/SyncStatus";
import { typeName, useResource, type Account, type Connection } from "@/lib/api";
import { Link, useLocation } from "@/lib/router";
import { cn } from "@/lib/utils";

export type Bank = { connection: Connection | null; institution: string; accounts: Account[] };

/** Accounts grouped by the connection that feeds them, in the API's order. */
export function useBanks(): { banks: Bank[]; loading: boolean; error?: string } {
  const accounts = useResource<{ accounts: Account[] }>("/api/v1/accounts");
  const connections = useResource<{ connections: Connection[] }>("/api/v1/connections");
  const all = accounts.data?.accounts ?? [];
  const banks: Bank[] = (connections.data?.connections ?? []).map((c) => ({
    connection: c,
    institution: c.institution,
    accounts: all.filter((a) => c.accountIds.includes(a.id)),
  }));
  const linked = new Set(banks.flatMap((b) => b.accounts.map((a) => a.id)));
  for (const a of all.filter((a) => !linked.has(a.id))) {
    let b = banks.find((x) => !x.connection && x.institution === a.institution);
    if (!b) banks.push((b = { connection: null, institution: a.institution, accounts: [] }));
    b.accounts.push(a);
  }
  return { banks, loading: accounts.loading || connections.loading, error: accounts.error || connections.error };
}

/** The sidebar's list: each connection's freshness, then its accounts and balances. */
export function SidebarAccounts() {
  const { banks } = useBanks();
  const { path } = useLocation();
  if (banks.length === 0) return null;
  return (
    <section aria-label="Accounts" className="flex flex-col gap-3 px-2">
      <span className="type-label text-ink-muted">Accounts</span>
      {banks.map((b) => (
        <div key={b.connection?.itemId ?? b.institution} className="flex flex-col gap-0.5">
          <SyncStatus compact institution={b.institution} state={b.connection?.health ?? "stale"} detail={b.connection?.short ?? "never"} />
          {b.accounts.map((a) => {
            const here = path === `/accounts/${a.id}`;
            return (
              <Link
                key={a.id}
                to={`/accounts/${a.id}`}
                aria-current={here ? "page" : undefined}
                className={cn(
                  "-mx-2 flex min-h-8 items-baseline justify-between gap-2 rounded-sm py-1.5 pr-2 pl-[30px] type-caption no-underline transition-colors",
                  here ? "bg-tinta-soft font-semibold text-tinta hover:text-tinta" : "text-ink-muted hover:bg-paper-sunken hover:text-ink",
                )}
              >
                <span className="truncate">{a.type === "credit_card" ? "Card" : typeName[a.type]}</span>
                <Amount cents={a.balanceCents} size="sm" tone={here ? "ink" : "auto"} />
              </Link>
            );
          })}
        </div>
      ))}
    </section>
  );
}

/** The same on a phone, as a panel at the top of Transactions. */
export function AccountsPanel({ banks }: { banks: Bank[] }) {
  if (banks.length === 0) return null;
  return (
    <Panel title="Accounts" className="lg:hidden">
      <div className="flex flex-col gap-3">
        {banks.map((b, i) => (
          <div key={b.connection?.itemId ?? b.institution} className={i < banks.length - 1 ? "flex flex-col gap-1 border-b border-rule pb-3" : "flex flex-col gap-1"}>
            <SyncStatus institution={b.institution} state={b.connection?.health ?? "stale"} detail={b.connection?.detail ?? "Not synced yet"} />
            {b.accounts.map((a) => (
              <Link
                key={a.id}
                to={`/accounts/${a.id}`}
                className="flex min-h-11 items-center justify-between gap-2 pl-10 text-[14px] leading-5 text-ink no-underline hover:text-ink"
              >
                <span className="text-ink-muted">{typeName[a.type]}</span>
                <span className="inline-flex items-center gap-1.5">
                  <Amount cents={a.balanceCents} />
                  <ChevronRight className="size-3.5 text-ink-muted" aria-hidden />
                </span>
              </Link>
            ))}
          </div>
        ))}
      </div>
    </Panel>
  );
}
