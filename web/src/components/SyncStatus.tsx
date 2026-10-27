import type { ReactNode } from "react";
import { Check, Link2Off, RefreshCw, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/utils";

export type SyncState = "ok" | "syncing" | "stale" | "reauth";

const states = {
  ok: { icon: Check, tone: "text-inflow", well: "bg-paper-sunken" },
  syncing: { icon: RefreshCw, tone: "text-ink-muted", well: "bg-paper-sunken" },
  stale: { icon: TriangleAlert, tone: "text-warning", well: "bg-warning-soft" },
  reauth: { icon: Link2Off, tone: "text-danger", well: "bg-danger-soft" },
};

type Props = {
  institution: string;
  state: SyncState;
  /** "Synced 2 h ago", "Last synced 3 days ago"; in compact, short: "2 h", "3 days". */
  detail: string;
  action?: ReactNode;
  /** One line with a 14px icon, for the sidebar's accounts list. */
  compact?: boolean;
  className?: string;
};

/** How fresh an institution's data is, so stale numbers never pass for current ones. */
export function SyncStatus({ institution, state, detail, action, compact, className }: Props) {
  const s = states[state];
  const spin = state === "syncing" && "animate-spin [animation-duration:1.2s]";
  if (compact) {
    return (
      <div className={cn("flex min-h-7 min-w-0 items-center gap-2 text-[14px] leading-5 text-ink", className)}>
        <s.icon className={cn("size-3.5 flex-none", s.tone, spin)} aria-hidden />
        <span className="min-w-0 flex-1 truncate">{institution}</span>
        <span
          className={cn(
            "type-figure-sm whitespace-nowrap text-ink-muted",
            state === "stale" && "font-medium text-warning",
            state === "reauth" && "font-medium text-danger",
          )}
        >
          {detail}
        </span>
      </div>
    );
  }
  return (
    <div className={cn("flex min-w-0 items-center gap-3", className)}>
      <span className={cn("inline-flex size-7 flex-none items-center justify-center rounded-sm", s.well, s.tone)}>
        <s.icon className={cn("size-4", spin)} aria-hidden />
      </span>
      <div className="grid min-w-0 flex-1">
        <span className="text-[14px] leading-5 font-semibold">{institution}</span>
        <span
          className={cn(
            "type-caption text-ink-muted",
            state === "stale" && "font-semibold text-warning",
            state === "reauth" && "font-semibold text-danger",
          )}
        >
          {detail}
        </span>
      </div>
      {action && <div>{action}</div>}
    </div>
  );
}
