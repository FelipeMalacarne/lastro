import { useId, useState } from "react";
import { PiggyBank, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { SheetActions } from "@/components/ui/sheet";
import { send, useResource, type BudgetLine, type BudgetMonth, type Category, type TransactionList } from "@/lib/api";
import { isBare, lineBody, lineColors, lineName, monthName, restore, saveLine, snapshot } from "@/lib/budget";
import { addMonths, formatBRL, formatDay, formatMonth, inputBRL, parseBRL, titleCase } from "@/lib/format";
import { Link } from "@/lib/router";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

/**
 * A pot (ADR-0011, ADR-0014; canvas: "Budget: shared lines and funds",
 * "Budget, phone: a fund in a sheet", "Pots in one pool: Budget"): its
 * balance against its target and where it comes from, its last six months,
 * covering it from investments while in the red, its amount, its target and
 * the money moved into or out of it this month.
 */
export function FundDetail({
  v,
  l,
  byId,
  inSheet,
  onEdit,
}: {
  v: BudgetMonth;
  l: BudgetLine;
  byId: Map<string, Category>;
  inSheet?: boolean;
  onEdit: () => void;
}) {
  const formId = useId();
  const name = lineName(l, byId);
  const colors = lineColors(l, byId);
  const month = monthName(v.month);
  const red = l.leftCents < 0;
  const bare = isBare(l);
  const [text, setText] = useState(inputBRL(l.limitCents));
  const [targetText, setTargetText] = useState(l.targetCents ? inputBRL(l.targetCents) : "");
  const [movedText, setMovedText] = useState(l.adjustedCents ? signedInput(l.adjustedCents) : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cents = parseBRL(text);
  const target = targetText.trim() ? parseBRL(targetText) : 0;
  const moved = movedText.trim() ? parseSigned(movedText) : 0;
  const changed = cents !== l.limitCents || (target || null) !== l.targetCents || moved !== l.adjustedCents;
  const bad = cents === null || target === null || moved === null;
  const toGo = l.targetCents ? Math.max(l.targetCents - l.leftCents, 0) : 0;
  const pct = l.targetCents ? Math.max(0, Math.min(100, Math.round((l.leftCents * 100) / l.targetCents))) : 0;
  // Months of its amount until a fund in the red is back above zero.
  const back = red && l.limitCents > 0 ? addMonths(v.month, Math.ceil(-l.leftCents / l.limitCents)) : null;

  async function run(f: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await f();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  // Money already invested moves into the fund: an adjustment, which neither
  // Not assigned nor pay yourself first sees.
  const cover = () =>
    run(async () => {
      const was = l.adjustedCents;
      const url = `/api/v1/budget/${v.month}/lines/${l.id}/adjustment`;
      await send("PUT", url, { amountCents: was - l.leftCents });
      toast({
        id: "budget",
        text: (
          <>
            <strong>{name}</strong> covered with {formatBRL(-l.leftCents)} from investments.
          </>
        ),
        undo: () => (was === 0 ? send("DELETE", url) : send("PUT", url, { amountCents: was })),
      });
    });

  const remove = () =>
    run(async () => {
      const before = snapshot(v);
      await send("DELETE", `/api/v1/budget/${v.month}/lines/${l.id}`);
      toast({
        id: "budget",
        text: (
          <>
            <strong>{name}</strong> has no pot from {month} on.
          </>
        ),
        undo: () => restore(v.month, before),
      });
    });

  return (
    <section aria-labelledby={inSheet ? undefined : "fund-title"} className={cn("flex flex-col gap-4", !inSheet && "rounded-md border border-rule bg-paper-raised p-5")}>
      <div className={cn("flex flex-wrap items-center justify-between gap-2", inSheet && "empty:hidden")}>
        {!inSheet && (
          <h2 id="fund-title" className="m-0 flex items-center gap-2 type-heading">
            {bare && <PiggyBank className="size-4 text-ink-muted" aria-hidden />}
            <span className="inline-flex gap-0.5" aria-hidden>
              {colors.map((c, i) => (
                <span key={i} className={cn("h-3 rounded-xs", colors.length > 1 ? "w-1.5" : "w-3")} style={{ background: `var(--${c})` }} />
              ))}
            </span>
            {name}
          </h2>
        )}
        {red ? (
          <Badge tone="warning" icon={TriangleAlert}>
            In the red
          </Badge>
        ) : (
          <Badge icon={PiggyBank}>{l.targetCents && toGo === 0 ? "Reached" : "Pot"}</Badge>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <span className="flex flex-wrap items-baseline gap-2">
          <Amount cents={l.leftCents} size="lg" tone={red ? "warning" : "ink"} />
          <span className="type-caption text-ink-muted">{l.targetCents ? `of ${formatBRL(l.targetCents)}` : `after ${month}'s spending`}</span>
        </span>
        {l.targetCents !== null && (
          <span
            role="meter"
            aria-label={`${name}: ${pct}% of its target`}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={pct}
            className="relative h-2.5 rounded-xs bg-paper-sunken shadow-[inset_0_0_0_1px_var(--rule)]"
          >
            <span className="absolute inset-y-0 left-0 rounded-xs bg-tinta" style={{ width: `${pct}%` }} />
          </span>
        )}
        <span className="type-caption text-ink-muted">
          {l.targetCents ? (toGo > 0 ? `${formatBRL(toGo)} to go. ` : "Reached: it takes nothing this month. ") : ""}
          {l.limitCents > 0 ? `${formatBRL(l.limitCents)} a month` : "No monthly amount: it grows only with the money you move into it."}
          {back && `: back above zero in ${monthName(back)}`}
        </span>
      </div>

      <div role="table" aria-label={`Where ${name}'s balance comes from`} className="flex flex-col type-caption">
        <Row label={`Saved by ${monthName(addMonths(v.month, -1))}`} cents={l.carriedCents} />
        <Row label={`${month}'s amount`} cents={l.amountCents} sign="always" />
        {l.adjustedCents !== 0 && <Row label="Moved from investments" cents={l.adjustedCents} sign="always" />}
        {l.byCategory.map((p) => (
          <FundTransactions key={p.category} month={v.month} category={p.category} />
        ))}
        <div role="row" className="flex justify-between border-t border-ink py-1.5 font-semibold">
          <span role="cell">Balance</span>
          <Amount cents={l.leftCents} size="sm" tone={red ? "warning" : "ink"} />
        </div>
      </div>

      {l.balances && l.balances.length > 1 && <Balances name={name} balances={l.balances} color={colors[0]} />}

      {l.spentCents > 0 && (
        <p className="m-0 flex gap-2 rounded-md bg-paper-sunken p-3 text-[14px] leading-5">
          <PiggyBank className="mt-0.5 size-4 flex-none text-ink-muted" aria-hidden />
          <span>
            What {name} paid in {month}, <strong>{formatBRL(l.spentCents)}</strong>, left pay yourself first as it was: the pot paid it.
          </span>
        </p>
      )}

      <form
        id={formId}
        className="flex flex-col gap-3 border-t border-rule pt-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (bad || !changed) return;
          const before = snapshot(v);
          const was = l.adjustedCents;
          const url = `/api/v1/budget/${v.month}/lines/${l.id}/adjustment`;
          void run(async () => {
            if (cents !== l.limitCents || (target || null) !== l.targetCents) {
              await saveLine(v.month, l.id, lineBody(l, { limitCents: cents, targetCents: target || null }));
            }
            if (moved !== was) await (moved === 0 ? send("DELETE", url) : send("PUT", url, { amountCents: moved }));
            toast({
              id: "budget",
              text: (
                <>
                  <strong>{name}</strong> saved from {month} on.
                </>
              ),
              undo: async () => {
                await restore(v.month, before);
                if (moved !== was) await (was === 0 ? send("DELETE", url) : send("PUT", url, { amountCents: was }));
              },
            });
          });
        }}
      >
        {red && (
          <div className="flex flex-col gap-1.5">
            <Button type="button" size="sm" className="self-start" disabled={busy} onClick={() => void cover()}>
              Cover {formatBRL(-l.leftCents)} from investments
            </Button>
            <span className="type-caption text-ink-muted">Moves money already invested into the pot, at once and with Undo. Pay yourself first and Not assigned stay as they are.</span>
          </div>
        )}
        <TextField
          label="Each month"
          prefix="R$"
          numeric
          inputMode="decimal"
          value={text}
          onChange={(e) => setText(e.target.value)}
          error={text && cents === null ? "Type an amount, like 300,00" : undefined}
          hint={`From ${month} on; earlier months keep theirs.${l.targetCents ? " Stops once the target is reached." : ""}`}
        />
        <TextField
          label="Target"
          prefix="R$"
          numeric
          inputMode="decimal"
          value={targetText}
          onChange={(e) => setTargetText(e.target.value)}
          error={targetText.trim() && target === null ? "Type an amount, like 15.000,00" : undefined}
          hint="Optional. Leave it empty for a pot that only saves up."
        />
        <TextField
          label={`Move in or out in ${month}`}
          prefix="R$"
          numeric
          inputMode="decimal"
          value={movedText}
          onChange={(e) => setMovedText(e.target.value)}
          error={movedText.trim() && moved === null ? "Type an amount, like 500,00 or −500,00" : undefined}
          hint="Money already invested: positive moves it in, negative takes it out. Pay yourself first and Not assigned stay as they are."
        />
        {error && (
          <p role="alert" className="m-0 type-caption text-danger">
            Could not save: {error}.
          </p>
        )}
        <SheetActions className="flex flex-wrap items-center gap-2">
          <Button type="submit" form={formId} size="sm" disabled={busy || bad || !changed}>
            Save pot
          </Button>
          <Button type="button" variant="ghost" size="sm" disabled={busy} data-opener="edit-line" onClick={onEdit}>
            Edit line
          </Button>
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void remove()}>
            Remove pot
          </Button>
        </SheetActions>
      </form>

      {!bare && (
        <Link to={`/transactions?month=${v.month}&category=${l.byCategory[0]?.category ?? ""}`} className="self-start text-[14px] font-semibold">
          See the {l.count} {l.count === 1 ? "transaction" : "transactions"}
        </Link>
      )}
    </section>
  );
}

/** An amount that may be negative, "−500,00" or "-500,00"; null when it is not one. */
function parseSigned(s: string): number | null {
  const t = s.trim();
  const neg = /^[-−]/.test(t);
  const c = parseBRL(neg ? t.slice(1) : t);
  return c === null ? null : neg ? -c : c;
}

/** −50000 → "−500,00", for an amount field that takes a sign. */
const signedInput = (cents: number) => (cents < 0 ? `−${inputBRL(-cents)}` : inputBRL(cents));

function Row({ label, cents, sign = "negative" }: { label: string; cents: number; sign?: "negative" | "always" }) {
  return (
    <div role="row" className="flex justify-between gap-3 border-t border-rule py-1.5">
      <span role="cell" className="min-w-0 truncate text-ink-muted">
        {label}
      </span>
      <Amount cents={cents} sign={sign} tone={sign === "always" && cents > 0 ? "inflow" : "ink"} size="sm" />
    </div>
  );
}

/** A category's movements this month in its fund: what it paid, and refunds back into it. */
function FundTransactions({ month, category }: { month: string; category: string }) {
  const list = useResource<TransactionList>(`/api/v1/transactions?month=${month}&category=${category}`);
  const txs = (list.data?.transactions ?? []).filter((t) => t.kind === "expense" || t.kind === "income");
  const shown = txs.slice(0, 5);
  const rest = txs.slice(5);
  return (
    <>
      {shown.map((t) => (
        <Row key={t.id} label={`${titleCase(t.description)}, ${formatDay(t.date)}`} cents={t.amountCents} sign="always" />
      ))}
      {rest.length > 0 && <Row label={`${rest.length} more`} cents={rest.reduce((s, t) => s + t.amountCents, 0)} sign="always" />}
    </>
  );
}

/** A fund's balance at each month's end: bars above zero, and below it while in the red. */
function Balances({ name, balances, color }: { name: string; balances: { month: string; balanceCents: number }[]; color?: string }) {
  const up = Math.max(1, ...balances.map((b) => b.balanceCents));
  const down = Math.max(0, ...balances.map((b) => -b.balanceCents));
  const scale = Math.max(up, down);
  const label = `${name}'s balance at the end of each month: ${balances.map((b) => `${formatMonth(b.month, true)} ${formatBRL(b.balanceCents)}`).join(", ")}.`;
  return (
    <div className="flex flex-col gap-2">
      <span className="type-label text-ink-muted">Balance, last {balances.length} months</span>
      <div role="img" aria-label={label} className="grid gap-1.5" style={{ gridTemplateColumns: `repeat(${balances.length}, minmax(0, 1fr))` }}>
        {balances.map((b, i) => {
          const now = i === balances.length - 1;
          return (
            <span key={b.month} className="flex flex-col">
              <span className="flex h-16 flex-col justify-end border-b border-ink">
                {b.balanceCents > 0 && (
                  <span className="rounded-t-xs" style={{ height: `${(b.balanceCents / scale) * 100}%`, background: now ? `var(--${color ?? "note-200"})` : "var(--rule-strong)" }} />
                )}
              </span>
              <span className="flex h-8 flex-col">
                {b.balanceCents < 0 && <span className="rounded-b-xs bg-warning" style={{ height: `${(-b.balanceCents / scale) * 200}%`, maxHeight: "100%" }} />}
              </span>
            </span>
          );
        })}
      </div>
      <div className="grid gap-1.5 type-figure-sm text-ink-muted" style={{ gridTemplateColumns: `repeat(${balances.length}, minmax(0, 1fr))` }}>
        {balances.map((b) => (
          <span key={b.month} className="text-center">
            {formatMonth(b.month, true).slice(0, 3)}
          </span>
        ))}
      </div>
      <span className="inline-flex items-center gap-1.5 type-caption text-ink-muted">
        <span className="w-4 border-t border-ink" aria-hidden />
        Zero: below it, the months after pay it back
      </span>
    </div>
  );
}
