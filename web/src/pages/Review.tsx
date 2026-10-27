import { useEffect, useState } from "react";
import { Check, RotateCcw, Sparkles } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryDecision } from "@/components/CategoryDecision";
import { Kbd } from "@/components/Kbd";
import { Panel } from "@/components/Panel";
import { Button } from "@/components/ui/button";
import { joinUndo, send, useResource, type Change, type Pattern, type Review as ReviewData, type ReviewGroup, type Undo } from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, formatDay } from "@/lib/format";
import { useWide } from "@/lib/media";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";

/** The review queue (canvas: "Transactions and review", "Review queue, phone"). */
export function Review() {
  const res = useResource<ReviewData>("/api/v1/review");
  const { live, byId } = useCategories();
  const wide = useWide();
  const [at, setAt] = useState(0);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const [undo, setUndo] = useState<{ token: Undo; what: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const groups = res.data?.groups ?? [];
  const index = Math.min(at, Math.max(groups.length - 1, 0));
  const group = groups[index];

  async function takeBack() {
    if (!undo || busy) return;
    setBusy(true);
    setError(null);
    try {
      await send("POST", "/api/v1/undo", undo.token);
      setDone(`Undone. ${undo.what}`);
      setUndo(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  // J and K move through the groups, S skips one, U undoes the last choice.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      if (e.metaKey || e.ctrlKey || e.altKey || t.closest("input, textarea, select, [contenteditable], [role=menu]")) return;
      if (e.key === "u") {
        e.preventDefault();
        void takeBack();
      } else if (e.key === "j" || e.key === "ArrowDown" || e.key === "s") {
        e.preventDefault();
        setAt((i) => Math.min(i + 1, groups.length - 1));
      } else if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        setAt((i) => Math.max(i - 1, 0));
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  async function choose(g: ReviewGroup, category: string, teach: Pattern | null) {
    setBusy(true);
    setError(null);
    try {
      let n: number;
      let token: Undo | null = null;
      if (teach) {
        const r = await send<Change>("POST", "/api/v1/review", { pattern: teach, category });
        n = r.categorized;
        token = r.undo;
      } else {
        for (const t of g.transactions) token = joinUndo(token, (await send<Change>("PUT", `/api/v1/transactions/${t.id}/category`, { category })).undo);
        n = g.transactions.length;
      }
      const name = byId.get(category)?.name ?? "the category";
      const many = `${n} ${n === 1 ? "transaction" : "transactions"}`;
      setDone(`${name}: ${many}${teach ? `, and a rule for “${teach.text}”` : ""}.`);
      if (token)
        setUndo({
          token,
          what: teach
            ? `The rule for “${teach.text}” is ${token.wasRule ? "back to its old category" : "gone"} and its ${many} are back as they were.`
            : `The ${many} are back in the queue.`,
        });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const waiting = res.data?.waiting ?? 0;
  const caption = res.data
    ? waiting
      ? `${waiting} ${waiting === 1 ? "transaction" : "transactions"} in ${groups.length} ${groups.length === 1 ? "group" : "groups"} · one choice settles a whole group`
      : "Nothing waiting"
    : undefined;

  return (
    <>
      <PageHeader title="To review" caption={caption} monthless />

      {(done || error) && (
        <div
          role="status"
          className={cn(
            "flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md py-2 pr-2 pl-3.5 text-[14px] leading-5",
            error ? "bg-danger-soft" : undo ? "bg-inflow-soft" : "bg-paper-sunken",
          )}
        >
          {!error && (undo ? <Check className="size-4 text-inflow" aria-hidden /> : <RotateCcw className="size-4 text-ink-muted" aria-hidden />)}
          <span className="min-w-0 flex-[1_1_320px]">{error ? `Could not save: ${error}.` : done}</span>
          {undo && !error && (
            <Button variant="ghost" size="sm" onClick={() => void takeBack()} disabled={busy}>
              Undo {wide && <Kbd>U</Kbd>}
            </Button>
          )}
        </div>
      )}

      {res.error && !res.data && <p className="m-0 type-body text-ink-muted">Could not load the queue: {res.error}.</p>}
      {!res.data && !res.error && <p className="m-0 type-body text-ink-muted">Loading the queue…</p>}

      {res.data && groups.length === 0 && (
        <Panel>
          <div className="flex flex-col items-start gap-3">
            <p className="m-0 type-body">Nothing to review. Every transaction has a category.</p>
            <Button asChild size="sm">
              <Link to="/transactions">Back to Transactions</Link>
            </Button>
          </div>
        </Panel>
      )}

      {group && (
        <div className="flex flex-wrap items-start gap-4 lg:gap-6">
          <Panel
            title={wide ? "Waiting" : undefined}
            className={cn("min-w-0 flex-[1_1_420px]", !wide && "order-2")}
            bodyClassName="-mx-5 border-t border-rule"
            footer={
              wide && (
                <span className="flex flex-wrap items-center gap-x-4 gap-y-1">
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>J</Kbd>
                    <Kbd>K</Kbd> move
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>1</Kbd>…<Kbd>9</Kbd> choose
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>S</Kbd> skip
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <Kbd>U</Kbd> undo
                  </span>
                </span>
              )
            }
          >
            <ul role="list" className="m-0 list-none p-0">
              {groups.map((g, i) => (
                <li key={(g.pattern?.text ?? "") + g.transactions[0]?.id}>
                  <button
                    type="button"
                    aria-current={i === index ? "true" : undefined}
                    onClick={() => {
                      setAt(i);
                      if (!wide) window.scrollTo({ top: 0, behavior: "smooth" });
                    }}
                    className={cn(
                      "grid min-h-16 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 border-0 border-b border-rule px-5 py-2.5 text-left focus-visible:focus-ring",
                      i === index ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken",
                    )}
                  >
                    <span className="flex min-w-0 flex-col gap-0.5">
                      <span className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-figure text-[14px] leading-5 font-medium">{g.pattern?.text ?? g.transactions[0]?.description}</span>
                        {g.count > 1 && <Badge figure>{g.count}×</Badge>}
                      </span>
                      <span className="truncate type-caption text-ink-muted">
                        {g.suggestion && byId.get(g.suggestion.category) ? (
                          <span className="inline-flex items-center gap-1">
                            <Sparkles className="size-3.5" aria-hidden />
                            Suggested: {byId.get(g.suggestion.category)?.name}
                          </span>
                        ) : null}
                        {g.suggestion && byId.get(g.suggestion.category) ? " · " : null}
                        {examples(g)} · latest {formatDay(g.latest)}
                      </span>
                    </span>
                    <Amount cents={g.totalCents} sign="always" />
                  </button>
                </li>
              ))}
            </ul>
          </Panel>

          <section
            aria-label="Decide"
            className={cn(
              "flex min-w-0 flex-[1_1_380px] flex-col gap-5 rounded-md border border-rule bg-paper-raised p-5 lg:sticky lg:top-6 lg:max-w-[460px]",
              !wide && "order-1",
            )}
          >
            <div className="flex flex-col gap-1">
              <span className="type-figure-sm text-ink-muted">
                {index + 1} of {groups.length} · latest {formatDay(group.latest)}
              </span>
              <div className="flex flex-wrap items-baseline justify-between gap-3">
                <h2 className="m-0 font-figure text-[20px] leading-7 font-medium">{group.pattern?.text ?? group.transactions[0]?.description}</h2>
                <Amount cents={group.totalCents} size="lg" sign="always" />
              </div>
              <span className="type-caption text-ink-muted">
                {group.count} {group.count === 1 ? "transaction" : "transactions"}
                {group.count > 1 ? `, ${formatBRL(Math.abs(group.totalCents / group.count))} on average` : ""}
              </span>
            </div>

            <ul role="list" className="m-0 flex list-none flex-col p-0">
              {group.transactions.map((t) => (
                <li key={t.id} className="grid grid-cols-[52px_minmax(0,1fr)_auto] items-center gap-x-2.5 border-b border-rule py-2 last:border-b-0">
                  <span className="type-figure-sm text-ink-muted">{formatDay(t.date)}</span>
                  <span className="truncate font-figure text-[12px] leading-4">{t.description}</span>
                  <Amount cents={t.amountCents} size="sm" />
                </li>
              ))}
              {group.count > group.transactions.length && (
                <li className="pt-2 type-caption text-ink-muted">and {group.count - group.transactions.length} more like these</li>
              )}
            </ul>

            <CategoryDecision
              categories={live}
              suggested={group.pattern}
              suggestion={group.suggestion}
              onChoose={(c, teach) => choose(group, c, teach)}
              keyboard={wide}
              busy={busy}
              reach={group.pattern ? `Settles these ${group.count} and every future match` : undefined}
            />

            <div className="flex flex-wrap gap-2 border-t border-rule pt-4">
              <Button variant="ghost" size="sm" onClick={() => setAt(Math.min(index + 1, groups.length - 1))} disabled={index >= groups.length - 1}>
                Skip for now {wide && <Kbd>S</Kbd>}
              </Button>
            </div>
          </section>
        </div>
      )}
    </>
  );
}

/** "PAG*IFOOD SP, PG *IFOOD": the distinct descriptions behind a group. */
function examples(g: ReviewGroup): string {
  const seen = [...new Set(g.transactions.map((t) => t.description))];
  return seen.slice(0, 2).join(", ") + (seen.length > 2 ? "…" : "");
}
