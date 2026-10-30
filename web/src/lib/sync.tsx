import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { ApiError, refreshAll, send, useResource, type Connection } from "@/lib/api";
import { MEU_PLUGGY } from "@/lib/notice";
import { navigate } from "@/lib/router";
import { toast, type ToastAction } from "@/lib/toast";
import { cn } from "@/lib/utils";

// The sync CronJob: minute 17 of every sixth hour, in Brasília time.
const syncHours = [0, 6, 12, 18];

/** The next "HH:17" the sync CronJob runs at, in Brasília time. */
export function nextSync(now = new Date()): string {
  const [h, m] = new Intl.DateTimeFormat("en-GB", { timeZone: "America/Sao_Paulo", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
    .format(now)
    .split(":")
    .map(Number);
  const next = syncHours.find((x) => x > h || (x === h && m < 17)) ?? syncHours[0];
  return `${String(next).padStart(2, "0")}:17`;
}

type SyncStatus = { running: boolean; started: string | null; finished: string | null; ok: boolean; message: string };

/** Opens Settings › Server, where the server says how it is set up. */
const openServer: ToastAction = {
  label: "Open Settings",
  run: () => {
    navigate("/settings#server");
    requestAnimationFrame(() => document.getElementById("server")?.scrollIntoView({ block: "start" }));
  },
};

/**
 * Why a sync could not start, in one sentence (canvas: R2ToastDesktop). The
 * server's error chain goes to the console, never on screen.
 */
function startFailure(e: unknown): { text: string; action?: ToastAction } {
  console.error("lastro: sync now:", e);
  if (e instanceof ApiError && e.status === 503 && e.code === "unavailable") {
    return { text: "Sync is off: Pluggy is not set up on this server.", action: openServer };
  }
  if (e instanceof ApiError && e.status === 409) return { text: "A sync is already running." };
  return { text: "Could not start the sync. Try again in a minute." };
}

/**
 * How a finished sync that went wrong reads: the bank that needs a new
 * login, with a way to Meu Pluggy, or one sentence; never the error chain.
 */
async function syncFailure(s: SyncStatus): Promise<{ text: string; action?: ToastAction } | null> {
  if (!s.ok) {
    console.error("lastro: sync failed:", s.message);
    return { text: `Sync failed: the server could not finish it. It tries again at ${nextSync()}.` };
  }
  // "…; 1 bank failed": when a bank wants a new login, say which and where.
  if (!/ failed$/.test(s.message)) return null;
  const conns = await fetch("/api/v1/connections", { headers: { Accept: "application/json" } })
    .then((x) => x.json() as Promise<{ connections: Connection[] }>)
    .then((x) => x.connections)
    .catch(() => []);
  const reauth = conns.find((c) => c.health === "reauth");
  if (!reauth) return null;
  return {
    text: `Sync failed: ${reauth.institution} needs you to log in again.`,
    action: { label: "Open Meu Pluggy", run: () => window.open(MEU_PLUGGY, "_blank", "noreferrer") },
  };
}

// One watcher for the page, however many Sync buttons it shows: it polls a
// running sync until it ends, then everything on screen reloads. A sync
// started from here reports its outcome in a toast, wherever the owner is.
let watching = false;
let reportOutcome = false;

function watch() {
  if (watching) return;
  watching = true;
  void (async () => {
    try {
      for (;;) {
        await new Promise((r) => setTimeout(r, 2000));
        const s = await fetch("/api/v1/sync", { headers: { Accept: "application/json" } })
          .then((x) => x.json() as Promise<SyncStatus>)
          .catch(() => null);
        if (!s || s.running) continue;
        refreshAll();
        if (reportOutcome) {
          const failure = await syncFailure(s);
          toast(failure ? { id: "sync", tone: "error", ...failure } : { id: "sync", text: `Synced: ${s.message}.` });
        }
        reportOutcome = false;
        return;
      }
    } finally {
      watching = false;
    }
  })();
}

/** A sync started from the screens (POST /api/v1/sync): it runs on the server. */
export function useSync() {
  const res = useResource<SyncStatus>("/api/v1/sync");
  const [error, setError] = useState<string | null>(null);
  const running = res.data?.running ?? false;
  useEffect(() => {
    if (running) watch();
  }, [running]);
  async function start() {
    setError(null);
    reportOutcome = true;
    toast({ id: "sync", tone: "busy", text: "Syncing every account…" });
    try {
      await send("POST", "/api/v1/sync");
      watch();
    } catch (e) {
      reportOutcome = false;
      const failure = startFailure(e);
      setError(failure.text);
      toast({ id: "sync", tone: "error", ...failure });
    }
  }
  const last = res.data;
  const outcome = error ?? (last && !last.running && last.finished ? (last.ok ? `Last sync from here: ${last.message}.` : `Last sync from here failed: ${last.message}.`) : null);
  return { running, start, outcome, failed: Boolean(error || (last && !last.running && last.finished && !last.ok)) };
}

/** "Sync now": every bank, through Meu Pluggy, then prices and the day's valuation. */
export function SyncButton({ label = "Sync now", iconOnly, className }: { label?: string; iconOnly?: boolean; className?: string }) {
  const { running, start } = useSync();
  const button = (
    <Button
      variant={iconOnly ? "ghost" : "secondary"}
      size={iconOnly ? "icon-sm" : "sm"}
      onClick={() => void start()}
      disabled={running}
      aria-label={iconOnly ? (running ? "Syncing every account" : "Sync every account now") : undefined}
      className={className}
    >
      <RefreshCw aria-hidden className={cn(running && "animate-spin motion-reduce:animate-none")} />
      {!iconOnly && (running ? "Syncing…" : label)}
    </Button>
  );
  return iconOnly ? <Tooltip label={running ? "Syncing every account" : "Sync every account now"}>{button}</Tooltip> : button;
}
