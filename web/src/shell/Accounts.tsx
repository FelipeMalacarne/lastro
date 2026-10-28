import { useState } from "react";
import { ChevronDown, ChevronRight, Link2Off, TriangleAlert } from "lucide-react";
import { Collapsible } from "radix-ui";

import { Amount } from "@/components/Amount";
import { SyncStatus } from "@/components/SyncStatus";
import { Tooltip } from "@/components/ui/tooltip";
import { typeName, useResource, type Account, type Connection } from "@/lib/api";
import { formatBRL } from "@/lib/format";
import { Link, useLocation } from "@/lib/router";
import { SyncButton, nextSync } from "@/lib/sync";
import { cn } from "@/lib/utils";

/** A connection and its accounts; `manual` is the group of accounts I type in. */
export type Bank = { connection: Connection | null; institution: string; accounts: Account[]; manual?: boolean };

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
  for (const a of all.filter((a) => !linked.has(a.id) && !a.manual)) {
    let b = banks.find((x) => !x.connection && x.institution === a.institution);
    if (!b) banks.push((b = { connection: null, institution: a.institution, accounts: [] }));
    b.accounts.push(a);
  }
  const manual = all.filter((a) => a.manual);
  if (manual.length) banks.push({ connection: null, institution: "Outside Open Finance", accounts: manual, manual: true });
  return { banks, loading: accounts.loading || connections.loading, error: accounts.error || connections.error };
}

/** The sidebar's list: each connection's freshness, then its accounts and balances. */
export function SidebarAccounts() {
  const { banks } = useBanks();
  const { path } = useLocation();
  if (banks.length === 0) return null;
  return (
    <section aria-label="Accounts" className="flex flex-col gap-3 px-2">
      <span className="flex items-center justify-between gap-2">
        <span className="type-label text-ink-muted">Accounts</span>
        <SyncButton iconOnly className="-my-1" />
      </span>
      {banks.map((b) => (
        <div key={b.connection?.itemId ?? b.institution} className="flex flex-col gap-0.5">
          {b.manual ? (
            <Tooltip label="Typed in by you">
              <span className="flex min-h-8 items-center self-start text-[14px] leading-5 font-semibold">{b.institution}</span>
            </Tooltip>
          ) : (
            <SyncStatus compact institution={b.institution} state={b.connection?.health ?? "stale"} detail={b.connection?.short ?? "never"} />
          )}
          {b.accounts.map((a) => {
            const here = path === `/accounts/${a.id}`;
            const link = (
              <Link
                key={a.id}
                to={`/accounts/${a.id}`}
                aria-current={here ? "page" : undefined}
                aria-label={`${a.manual ? a.name : `${b.institution} ${a.type === "credit_card" ? "card" : typeName[a.type].toLowerCase()}`}, ${formatBRL(a.balanceCents)}`}
                className={cn(
                  "-mx-2 flex min-h-8 items-baseline justify-between gap-2 rounded-sm py-1.5 pr-2 pl-[30px] type-caption no-underline transition-colors",
                  here ? "bg-tinta-soft font-semibold text-tinta hover:text-tinta" : "text-ink-muted hover:bg-paper-sunken hover:text-ink",
                )}
              >
                <span className="truncate">{a.manual ? a.name : a.type === "credit_card" ? "Card" : typeName[a.type]}</span>
                <Amount cents={a.balanceCents} size="sm" tone={here ? "ink" : "auto"} />
              </Link>
            );
            // A name typed in may not fit the sidebar.
            return a.manual ? (
              <Tooltip key={a.id} label={a.name} side="right">
                {link}
              </Tooltip>
            ) : (
              link
            );
          })}
        </div>
      ))}
      <span className="type-caption text-ink-muted">
        Next sync at <span className="font-figure">{nextSync()}</span>
      </span>
    </section>
  );
}

/** The same on a phone, as a panel at the top of Transactions. */
export function AccountsPanel({ banks }: { banks: Bank[] }) {
  const [open, setOpen] = useState(false);
  if (banks.length === 0) return null;
  // Closed, the one connection that needs attention shows on the summary line,
  // read as the sidebar reads it: a bank with no connection has never synced.
  const health = (b: Bank) => (b.manual ? "ok" : (b.connection?.health ?? "stale"));
  const worst = banks.find((b) => health(b) === "reauth") ?? banks.find((b) => health(b) === "stale");
  const reauth = worst && health(worst) === "reauth";
  const WorstIcon = reauth ? Link2Off : TriangleAlert;
  return (
    // Folded by default, so the month and the list come first on a phone.
    <Collapsible.Root open={open} onOpenChange={setOpen} className="group rounded-md border border-rule bg-paper-raised lg:hidden">
      <Collapsible.Trigger className="flex min-h-12 w-full cursor-pointer items-center justify-between gap-3 rounded-md border-0 bg-transparent px-5 py-2 text-left text-ink focus-visible:focus-ring">
        <span className="type-heading">Accounts · {banks.reduce((n, b) => n + b.accounts.length, 0)}</span>
        <span className="flex min-w-0 items-center gap-2">
          {!open && worst && (
            <span className={cn("inline-flex min-w-0 items-center gap-1.5 type-caption font-semibold", reauth ? "text-danger" : "text-warning")}>
              <WorstIcon className="size-3.5 flex-none" aria-hidden />
              <span className="truncate">
                {worst.institution} {worst.connection?.short ?? "never"}
              </span>
            </span>
          )}
          <ChevronDown className="size-4 flex-none text-ink-muted transition-transform duration-[160ms] group-data-[state=open]:rotate-180" aria-hidden />
        </span>
      </Collapsible.Trigger>
      <Collapsible.Content className="overflow-hidden data-[state=closed]:animate-[lt-collapse_160ms_ease-in] data-[state=open]:animate-[lt-expand_160ms_ease-out]">
        <div className="flex flex-col gap-3 border-t border-rule px-5 pt-3 pb-4">
          {banks.map((b, i) => (
            <div key={b.connection?.itemId ?? b.institution} className={i < banks.length - 1 ? "flex flex-col gap-1 border-b border-rule pb-3" : "flex flex-col gap-1"}>
              {b.manual ? (
                <span className="flex min-h-10 items-center text-[15px] leading-[22px] font-semibold">{b.institution}</span>
              ) : (
                <SyncStatus institution={b.institution} state={b.connection?.health ?? "stale"} detail={b.connection?.detail ?? "Not synced yet"} />
              )}
              {b.accounts.map((a) => (
                <Link
                  key={a.id}
                  to={`/accounts/${a.id}`}
                  className="flex min-h-11 items-center justify-between gap-2 pl-10 text-[14px] leading-5 text-ink no-underline hover:text-ink"
                >
                  <span className="text-ink-muted">{a.manual ? a.name : typeName[a.type]}</span>
                  <span className="inline-flex items-center gap-1.5">
                    <Amount cents={a.balanceCents} />
                    <ChevronRight className="size-3.5 text-ink-muted" aria-hidden />
                  </span>
                </Link>
              ))}
            </div>
          ))}
        </div>
      </Collapsible.Content>
    </Collapsible.Root>
  );
}
