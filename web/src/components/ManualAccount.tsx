import { useState } from "react";
import { X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { BalanceChart } from "@/components/charts/BalanceChart";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { send, useResource, type Account, type BalanceHistory } from "@/lib/api";
import { formatBRL, formatDate, inputBRL, parseBRL, todayISO } from "@/lib/format";

/** An account typed in (canvas: "Accounts outside Open Finance"): a wallet or an asset. */
export function ManualAccountView({ a }: { a: Account }) {
  return a.type === "cash" ? <WalletView a={a} /> : <AssetView a={a} />;
}

function Hero({ a, caption }: { a: Account; caption: string }) {
  return (
    <Panel>
      <div className="flex flex-col gap-1">
        <span className="type-label text-ink-muted">Balance</span>
        <Amount cents={a.balanceCents} size="hero" sign="none" />
        <span className="type-caption text-ink-muted">{caption}</span>
      </div>
    </Panel>
  );
}

function WalletView({ a }: { a: Account }) {
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-6">
      <Hero a={a} caption={`Typed in by you · balance updated ${formatDate(a.balanceAt.slice(0, 10))}`} />
      <AddTransaction a={a} />
    </div>
  );
}

/**
 * A transaction typed in: a wallet entry, or, with missing, one a synced
 * account's bank never sent (canvas: "A transaction the bank never sent").
 */
export function AddTransaction({ a, missing }: { a: Account; missing?: boolean }) {
  const [day, setDay] = useState(todayISO());
  const [description, setDescription] = useState("");
  const [dir, setDir] = useState<"out" | "in">("out");
  const [amount, setAmount] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const cents = parseBRL(amount);
  const valid = Boolean(description.trim()) && Boolean(cents) && /^\d{4}-\d{2}-\d{2}$/.test(day);
  async function add(e: React.FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true);
    setMsg(null);
    try {
      await send("POST", `/api/v1/accounts/${a.id}/transactions`, { date: day, description: description.trim(), amountCents: dir === "out" ? -cents! : cents });
      setDescription("");
      setAmount("");
      setMsg(
        missing
          ? "Added. If the bank sends it later, lastro asks whether the two are the same."
          : "Added. Rules and the model categorize it like any other.",
      );
    } catch (err) {
      setMsg(`Could not add: ${(err as Error).message}.`);
    } finally {
      setBusy(false);
    }
  }
  const bank = a.institution;
  return (
    <Panel title={missing ? "Add a missing transaction" : "Add a transaction"}>
      <form onSubmit={add} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-end gap-3">
          <TextField label="Date" type="date" value={day} max={todayISO()} onChange={(e) => setDay(e.target.value)} className="min-w-0 flex-[0_1_170px]" />
          <TextField
            label="Description"
            placeholder={missing ? "e.g. Pix sent" : "e.g. Feira de domingo"}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="min-w-0 flex-[1_1_200px]"
          />
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1.5">
            <span className="text-[13px] leading-[18px] font-semibold">Direction</span>
            <SegmentedControl
              label="Direction"
              value={dir}
              onChange={(v) => setDir(v as "out" | "in")}
              options={[
                { value: "out", label: "Out" },
                { value: "in", label: "In" },
              ]}
            />
          </div>
          <TextField label="Amount" placeholder="R$ 0,00" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} className="min-w-0 flex-[0_1_150px]" />
          <Button type="submit" variant={missing ? "secondary" : "primary"} disabled={!valid || busy}>
            Add
          </Button>
        </div>
        <span className="type-caption text-ink-muted">
          {msg ??
            (missing
              ? `For one the bank app shows but ${bank} never sent. The balance stays the bank's, which already counts it; the history and the month's totals count it too. A sync never removes it.`
              : "Money in from a withdrawal at the ATM pairs with the checking side as a transfer, so it never counts twice. Open an entry to remove it.")}
        </span>
      </form>
    </Panel>
  );
}

function AssetView({ a }: { a: Account }) {
  const history = useResource<BalanceHistory>(`/api/v1/accounts/${a.id}/balance`).data;
  const points = history?.points ?? [];
  const [day, setDay] = useState(todayISO());
  const [balance, setBalance] = useState(inputBRL(a.balanceCents));
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const cents = parseBRL(balance);
  async function run(f: () => Promise<unknown>, done: string) {
    setBusy(true);
    setMsg(null);
    try {
      await f();
      setMsg(done);
    } catch (err) {
      setMsg(`Could not save: ${(err as Error).message}.`);
    } finally {
      setBusy(false);
    }
  }
  const updates = [...points].reverse();
  return (
    <>
      <Hero a={a} caption={`Asset typed in by you${points.length ? ` · balance of ${formatDate(points.at(-1)!.date)}` : ""}`} />
      <Panel title="Balance updates" footer="Each update is one point. Net worth uses the latest; the chart joins them with straight lines and never guesses between them.">
        <div className="flex flex-col gap-4">
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (cents !== null) void run(() => send("PUT", `/api/v1/accounts/${a.id}/balance`, { date: day, balanceCents: cents }), "Saved.");
            }}
          >
            <TextField label="Balance on" type="date" value={day} max={todayISO()} onChange={(e) => setDay(e.target.value)} className="min-w-0 flex-[0_1_170px]" />
            <TextField label="Balance" inputMode="decimal" value={balance} onChange={(e) => setBalance(e.target.value)} className="min-w-0 flex-[0_1_180px]" />
            <Button type="submit" variant="primary" disabled={busy || cents === null || points.some((p) => p.date === day && p.balanceCents === cents)}>
              Update
            </Button>
            {msg && <span className="type-caption text-ink-muted">{msg}</span>}
          </form>
          {points.length > 1 && <BalanceChart points={points.map((p) => ({ date: p.date, cents: p.balanceCents }))} label={`Balance from ${formatBRL(points[0].balanceCents)} to ${formatBRL(points.at(-1)!.balanceCents)}`} />}
          <ul role="list" className="m-0 flex list-none flex-col p-0">
            {updates.map((u, i) => {
              const before = updates[i + 1];
              const change = before ? u.balanceCents - before.balanceCents : null;
              return (
                <li key={u.date} className="grid min-h-11 grid-cols-[110px_minmax(0,1fr)_auto_auto] items-center gap-3 border-t border-rule">
                  <span className="type-figure-sm text-ink-muted">{formatDate(u.date)}</span>
                  <span className="type-caption text-ink-muted">{change === null ? "First update" : `${formatBRL(change, { sign: "always" })} since ${formatDate(before!.date)}`}</span>
                  <Amount cents={u.balanceCents} sign="none" />
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={`Remove the update of ${formatDate(u.date)}`}
                    disabled={busy || updates.length < 2}
                    onClick={() => void run(() => send("DELETE", `/api/v1/accounts/${a.id}/balance/${u.date}`), "Removed.")}
                  >
                    <X aria-hidden />
                  </Button>
                </li>
              );
            })}
          </ul>
        </div>
      </Panel>
    </>
  );
}
