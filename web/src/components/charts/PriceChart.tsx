import { formatBRL, formatDay, formatPercent } from "@/lib/format";

type Close = { date: string; closeCents: number };

/** A ticker's daily closes as a line, with the period's change, low and high. */
export function PriceChart({ ticker, closes, width = 560 }: { ticker: string; closes: Close[]; width?: number }) {
  if (closes.length < 2) return null;
  const H = 140;
  const pad = 6;
  const vals = closes.map((c) => c.closeCents);
  const lo = Math.min(...vals);
  const hi = Math.max(...vals);
  const span = hi - lo || 1;
  const x = (i: number) => pad + (i / (closes.length - 1)) * (width - pad * 2);
  const y = (v: number) => pad + (1 - (v - lo) / span) * (H - pad * 2);
  const d = vals.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(v).toFixed(1)}`).join(" ");
  const first = closes[0];
  const last = closes[closes.length - 1];
  const change = Math.round(((last.closeCents - first.closeCents) * 10000) / first.closeCents);
  const up = change >= 0;
  return (
    <figure className="m-0 flex flex-col gap-2">
      <figcaption className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <span className="type-label text-ink-muted">
          {ticker} since {formatDay(first.date)} {first.date.slice(0, 4)}
        </span>
        <span className="flex flex-wrap gap-x-4 type-caption text-ink-muted">
          <span>
            <strong className="font-figure font-medium text-ink">{formatBRL(last.closeCents)}</strong> on {formatDay(last.date)}
          </span>
          <span className={up ? "text-inflow" : "text-ink"}>{formatPercent(change, { sign: "always" })} in the period</span>
          <span>
            low {formatBRL(lo)} · high {formatBRL(hi)}
          </span>
        </span>
      </figcaption>
      <svg
        viewBox={`0 0 ${width} ${H}`}
        className="h-auto w-full"
        role="img"
        aria-label={`${ticker} went from ${formatBRL(first.closeCents)} to ${formatBRL(last.closeCents)} (${formatPercent(change, { sign: "always" })}), between ${formatBRL(lo)} and ${formatBRL(hi)}.`}
      >
        <line x1={pad} x2={width - pad} y1={y(first.closeCents)} y2={y(first.closeCents)} className="stroke-rule-strong" strokeDasharray="3 3" />
        <path d={d} fill="none" stroke="var(--tinta)" strokeWidth={1.75} strokeLinejoin="round" />
        <circle cx={x(closes.length - 1)} cy={y(last.closeCents)} r={3} fill="var(--tinta)" />
      </svg>
    </figure>
  );
}
