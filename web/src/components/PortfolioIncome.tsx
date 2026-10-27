import { useEffect, useState } from "react";

import { Amount } from "@/components/Amount";
import { Panel } from "@/components/Panel";
import { Sparkline } from "@/components/Sparkline";
import { Button } from "@/components/ui/button";
import { send, useResource } from "@/lib/api";
import { formatBRL, formatDay, formatPercent } from "@/lib/format";

type PaymentView = { ticker: string; label: string; comDate: string; paymentDate: string | null; rateMicro: number; cents: number };
type DividendsView = {
  since: string | null;
  yearCents: number;
  valueCents: number;
  years: { year: number; cents: number }[];
  tickers: { ticker: string; yearCents: number; trailingMicro: number; yieldBps: number; next: PaymentView | null }[];
  forwardCents: number;
  upcoming: PaymentView[];
  missing: string[];
};
type ValuationPoint = { date: string; peMilli: number | null; pbMilli: number | null; yieldBps: number };

/** "R$ 0,82" a share, with four decimals when it is under ten cents. */
export function formatPerShare(micro: number): string {
  if (micro >= 100_000) return formatBRL(Math.round(micro / 10_000));
  return `R$ ${(micro / 1_000_000).toFixed(4).replace(".", ",")}`;
}

/** 28400 → "28,4". */
export function formatMultiple(milli: number | null): string {
  return milli === null ? "—" : (milli / 1000).toFixed(1).replace(".", ",");
}

const labelName: Record<string, string> = { DIVIDENDO: "Dividend", JCP: "JCP", RENDIMENTO: "Income" };

/** What the shares held paid (F8; canvas: "Dividends, valuation and price alerts"). */
export function Dividends() {
  const d = useResource<DividendsView>("/api/v1/dividends").data;
  if (!d || (d.years.length === 0 && d.tickers.length === 0)) return null;
  const year = new Date().getFullYear();
  const max = Math.max(1, ...d.years.map((y) => y.cents));
  const share = d.valueCents > 0 ? formatPercent(Math.round((d.yearCents * 10000) / d.valueCents)) : null;
  const title = `${formatBRL(d.yearCents)} in dividends in ${year} so far${share ? `, ${share} of the shares' value` : ""}`;
  return (
    <Panel
      title={title}
      footer={
        <>
          Each payment per share times the quantity you held on its com date
          {d.since ? `, as lastro has seen it since ${formatDay(d.since)} ${d.since.slice(0, 4)}; earlier ones are not counted` : ""}. Dividends, JCP (before its 15% tax) and
          fund income, from brapi.
          {d.missing.length > 0 && ` No payments known for ${d.missing.join(", ")}.`}
        </>
      }
    >
      <div className="flex flex-wrap gap-8">
        <div className="flex min-w-0 flex-[4_1_260px] flex-col gap-2.5">
          <span className="type-label text-ink-muted">By year</span>
          {d.years.map((y) => (
            <div key={y.year} className="grid grid-cols-[48px_minmax(0,1fr)_auto] items-center gap-3">
              <span className="font-figure text-[14px] font-medium">{y.year}</span>
              <span className="h-3 rounded-xs bg-note-1" style={{ width: `${(y.cents / max) * 100}%` }} />
              <Amount cents={y.cents} sign="none" />
            </div>
          ))}
          <div className="flex flex-col gap-0.5 border-t border-rule pt-2.5">
            <span className="type-caption text-ink-muted">At today's holdings, the last 12 months of payments would be</span>
            <span className="flex flex-wrap items-baseline gap-1.5">
              <Amount cents={d.forwardCents} sign="none" estimate />
              <span className="type-caption text-ink-muted">a year · Estimate</span>
            </span>
          </div>
          {d.upcoming.length > 0 && (
            <div className="flex flex-col gap-1 border-t border-rule pt-2.5">
              <span className="type-label text-ink-muted">Earned, still to be paid</span>
              {d.upcoming.map((p) => (
                <span key={p.ticker + p.comDate + p.label} className="flex justify-between gap-3 text-[14px] leading-5">
                  <span>
                    <span className="font-figure">{p.ticker}</span> {labelName[p.label] ?? p.label}
                    <span className="type-caption text-ink-muted">{p.paymentDate ? ` · ${formatDay(p.paymentDate)}` : " · date not set"}</span>
                  </span>
                  <Amount cents={p.cents} sign="none" size="sm" />
                </span>
              ))}
            </div>
          )}
        </div>
        <div className="min-w-0 flex-[7_1_420px]" role="table" aria-label="Dividends by stock">
          <div role="row" className="grid h-8 grid-cols-[minmax(0,1fr)_100px_100px_64px] items-center gap-3 bg-paper-sunken px-2 type-label text-ink-muted">
            <span role="columnheader">Stock</span>
            <span role="columnheader" className="text-right">
              {year}
            </span>
            <span role="columnheader" className="text-right">
              12 m a share
            </span>
            <span role="columnheader" className="text-right">
              Yield
            </span>
          </div>
          {d.tickers.map((t) => (
            <div key={t.ticker} role="row" className="grid min-h-[52px] grid-cols-[minmax(0,1fr)_100px_100px_64px] items-center gap-3 border-b border-rule px-2 py-1.5">
              <span role="cell" className="flex min-w-0 flex-col">
                <span className="font-figure text-[14px] font-medium">{t.ticker}</span>
                {t.next && (
                  <span className="truncate type-caption text-ink-muted">
                    Next: {labelName[t.next.label] ?? t.next.label} {formatPerShare(t.next.rateMicro)}
                    {t.next.paymentDate ? ` on ${formatDay(t.next.paymentDate)}` : ", date not set"}
                  </span>
                )}
              </span>
              <span role="cell" className="text-right">
                <Amount cents={t.yearCents} sign="none" size="sm" />
              </span>
              <span role="cell" className="text-right type-figure-sm">
                {t.trailingMicro ? formatPerShare(t.trailingMicro) : "—"}
              </span>
              <span role="cell" className="text-right type-figure-sm">
                {t.yieldBps ? formatPercent(t.yieldBps) : "—"}
              </span>
            </div>
          ))}
          <p className="m-0 px-2 pt-2 type-caption text-ink-muted">Yield is the last 12 months of payments over today's price.</p>
        </div>
      </div>
    </Panel>
  );
}

