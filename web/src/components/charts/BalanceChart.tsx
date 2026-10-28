import { ChartCursor } from "@/components/charts/ChartCursor";
import { formatAxis, formatBRL, formatDate, niceStep } from "@/lib/format";

export type BalancePoint = { date: string; cents: number };

type Props = {
  points: BalancePoint[];
  /** What the chart shows, for screen readers. */
  label: string;
  /** The point under the pointer or the keyboard, null when none. */
  onHover?: (p: BalancePoint | null) => void;
};

const W = 1000;
const H = 220;
const MON = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/**
 * A balance over time: a tinta line over a tinta-soft area, today at the
 * right edge. The cursor reads a day on hover, touch or the arrow keys.
 */
export function BalanceChart({ points, label, onHover }: Props) {
  if (points.length === 0) return null;
  // One day of history draws no line: say so instead of a ramp from zero.
  if (points.length === 1)
    return (
      <p className="m-0 type-body text-ink-muted">
        The balance history starts at the first sync. One day so far: {formatBRL(points[0].cents)}.
      </p>
    );

  const values = points.map((p) => p.cents);
  const max = Math.max(...values, 0);
  const min = Math.min(...values, 0);
  const step = niceStep(max - min, 4);
  const top = Math.ceil(max / step) * step || step;
  const bottom = Math.floor(min / step) * step;
  const n = points.length;
  const x = (i: number) => (n === 1 ? W : (i / (n - 1)) * W);
  const y = (v: number) => H - ((v - bottom) / (top - bottom)) * H;
  const line = points.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(p.cents).toFixed(1)}`).join(" ");
  const area = `${line} L${W} ${y(0)} L0 ${y(0)} Z`;

  const grid: number[] = [];
  for (let v = bottom; v <= top; v += step) grid.push(v);

  // About six month labels, at the first day of a month.
  const starts = points.flatMap((p, i) => (p.date.endsWith("-01") ? [i] : []));
  const every = Math.max(1, Math.ceil(starts.length / 6));
  const ticks = starts.filter((_, k) => k % every === 0).map((i) => {
    const [yy, mm] = points[i].date.split("-").map(Number);
    return { i, label: MON[mm - 1] + (mm === 1 ? ` ${String(yy).slice(2)}` : "") };
  });

  return (
    <div className="relative mr-1 mb-7 ml-16 h-[220px] max-lg:ml-11 max-lg:h-40">
      {grid.map((v) => (
        <div key={v} className="pointer-events-none absolute inset-x-0" style={{ top: `${(1 - (v - bottom) / (top - bottom)) * 100}%` }}>
          <div className={v === 0 ? "border-t border-rule-strong" : "border-t border-rule"} />
          <span className="absolute right-[calc(100%+10px)] -translate-y-1/2 type-figure-sm whitespace-nowrap text-ink-muted max-lg:right-[calc(100%+6px)]">{formatAxis(v)}</span>
        </div>
      ))}
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-hidden className="absolute inset-0 size-full overflow-visible">
        <path d={area} fill="var(--tinta-soft)" />
        <path d={line} fill="none" stroke="var(--tinta)" strokeWidth={2} strokeLinejoin="round" style={{ vectorEffect: "non-scaling-stroke" }} />
      </svg>
      {ticks.map((t) => (
        <span key={t.i} className="pointer-events-none absolute top-[calc(100%+8px)] -translate-x-1/2 type-figure-sm whitespace-nowrap text-ink-muted" style={{ left: `${(x(t.i) / W) * 100}%` }}>
          {t.label}
        </span>
      ))}
      <div className="pointer-events-none absolute -top-1.5 bottom-0 left-full border-l border-ink" />
      <ChartCursor
        W={W}
        H={H}
        plot={{ left: 0, right: W, top: 0, bottom: H }}
        xs={points.map((_, i) => x(i))}
        unit="day"
        label={`${label}.`}
        onPick={(i) => onHover?.(i === null ? null : points[i])}
        read={(i) => ({
          label: formatDate(points[i].date),
          rows: [{ label: "Balance", cents: points[i].cents, color: "var(--tinta)", sign: "negative" }],
          dots: [{ y: y(points[i].cents), color: "var(--tinta)" }],
        })}
      />
    </div>
  );
}
