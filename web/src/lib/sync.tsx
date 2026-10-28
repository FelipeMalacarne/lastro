import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { refreshAll, send, useResource } from "@/lib/api";
import { toast } from "@/lib/toast";
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
        if (reportOutcome) toast({ id: "sync", tone: s.ok ? "done" : "error", text: s.ok ? `Synced: ${s.message}.` : `Sync failed: ${s.message}.` });
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
      setError((e as Error).message);
      toast({ id: "sync", tone: "error", text: `Could not start the sync: ${(e as Error).message}.` });
    }
  }
  const last = res.data;
  const outcome = error ?? (last && !last.running && last.finished ? (last.ok ? `Last sync from here: ${last.message}.` : `Last sync from here failed: ${last.message}.`) : null);
  return { running, start, outcome, failed: Boolean(error || (last && !last.running && last.finished && !last.ok)) };
}

/** "Sync now": every bank, through Meu Pluggy, then prices and the day's valuation. */
export function SyncButton({ label = "Sync now", iconOnly, className }: { label?: string; iconOnly?: boolean; className?: string }) {
  const { running, start } = useSync();
  return (
    <Button
      variant={iconOnly ? "ghost" : "secondary"}
      size={iconOnly ? "icon-sm" : "sm"}
      onClick={() => void start()}
      disabled={running}
      aria-label={iconOnly ? (running ? "Syncing every account" : "Sync every account now") : undefined}
      title={iconOnly ? "Sync every account now" : undefined}
      className={className}
    >
      <RefreshCw aria-hidden className={cn(running && "animate-spin motion-reduce:animate-none")} />
      {!iconOnly && (running ? "Syncing…" : label)}
    </Button>
  );
}