/** P/E, P/B and dividend yield of a stock, with their history (F8). */
export function Valuation({ ticker }: { ticker: string }) {
  const points = useResource<{ points: ValuationPoint[] }>(`/api/v1/valuation/${encodeURIComponent(ticker)}`).data?.points;
  if (!points) return null;
  if (points.length === 0)
    return <p className="m-0 border-t border-rule pt-3 type-caption text-ink-muted">No earnings or book value for {ticker} yet. brapi sends them with LASTRO_BRAPI_TOKEN set; the history starts the day lastro first reads them.</p>;
  const last = points.at(-1)!;
  const first = points[0];
  const metric = (name: string, value: string, series: (number | null)[], was: string, note?: string) => {
    const vs = series.filter((x): x is number => x !== null);
    return (
      <div key={name} className="flex flex-col gap-1 rounded-md border border-rule p-3">
        <span className="type-caption text-ink-muted">{name}</span>
        <span className="font-display text-[24px] leading-8 font-semibold [font-stretch:112%]">{value}</span>
        {vs.length > 2 && <Sparkline values={vs} width={140} height={28} zero={false} label={`${name} since ${formatDay(first.date)}: ${was} then ${value}`} />}
        <span className="type-caption text-ink-muted">{note ?? `Was ${was} on ${formatDay(first.date)}`}</span>
      </div>
    );
  };
  return (
    <div className="flex flex-col gap-3 border-t border-rule pt-3">
      <span className="type-label text-ink-muted">Valuation · {formatDay(last.date)}</span>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3">
        {metric("P/E", formatMultiple(last.peMilli), points.map((p) => p.peMilli), formatMultiple(first.peMilli))}
        {metric("P/B", formatMultiple(last.pbMilli), points.map((p) => p.pbMilli), formatMultiple(first.pbMilli))}
        {metric("Dividend yield", formatPercent(last.yieldBps), points.map((p) => p.yieldBps), formatPercent(first.yieldBps), "12 months over the day's close")}
      </div>
      <span className="type-caption text-ink-muted">
        Price over the last 12 months of earnings per share, and over book value per share. A loss-making company shows “—” for P/E, never a negative multiple.
      </span>
    </div>
  );
}

/** A small line under a thesis: "P/E 28,4 · yield 1,6%". */
export function ValuationLine({ ticker }: { ticker: string }) {
  const last = useResource<{ points: ValuationPoint[] }>(`/api/v1/valuation/${encodeURIComponent(ticker)}`).data?.points.at(-1);
  if (!last) return null;
  return (
    <span className="type-caption text-ink-muted">
      P/E {formatMultiple(last.peMilli)} · P/B {formatMultiple(last.pbMilli)} · yield {formatPercent(last.yieldBps)}
    </span>
  );
}

/** The fall in a day that raises an alert for a ticker (F8 P2). */
export function PriceAlert({ ticker }: { ticker: string }) {
  const alerts = useResource<{ alerts: Record<string, number> }>("/api/v1/price-alerts").data?.alerts;
  const current = alerts?.[ticker];
  const [text, setText] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => setText(current ? String(current / 100).replace(".", ",") : "5"), [current]);
  async function run(f: () => Promise<unknown>, done: string) {
    setBusy(true);
    setMsg(null);
    try {
      await f();
      setMsg(done);
    } catch (e) {
      setMsg(`Could not save: ${(e as Error).message}.`);
    } finally {
      setBusy(false);
    }
  }
  const bps = Math.round(Number(text.replace(",", ".")) * 100);
  const valid = Number.isFinite(bps) && bps >= 50 && bps <= 5000;
  return (
    <form
      className="flex flex-col gap-2 border-t border-rule pt-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid) void run(() => send("PUT", `/api/v1/price-alerts/${ticker}`, { dropBps: bps }), "Saved. Checked after each day's close.");
      }}
    >
      <span className="type-label text-ink-muted">Price alert</span>
      <div className="flex flex-wrap items-end gap-x-3 gap-y-2">
        <label className="flex flex-col gap-1.5 text-[13px] leading-[18px] font-semibold">
          Tell me when it falls more than
          <span className="flex h-9 w-[110px] items-center gap-1 rounded-sm border border-rule-strong bg-paper-raised px-2.5 font-figure text-[14px] font-normal focus-within:focus-ring">
            <input
              inputMode="decimal"
              aria-label="Percent"
              value={text}
              onChange={(e) => setText(e.target.value)}
              className="w-full min-w-0 border-0 bg-transparent text-right text-ink outline-none"
            />
            <span className="text-ink-muted">%</span>
          </span>
        </label>
        <span className="flex h-9 items-center text-[14px]">in a day</span>
        <Button type="submit" size="sm" disabled={!valid || busy}>
          Save
        </Button>
        {current !== undefined && (
          <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => void run(() => send("DELETE", `/api/v1/price-alerts/${ticker}`), "No alert.")}>
            No alert
          </Button>
        )}
      </div>
      <span className="type-caption text-ink-muted">
        {msg ?? (valid ? "It shows on the dashboard with the thesis and goes to Telegram once." : "Between 0,5% and 50%.")}
      </span>
    </form>
  );
}
