import { useId, useMemo, useState } from "react";

import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { SheetActions } from "@/components/ui/sheet";
import { send, type BudgetLine, type BudgetMonth, type Category } from "@/lib/api";
import { isShared, lineName, monthName, restore, saveLine, snapshot, type LineBody } from "@/lib/budget";
import { formatBRL, inputBRL, parseBRL } from "@/lib/format";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

type Kind = "limit" | "fund";

/**
 * A line's editor (canvas: "Budget: shared lines and funds", editing a line;
 * "Pots in one pool: Budget", adding a pot): a limit or a pot, its name, the
 * categories it covers (none for a pot like the emergency fund), its amount,
 * for a limit whether it rolls over and for a pot its target; a new pot also
 * takes what is already saved for it. Beside the list on desktop, in the
 * sheet on a phone.
 */
export function LineEditor({
  v,
  line,
  categories,
  byId,
  inSheet,
  onDone,
}: {
  v: BudgetMonth;
  /** The line to change; none for a new one. */
  line?: BudgetLine;
  categories: Category[];
  byId: Map<string, Category>;
  inSheet?: boolean;
  /** Closes the editor; removed when the line is gone. */
  onDone: (removed?: boolean) => void;
}) {
  const formId = useId();
  const [kind, setKind] = useState<Kind>(line?.fund ? "fund" : "limit");
  const [name, setName] = useState(line?.name ?? "");
  const [picked, setPicked] = useState<Set<string>>(() => new Set(line?.byCategory.map((p) => p.category) ?? []));
  const [amount, setAmount] = useState(line ? inputBRL(line.limitCents) : "");
  const [target, setTarget] = useState(line?.targetCents ? inputBRL(line.targetCents) : "");
  const [rollover, setRollover] = useState(line ? line.rollover : v.rolloverNew);
  const [saved, setSaved] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Where each category is now, so ticking one says what it leaves.
  const where = useMemo(() => {
    const m = new Map<string, BudgetLine>();
    for (const l of v.lines) for (const p of l.byCategory) m.set(p.category, l);
    return m;
  }, [v.lines]);
  const options = categories.filter((c) => c.budgeted && !c.archived);
  const cents = amount.trim() ? parseBRL(amount) : null;
  const savedCents = saved.trim() ? parseBRL(saved) : 0;
  const targetCents = target.trim() ? parseBRL(target) : 0;
  const shared = picked.size > 1;
  const bare = kind === "fund" && picked.size === 0;
  const named = shared || bare;
  const month = monthName(v.month);
  const problem =
    picked.size === 0 && kind === "limit"
      ? "Tick at least one category."
      : named && !name.trim()
        ? shared
          ? "A line with more than one category needs a name."
          : "A pot without categories needs a name."
        : cents === null
          ? `Type ${kind === "fund" ? "what goes in each month" : "the monthly limit"}, like 500,00.`
          : savedCents === null
            ? "Type what is already saved, like 750,00, or leave it empty."
            : kind === "fund" && (targetCents === null || targetCents < 0)
              ? "Type a target, like 15.000,00, or leave it empty."
              : null;

  function note(c: Category): { text: string; strong?: boolean } {
    const at = where.get(c.id);
    if (line && at?.id === line.id) return { text: "In this line" };
    if (!at) return { text: "No limit yet" };
    const own = !isShared(at);
    const what = own ? `its own ${at.fund ? "pot" : "limit"} of ${formatBRL(at.limitCents)}` : lineName(at, byId);
    return picked.has(c.id) ? { text: `Leaves ${what}`, strong: true } : { text: own ? `Has ${what}` : `In ${what}` };
  }

  function toggle(id: string, on: boolean) {
    setPicked((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (problem || cents === null) return;
    const before = snapshot(v);
    const body: LineBody = {
      name: named ? name.trim() : "",
      categories: [...picked],
      limitCents: cents,
      rollover: kind === "limit" && rollover,
      fund: kind === "fund",
      targetCents: kind === "fund" && targetCents ? targetCents : null,
    };
    const moved = [...picked].filter((id) => where.has(id) && where.get(id)!.id !== line?.id).map((id) => byId.get(id)?.name ?? "A category");
    const called = named ? body.name : (byId.get(body.categories[0])?.name ?? "The line");
    setBusy(true);
    setError(null);
    try {
      if (line) {
        await saveLine(v.month, line.id, body);
      } else {
        const r = await send<{ id: string }>("POST", `/api/v1/budget/${v.month}/lines`, body);
        if (kind === "fund" && savedCents) await send("PUT", `/api/v1/budget/${v.month}/lines/${r.id}/adjustment`, { amountCents: savedCents });
      }
      toast({
        id: "budget",
        text: (
          <>
            <strong>{called}</strong> {line ? "saved" : "added"}
            {moved.length > 0 && `; ${moved.join(", ")} moved into it`}.
          </>
        ),
        undo: () => restore(v.month, before),
      });
      onDone();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!line) return;
    const before = snapshot(v);
    const called = lineName(line, byId);
    setBusy(true);
    setError(null);
    try {
      await send("DELETE", `/api/v1/budget/${v.month}/lines/${line.id}`);
      toast({
        id: "budget",
        text: (
          <>
            <strong>{called}</strong> has no {line.fund ? "pot" : "limit"} from {month} on.
          </>
        ),
        undo: () => restore(v.month, before),
      });
      onDone(true);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const title = line ? lineName(line, byId) : kind === "fund" ? "A new pot" : "A new line";
  const body = (
    <form id={formId} onSubmit={save} className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <SegmentedControl
          label="Kind"
          value={kind}
          onChange={setKind}
          options={[
            { value: "limit", label: "Limit" },
            { value: "fund", label: "Pot" },
          ]}
        />
        <span className="type-caption text-ink-muted">
          {kind === "fund"
            ? "A pot sets money apart, kept invested, and carries its balance, debt included: spending that comes in lumps, the emergency fund, a goal."
            : "A limit is for spending within the month."}
        </span>
      </div>
      {named && (
        <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} maxLength={40} hint="Needed when a pot has no category or a line more than one." />
      )}
      <fieldset className="m-0 flex flex-col gap-2 border-0 p-0">
        <legend className="mb-1.5 p-0 text-[14px] leading-5 font-semibold">
          Categories{kind === "fund" && <span className="font-normal text-ink-muted"> · optional for a pot</span>}
        </legend>
        <div className="grid gap-x-4 gap-y-3 sm:grid-cols-2">
          {options.map((c) => {
            const n = note(c);
            return (
              <label key={c.id} className="flex cursor-pointer items-start gap-2.5">
                <Checkbox checked={picked.has(c.id)} onCheckedChange={(on) => toggle(c.id, on === true)} className="mt-px" />
                <span className="flex min-w-0 flex-col">
                  <span className="text-[14px] leading-5 font-semibold">{c.name}</span>
                  <span className={cn("type-caption", n.strong ? "text-ink" : "text-ink-muted")}>{n.text}</span>
                </span>
              </label>
            );
          })}
        </div>
        {bare ? (
          <span className="type-caption text-ink-muted">None ticked: money that leaves only when you move it, like the emergency fund or a goal.</span>
        ) : (
          !shared && <span className="type-caption text-ink-muted">Tick more to share one {kind === "fund" ? "pot" : "limit"}, with a name for it.</span>
        )}
      </fieldset>
      <TextField
        label={kind === "fund" ? "Each month" : "Monthly limit"}
        prefix="R$"
        numeric
        inputMode="decimal"
        value={amount}
        onChange={(e) => setAmount(e.target.value)}
        error={amount.trim() && cents === null ? "Type an amount, like 500,00" : undefined}
        hint={kind === "fund" ? (bare ? "Zero is fine: it can grow only with the money you move into it." : "Invest it when the salary lands, wherever the pot is kept.") : `From ${month} on; earlier months keep their lines.`}
      />
      {kind === "fund" && (
        <TextField
          label="Target"
          prefix="R$"
          numeric
          inputMode="decimal"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          error={target.trim() && targetCents === null ? "Type an amount, like 15.000,00" : undefined}
          hint="Optional. Once reached, the pot stops taking its monthly amount; spending below it asks again."
        />
      )}
      {kind === "limit" ? (
        <label className="flex cursor-pointer items-start gap-2.5">
          <Checkbox checked={rollover} onCheckedChange={(on) => setRollover(on === true)} className="mt-px" />
          <span className="flex flex-col">
            <span className="text-[14px] leading-5 font-semibold">Roll over what is left</span>
            <span className="type-caption text-ink-muted">
              {line && line.carriedCents > 0 ? `The ${formatBRL(line.carriedCents)} carried into ${month} stays with the line.` : "Whatever remains at the end of a month is added to the next one's limit."}
            </span>
          </span>
        </label>
      ) : (
        <>
          {!line && (
            <TextField
              label="Already saved for it"
              prefix="R$"
              numeric
              inputMode="decimal"
              value={saved}
              onChange={(e) => setSaved(e.target.value)}
              error={saved.trim() && savedCents === null ? "Type an amount, like 750,00" : undefined}
              hint="What the Caixinhas already hold for this. It starts the balance; pay yourself first does not change."
            />
          )}
          <p className="m-0 rounded-md bg-paper-sunken p-3 type-caption text-ink-muted">A pot carries its balance from month to month, debt included, so it never rolls over.</p>
        </>
      )}
      {error && (
        <p role="alert" className="m-0 type-caption text-danger">
          Could not save: {error}.
        </p>
      )}
      {!error && problem && (picked.size > 0 || bare) && (amount.trim() || named) && <p className="m-0 type-caption text-ink-muted">{problem}</p>}
      <SheetActions className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex gap-2">
          <Button type="submit" form={formId} variant="primary" size="sm" disabled={busy || Boolean(problem)}>
            {line ? "Save line" : kind === "fund" ? "Add pot" : "Add line"}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => onDone()}>
            Cancel
          </Button>
        </span>
        {line && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void remove()}>
            Remove {line.fund ? "pot" : "line"}
          </Button>
        )}
      </SheetActions>
    </form>
  );

  if (inSheet) return body;
  return <Panel title={title}>{body}</Panel>;
}
