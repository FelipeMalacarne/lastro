import { useEffect, useState } from "react";
import { Check, ChevronLeft, CircleX, Clock, Copy, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { Panel } from "@/components/Panel";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { SkeletonRows } from "@/components/ui/skeleton";
import {
  assetClasses,
  send,
  useResource,
  type AssetClass,
  type BudgetMonth,
  type ContributionPlan,
  type PlannedPurchase,
  type PortfolioOverview,
} from "@/lib/api";
import { addMonths, formatBRL, formatDay, formatMonth, formatPercent, inputBRL, monthOf, parseBRL } from "@/lib/format";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";
import { classOf } from "@/pages/Portfolio";

/** The next payday after today, as "2026-11-05". */
function nextPayday(payday: number): string {
  const now = new Date();
  let d = new Date(now.getFullYear(), now.getMonth(), Math.min(payday, 28));
  if (d <= now) d = new Date(now.getFullYear(), now.getMonth() + 1, Math.min(payday, 28));
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const why: Record<string, (s: ContributionPlan["splits"][number], band: number) => string> = {
  below_band: (s, band) => `Below its band: first up to ${formatPercent(s.targetBp - band, { decimals: 0 })}, then its share of the gap.`,
  gap: (s) => (s.targetBp - s.beforeBp < 10 ? "On its target: keeps its share as the total grows." : `${formatPoints(s.targetBp - s.beforeBp)} under target: its share of the gap.`),
  target: () => "Every class at its target: split by the targets.",
  above_band: () => "Above its band: no new money this time.",
  at_target: () => "At its target: nothing this time.",
};

function formatPoints(bp: number): string {
  const p = Math.abs(bp) / 100;
  return `${p.toFixed(1).replace(".", ",")} ${p.toFixed(1) === "1.0" ? "point" : "points"}`;
}

/** Where to contribute (canvas: "Where to contribute", and its phone version). */
export function Contribute() {
  // The next contribution is this month's until its pay yourself first is
  // done, then next month's (each month inherits the budget before it).
  const thisRes = useResource<BudgetMonth>(`/api/v1/budget?month=${monthOf(new Date())}`);
  const thisMonth = thisRes.data;
  const nextMonth = useResource<BudgetMonth>(thisMonth?.payYourselfFirst.doneOn ? `/api/v1/budget?month=${addMonths(monthOf(new Date()), 1)}` : null).data;
  const budget = thisMonth?.payYourselfFirst.doneOn ? nextMonth : thisMonth;
  const overview = useResource<PortfolioOverview>("/api/v1/portfolio").data;
  const [text, setText] = useState<string | null>(null);
  const [cents, setCents] = useState<number | null>(null);

  // The amount starts as this month's pay yourself first.
  const planned = budget?.payYourselfFirst.plannedCents ?? 0;
  const shown = text ?? (planned ? inputBRL(planned) : "");
  const parsed = parseBRL(shown);
  useEffect(() => {
    const t = setTimeout(() => setCents(parsed), 250);
    return () => clearTimeout(t);
  }, [parsed]);
  const planRes = useResource<ContributionPlan>(cents !== null ? `/api/v1/contribute?amount=${cents}` : null);
  const plan = planRes.data;
  const payday = budget ? nextPayday(budget.payday) : null;

  return (
    <>
      <div className="flex flex-col gap-1">
        <Link to="/portfolio" className="inline-flex items-center gap-1 self-start text-[13px] font-semibold no-underline">
          <ChevronLeft className="size-3.5" aria-hidden /> Portfolio
        </Link>
        <PageHeader
          title="Where to contribute"
          caption={[payday ? `Next contribution on ${formatDay(payday)}` : null, overview?.asOf ? `prices as of ${formatDay(overview.asOf)}` : null].filter(Boolean).join(" · ") || undefined}
          monthless
          actions={
            <Button asChild variant="ghost">
              <Link to="/settings#investments">Edit targets</Link>
            </Button>
          }
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Panel className="lg:p-6" footer="Classes outside their band come first; the rest is split by how far each class is below its target. lastro never suggests selling.">
          <div className="flex flex-col gap-5">
            <div className="grid gap-4 sm:grid-cols-[minmax(0,240px)_minmax(0,1fr)] sm:items-start">
              <TextField
                label="Amount to contribute"
                prefix="R$"
                numeric
                inputMode="decimal"
                value={shown}
                onChange={(e) => setText(e.target.value)}
                error={shown && parsed === null ? "Type an amount, like 3.000,00" : undefined}
                hint={planned && text === null && budget ? `Your pay yourself first for ${formatMonth(budget.month).split(" ")[0]}.` : undefined}
              />
              {plan && <p className="m-0 type-body sm:pt-6">{summary(plan)}</p>}
            </div>
            {plan && <Split plan={plan} />}
            {plan && payday && <PlanActions plan={plan} payday={payday} />}
            {!plan && planRes.error && <p className="m-0 type-body text-ink-muted">Could not load the split: {planRes.error}.</p>}
            {!plan && cents === null && (
              <p className="m-0 type-body text-ink-muted">
                {thisRes.error && !thisMonth && text === null && `Could not load your pay yourself first: ${thisRes.error}. `}
                Type an amount to see where it goes.
              </p>
            )}
          </div>
        </Panel>
        {plan && <Moves plan={plan} />}
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Schedule />
        {plan && <Terminal plan={plan} />}
      </div>
    </>
  );
}

function names(list: AssetClass[]): string {
  const n = list.map((c) => classOf(c).name);
  return n.length <= 1 ? n.join("") : n.slice(0, -1).join(", ") + " and " + n.at(-1);
}

/** One sentence on why the split looks the way it does. */
function summary(p: ContributionPlan): string {
  const below = p.splits.filter((s) => s.why === "below_band").map((s) => s.class);
  const above = p.splits.filter((s) => s.why === "above_band").map((s) => s.class);
  const parts: string[] = [];
  if (below.length) parts.push(`${names(below)} ${below.length === 1 ? "is" : "are"} below ${below.length === 1 ? "its" : "their"} band, so ${below.length === 1 ? "it comes" : "they come"} first.`);
  if (above.length) parts.push(`${names(above)} ${above.length === 1 ? "is" : "are"} above ${above.length === 1 ? "its" : "their"} band and ${above.length === 1 ? "gets" : "get"} nothing this time.`);
  if (!parts.length) parts.push(p.splits.every((s) => s.why === "target") ? "Every class is at its target, so the money follows the targets." : "Every class is inside its band: the money goes where each is furthest under its target.");
  return parts.join(" ");
}

function Split({ plan }: { plan: ContributionPlan }) {
  return (
    <div role="list" aria-label={`Split of ${formatBRL(plan.amountCents)}`} className="flex flex-col">
      {plan.splits.map((s) => {
        const k = classOf(s.class);
        const none = s.amountCents === 0;
        return (
          <div key={s.class} role="listitem" className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 border-t border-rule py-3">
            <span className="flex min-w-0 flex-col gap-1">
              <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <CategoryTag name={k.name} color={k.color} />
                {s.buy && !none && <span className="truncate text-[14px] leading-5 font-semibold">{s.buy}</span>}
              </span>
              <span className={cn("type-caption", none ? "text-ink-muted" : "text-ink")}>{why[s.why](s, plan.bandBp)}</span>
            </span>
            <Amount cents={s.amountCents} size="lg" sign="none" tone={none ? "muted" : "ink"} />
          </div>
        );
      })}
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 border-t-2 border-ink pt-3">
        <span className="flex flex-col">
          <span className="text-[14px] font-semibold">Total</span>
          <span className="inline-flex items-center gap-1 type-caption text-inflow">
            <Check className="size-3.5" aria-hidden /> Adds up to the amount. Nothing to sell.
          </span>
        </span>
        <Amount cents={plan.amountCents} size="lg" sign="none" />
      </div>
    </div>
  );
}

/** Puts the split on the schedule for payday, or copies it. */
function PlanActions({ plan, payday }: { plan: ContributionPlan; payday: string }) {
  const [state, setState] = useState<"idle" | "busy" | "saved" | "copied" | string>("idle");
  const scheduled = useResource<{ purchases: PlannedPurchase[] }>("/api/v1/purchases").data?.purchases ?? [];
  const existing = scheduled.filter((p) => p.status === "planned" && p.plannedOn === payday);
  const already = state !== "saved" && existing.length > 0;
  const monthName = formatMonth(payday.slice(0, 7)).split(" ")[0];
  async function save() {
    setState("busy");
    try {
      // A new split replaces the one already on the schedule for that day; the old one stays in the history, cancelled.
      for (const p of existing) await send("PATCH", `/api/v1/purchases/${p.id}`, { status: "cancelled", note: "Replaced by a new plan" });
      for (const s of plan.splits.filter((s) => s.amountCents > 0)) {
        await send("POST", "/api/v1/purchases", {
          plannedOn: payday,
          what: s.buy ?? classOf(s.class).name,
          class: s.class,
          amountCents: s.amountCents,
          condition: "On payday",
        });
      }
      setState("saved");
    } catch (e) {
      setState((e as Error).message);
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(terminalText(plan).join("\n"));
      setState("copied");
    } catch {
      setState("Copying is blocked in this browser");
    }
  }
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button variant="primary" onClick={() => void save()} disabled={state === "busy" || state === "saved" || plan.amountCents === 0}>
        {state === "saved" ? <Check className="size-4" aria-hidden /> : null}
        {state === "saved" ? "On the schedule" : already ? `Replace ${monthName}'s plan` : `Save as ${monthName}'s plan`}
      </Button>
      <Button variant="ghost" onClick={() => void copy()}>
        <Copy className="size-4" aria-hidden /> {state === "copied" ? "Copied" : "Copy the split"}
      </Button>
      {already && (
        <span className="type-caption text-ink-muted">
          {formatDay(payday)} already has {existing.length} {existing.length === 1 ? "purchase" : "purchases"} planned, below; replacing cancels {existing.length === 1 ? "it" : "them"}.
        </span>
      )}
      {state === "saved" && <span className="type-caption text-ink-muted">Planned for {formatDay(payday)}, below.</span>}
      {!["idle", "busy", "saved", "copied"].includes(state) && <span className="type-caption text-danger">{state}.</span>}
    </div>
  );
}

/** Each class's weight before and after, against its target and band. */
function Moves({ plan }: { plan: ContributionPlan }) {
  const band = plan.bandBp;
  const statusOf = (bp: number, t: number) => (bp < t - band ? "below" : bp > t + band ? "above" : "in");
  const back = plan.splits.filter((s) => statusOf(s.beforeBp, s.targetBp) !== "in" && statusOf(s.afterBp, s.targetBp) === "in").map((s) => s.class);
  const out = plan.splits.filter((s) => statusOf(s.afterBp, s.targetBp) !== "in").map((s) => s.class);
  const title = back.length
    ? `${names(back)} ${back.length === 1 ? "goes" : "go"} back inside ${back.length === 1 ? "its" : "their"} band`
    : out.length
      ? `${names(out)} ${out.length === 1 ? "stays" : "stay"} outside ${out.length === 1 ? "its" : "their"} band`
      : "Every class stays inside its band";
  const scale = Math.max(50, ...plan.splits.map((s) => Math.ceil((Math.max(s.beforeBp, s.afterBp, s.targetBp + band) / 100 + 5) / 10) * 10));
  const w = (bp: number) => `${Math.min(bp / 100 / scale, 1) * 100}%`;
  return (
    <Panel title={title} footer={`After the contribution, at today's prices. Bands are ${(band / 100).toString().replace(".", ",")} points either side of the target.`}>
      <div className="flex flex-col gap-4">
        {plan.splits.map((s) => {
          const k = classOf(s.class);
          return (
            <div key={s.class} className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between gap-3">
                <CategoryTag name={k.name} color={k.color} />
                <span className="type-figure-sm text-ink-muted">
                  {formatPercent(s.beforeBp)} → <strong className="font-medium text-ink">{formatPercent(s.afterBp)}</strong>
                </span>
              </div>
              <div
                role="img"
                aria-label={`${k.name} goes from ${formatPercent(s.beforeBp)} to ${formatPercent(s.afterBp)}, target ${formatPercent(s.targetBp, { decimals: 0 })}`}
                className="relative h-5"
              >
                <span className="absolute inset-y-0 rounded-xs border border-dashed border-ink-muted/60 bg-ink/5" style={{ left: w(Math.max(s.targetBp - band, 0)), width: `calc(${w(s.targetBp + band)} - ${w(Math.max(s.targetBp - band, 0))})` }} />
                <span className="absolute top-1 h-1.5 rounded-xs bg-rule-strong" style={{ width: w(s.beforeBp) }} />
                <span className="absolute bottom-1 h-1.5 rounded-xs" style={{ width: w(s.afterBp), background: `var(--${k.color})` }} />
                <span className="absolute -inset-y-0.5 w-0.5 bg-ink" style={{ left: w(s.targetBp) }} />
              </div>
            </div>
          );
        })}
        <div className="flex flex-wrap gap-x-4 gap-y-1 type-caption text-ink-muted">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-1.5 w-4 rounded-xs bg-rule-strong" aria-hidden />
            Before
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-1.5 w-4 rounded-xs bg-note-100" aria-hidden />
            After
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-3 w-0.5 bg-ink" aria-hidden />
            Target
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-3 w-4 rounded-xs border border-dashed border-ink-muted/60 bg-ink/5" aria-hidden />
            Band
          </span>
        </div>
      </div>
    </Panel>
  );
}

const statusBadge = {
  planned: { tone: "tinta" as const, icon: Clock, label: "Planned" },
  done: { tone: "inflow" as const, icon: Check, label: "Done" },
  cancelled: { tone: "neutral" as const, icon: CircleX, label: "Cancelled" },
};

/** Planned buys, with their condition, marked done or cancelled by hand (F7). */
function Schedule() {
  const res = useResource<{ purchases: PlannedPurchase[] }>("/api/v1/purchases");
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const list = res.data?.purchases ?? [];
  const today = new Date().toISOString().slice(0, 10);
  const planned = list.filter((p) => p.status === "planned").sort((a, b) => a.plannedOn.localeCompare(b.plannedOn));
  const past = list.filter((p) => p.status !== "planned");

  async function decide(p: PlannedPurchase, status: "done" | "cancelled") {
    setBusy(p.id);
    try {
      await send("PATCH", `/api/v1/purchases/${p.id}`, { status, note: "" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <Panel
      title="Purchase schedule"
      bodyClassName="-mx-5"
      action={
        !adding && (
          <Button size="sm" onClick={() => setAdding(true)}>
            Add a purchase
          </Button>
        )
      }
      footer="lastro never buys anything. Mark a purchase done when you made it, or cancel it with a reason."
    >
      {adding && <PurchaseForm onDone={() => setAdding(false)} />}
      {!res.data && res.error && <p className="m-0 px-5 type-body text-ink-muted">Could not load the purchases: {res.error}.</p>}
      {!res.data && !res.error && <SkeletonRows count={3} className="border-t border-rule" />}
      {res.data && list.length === 0 && !adding && <p className="m-0 px-5 type-body text-ink-muted">Nothing planned. Plan this month's split from above, or add a purchase.</p>}
      <div role="table" aria-label="Planned and past purchases">
        {[...planned, ...past].map((p) => {
          const b = statusBadge[p.status];
          const waiting = p.status === "planned" && p.plannedOn <= today;
          return (
            <div
              key={p.id}
              role="row"
              className={cn(
                "grid grid-cols-[60px_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1.5 border-t border-rule px-5 py-2.5",
                waiting ? "bg-warning-soft/50" : "bg-paper-raised",
                p.status !== "planned" && "text-ink-muted",
              )}
            >
              <span role="cell" className="type-figure-sm">
                {formatDay(p.plannedOn)}
              </span>
              <span role="cell" className="flex min-w-0 flex-col">
                <span className={cn("truncate text-[14px] leading-5 font-semibold", p.status !== "planned" && "font-normal")}>{p.what}</span>
                <span className="truncate type-caption text-ink-muted">{p.note || p.condition || classOf(p.class).name}</span>
              </span>
              <span role="cell" className="flex flex-col items-end gap-1">
                <Amount cents={p.amountCents} sign="none" tone={p.status === "planned" ? "ink" : "muted"} />
                <Badge tone={waiting ? "warning" : b.tone} icon={waiting ? TriangleAlert : b.icon}>
                  {waiting ? "Due" : b.label}
                </Badge>
              </span>
              {p.status === "planned" && (
                <span className="col-span-3 flex gap-2 pl-[72px]">
                  <Button size="sm" variant="ghost" disabled={busy === p.id} onClick={() => void decide(p, "done")}>
                    <Check className="size-3.5" aria-hidden /> Done
                  </Button>
                  <Button size="sm" variant="ghost" disabled={busy === p.id} onClick={() => void decide(p, "cancelled")}>
                    Cancel
                  </Button>
                </span>
              )}
            </div>
          );
        })}
      </div>
    </Panel>
  );
}

function PurchaseForm({ onDone }: { onDone: () => void }) {
  const [on, setOn] = useState(new Date().toISOString().slice(0, 10));
  const [what, setWhat] = useState("");
  const [cls, setCls] = useState<AssetClass>("br_stocks");
  const [amount, setAmount] = useState("");
  const [condition, setCondition] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const cents = parseBRL(amount);
  return (
    <form
      className="mb-3 flex flex-col gap-3 border-y border-rule bg-paper px-5 py-4"
      onSubmit={async (e) => {
        e.preventDefault();
        // One purchase per Add, however many times it is pressed while saving.
        if (busy) return;
        setBusy(true);
        setError(null);
        try {
          await send("POST", "/api/v1/purchases", { plannedOn: on, what, class: cls, amountCents: cents, condition });
          onDone();
        } catch (err) {
          setError((err as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField label="What" value={what} onChange={(e) => setWhat(e.target.value)} placeholder="WEGE3, Tesouro IPCA+ 2035" />
        <TextField label="Amount" prefix="R$" numeric inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
        <TextField label="On" type="date" value={on} onChange={(e) => setOn(e.target.value)} />
        <TextField label="Condition" value={condition} onChange={(e) => setCondition(e.target.value)} placeholder="Only after the earnings report" />
      </div>
      <div role="radiogroup" aria-label="Class" className="flex flex-wrap gap-1.5">
        {assetClasses.map((k) => (
          <button
            key={k.id}
            type="button"
            role="radio"
            aria-checked={cls === k.id}
            onClick={() => setCls(k.id)}
            className={cn("flex h-9 cursor-pointer items-center rounded-sm border px-2.5 focus-visible:focus-ring", cls === k.id ? "border-tinta bg-tinta-soft" : "border-rule bg-paper-raised")}
          >
            <CategoryTag name={k.name} color={k.color} />
          </button>
        ))}
      </div>
      {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
      <div className="flex gap-2">
        <Button type="submit" variant="primary" size="sm" disabled={busy || !what.trim() || !cents}>
          Add
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

/** The same split as lastro contribute prints it. */
function terminalText(plan: ContributionPlan): string[] {
  const width = formatBRL(plan.amountCents).length;
  const pad = (s: string) => " ".repeat(Math.max(0, width - s.length)) + s;
  const lines = [`$ lastro contribute --amount ${(plan.amountCents / 100).toFixed(2).replace(/\.00$/, "")}`];
  for (const s of plan.splits) {
    const note = [s.why === "below_band" ? "below band" : s.why === "above_band" ? "above band" : "", s.amountCents > 0 && s.buy ? s.buy : ""].filter(Boolean).join("; ");
    lines.push(`${classOf(s.class).name.padEnd(17)} ${pad(formatBRL(s.amountCents))}  ${note}`.trimEnd());
  }
  lines.push(`${"".padEnd(17)} ${"-".repeat(width)}`);
  lines.push(`${"Total".padEnd(17)} ${pad(formatBRL(plan.amountCents))}`);
  return lines;
}

function Terminal({ plan }: { plan: ContributionPlan }) {
  const lines = terminalText(plan);
  return (
    <Panel title="Same answer in the terminal" footer="lastro contribute and this screen use the same planner.">
      <pre className="m-0 overflow-x-auto rounded-sm bg-paper-sunken p-4 font-figure text-[12px] leading-5">
        {lines.map((l, i) => (
          <span key={i} className={cn("block", i === 0 || l.trim().startsWith("-") ? "text-ink-muted" : "text-ink")}>
            {l}
          </span>
        ))}
      </pre>
    </Panel>
  );
}
