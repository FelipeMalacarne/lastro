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

/** "2026-10-26" → days since 1970, so points sit by their dates, not by their order. */
const dayNumber = (d: string) => {
  const [y, m, dd] = d.split("-").map(Number);
  return Date.UTC(y, m - 1, dd) / 86_400_000;
};

/**
 * About six labels: the first of each month, or each January over a span of
 * years. When the months cross a year, the first label of each year carries
 * it ("Apr 25", "Aug", "Dec", "Apr 26").
 */
function axisTicks(first: string, last: string): { day: number; label: string }[] {
  const [y0, m0] = first.split("-").map(Number);
  const [y1, m1] = last.split("-").map(Number);
  const years = dayNumber(last) - dayNumber(first) > 2 * 366;
  const starts: { y: number; m: number; day: number }[] = [];
  for (let k = y0 * 12 + m0 - 1; k <= y1 * 12 + m1 - 1; k++) {
    const y = Math.floor(k / 12);
    const m = (k % 12) + 1;
    const date = `${y}-${String(m).padStart(2, "0")}-01`;
    if (date < first || (years && m !== 1)) continue;
    starts.push({ y, m, day: dayNumber(date) });
  }
  const every = Math.max(1, Math.ceil(starts.length / 6));
  const kept = starts.filter((_, k) => k % every === 0);
  const crossesYears = kept.length > 0 && kept[0].y !== kept[kept.length - 1].y;
  return kept.map((t, k) => ({
    day: t.day,
    label: years ? String(t.y) : MON[t.m - 1] + (crossesYears && (k === 0 || kept[k - 1].y !== t.y) ? ` ${String(t.y).slice(2)}` : ""),
  }));
}

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
  // By date: two updates a year apart sit further apart than two a week apart.
  const start = dayNumber(points[0].date);
  const span = dayNumber(points[points.length - 1].date) - start || 1;
  const xAt = (day: number) => ((day - start) / span) * W;
  const x = (i: number) => xAt(dayNumber(points[i].date));
  const y = (v: number) => H - ((v - bottom) / (top - bottom)) * H;
  const line = points.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(p.cents).toFixed(1)}`).join(" ");
  const area = `${line} L${W} ${y(0)} L0 ${y(0)} Z`;

  const grid: number[] = [];
  for (let v = bottom; v <= top; v += step) grid.push(v);
  const ticks = axisTicks(points[0].date, points[points.length - 1].date);
  // The y labels' gutter grows with the longest one (a figure is 0.6em of 12px), so "999.999.999" is never cut.
  const gutter = Math.ceil(Math.max(...grid.map((v) => formatAxis(v).length)) * 7.2) + 10;

  return (
    <div
      className="relative mr-1 mb-7 ml-[max(4rem,var(--gutter))] h-[220px] max-lg:ml-[max(2.75rem,var(--gutter))] max-lg:h-40"
      style={{ "--gutter": `${gutter}px` } as React.CSSProperties}
    >
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
        <span key={t.day} className="pointer-events-none absolute top-[calc(100%+8px)] -translate-x-1/2 type-figure-sm whitespace-nowrap text-ink-muted" style={{ left: `${(xAt(t.day) / W) * 100}%` }}>
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
