import { ChartCursor } from "@/components/charts/ChartCursor";
import type { DayView } from "@/lib/api";
import { formatAxis, formatBRL, formatDay, formatMonth, niceStep } from "@/lib/format";

type Props = {
  month: string; // "2026-10"
  days: DayView[];
  previous: DayView[];
  today: string; // "2026-10-27"
  /** Drawing width: narrower on a phone, so the text stays legible. */
  width?: number;
};

const T = 24;
const B = 28;

/**
 * Spending added up day by day: this month (ink, with its area) until today,
 * the month before (muted) for the whole month. The day that moved the line
 * the most is named, so a jump reads as rent, not as a spree.
 */
export function RunningTotal({ month, days, previous, today, width = 560 }: Props) {
  const W = width;
  const narrow = W < 500;
  const H = narrow ? 240 : 300;
  const L = narrow ? 44 : 56; // room for the axis
  const R = narrow ? 72 : 92; // room for the end labels
  const [y, m] = month.split("-").map(Number);
  const count = new Date(y, m, 0).getDate();
  const spent = new Map(days.map((d) => [Number(d.date.slice(8)), d.spentCents]));
  const before = new Map(previous.map((d) => [Number(d.date.slice(8)), d.spentCents]));
  const last = today.startsWith(month) ? Number(today.slice(8)) : count;

  const run = (src: Map<number, number>, until: number) => {
    let s = 0;
    return Array.from({ length: until }, (_, i) => (s += src.get(i + 1) ?? 0));
  };
  const now = run(spent, last);
  const prev = run(before, count);
  const max = Math.max(1, ...now, ...prev);
  const step = niceStep(max, 3);
  const top = Math.ceil(max / step) * step;

  const x = (d: number) => L + ((d - 1) / Math.max(count - 1, 1)) * (W - L - R);
  const yv = (c: number) => T + (1 - c / top) * (H - T - B);
  const path = (vals: number[]) => vals.map((v, i) => `${i ? "L" : "M"}${x(i + 1).toFixed(1)} ${yv(v).toFixed(1)}`).join(" ");

  const ticks = Array.from({ length: Math.floor(top / step) + 1 }, (_, i) => i * step);
  const labelDays = W < 450 ? [1, 15, count] : [1, 8, 15, 22, count];

  // The biggest single day, if it moved the month by a quarter or more.
  let jump = { day: 0, cents: 0 };
  for (const [d, c] of spent) if (d <= last && c > jump.cents) jump = { day: d, cents: c };
  const named = jump.cents > 0 && jump.cents >= (now[last - 1] ?? 0) / 4 ? jump : null;

  const endNow = now[last - 1] ?? 0;
  const prevSame = prev[Math.min(last, count) - 1] ?? 0;
  const prevName = formatMonth(prevMonth(month), true).slice(0, 3);
  const nowName = formatMonth(month, true).slice(0, 3);
  // Keep the two end labels apart.
  let yNow = yv(endNow) + 4;
  let yPrev = yv(last < count ? prevSame : prev[count - 1]) + 4;
  if (Math.abs(yNow - yPrev) < 16) {
    if (yNow < yPrev) yPrev = yNow + 16;
    else yNow = yPrev + 16;
  }

  const dayOf = (i: number) => `${month}-${String(i + 1).padStart(2, "0")}`;
  const prevCount = prev.length;
  return (
    <div className="relative">
      <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" aria-hidden>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={L} x2={W - R + 8} y1={yv(t)} y2={yv(t)} className="stroke-rule" strokeWidth={1} />
            <text x={L - 8} y={yv(t) + 4} textAnchor="end" className="fill-ink-muted font-figure text-[11px]">
              {formatAxis(t)}
            </text>
          </g>
        ))}
        {labelDays.map((d) => (
          <text key={d} x={x(d)} y={H - 8} textAnchor="middle" className="fill-ink-muted font-figure text-[11px]">
            {d}
          </text>
        ))}
        <path d={path(prev)} fill="none" className="stroke-rule-strong" strokeWidth={2} strokeDasharray="4 3" />
        <path d={`${path(now)} L${x(last).toFixed(1)} ${yv(0)} L${x(1)} ${yv(0)} Z`} className="fill-ink/8" />
        <path d={path(now)} fill="none" className="stroke-ink" strokeWidth={2.25} strokeLinejoin="round" />
        {last < count && (
          <>
            <line x1={x(last)} x2={x(last)} y1={T - 8} y2={yv(0)} className="stroke-tinta" strokeWidth={1.5} />
            <text x={x(last)} y={T - 12} textAnchor="middle" className="fill-tinta text-[11px] font-semibold">
              Today
            </text>
            <circle cx={x(last)} cy={yv(prevSame)} r={3} className="fill-rule-strong" />
          </>
        )}
        <circle cx={x(last)} cy={yv(endNow)} r={4} className="fill-ink" />
        <text x={x(last) + 8} y={yNow} className="fill-ink font-figure text-[12px] font-medium">
          {nowName} {formatAxis(endNow)}
        </text>
        <text x={(last < count ? x(last) : x(count)) + 8} y={yPrev} className="fill-ink-muted font-figure text-[12px]">
          {prevName} {formatAxis(last < count ? prevSame : prev[count - 1])}
        </text>
        {named && (
          <text x={Math.min(x(named.day) + 8, W - R - 120)} y={Math.min(yv(now[named.day - 1]) + 16, yv(0) - 6)} className="fill-ink-muted text-[11px]">
            {formatDay(`${month}-${String(named.day).padStart(2, "0")}`)}: {formatBRL(named.cents)}
          </text>
        )}
      </svg>
      <ChartCursor
        W={W}
        H={H}
        plot={{ left: L, right: W - R, top: T, bottom: H - B }}
        xs={Array.from({ length: count }, (_, i) => x(i + 1))}
        start={last - 1}
        unit="day"
        label={`Spending added up by day. ${nowName} reaches ${formatBRL(endNow)} by day ${last}; ${prevName} had reached ${formatBRL(prevSame)} by the same day.`}
        read={(i) => {
          const rows = [];
          const dots = [];
          if (i < now.length) {
            rows.push({ label: nowName, cents: now[i], color: "var(--ink)" });
            dots.push({ y: yv(now[i]), color: "var(--ink)" });
          }
          if (i < prevCount) {
            rows.push({ label: prevName, cents: prev[i], color: "var(--rule-strong)", dashed: true });
            dots.push({ y: yv(prev[i]), color: "var(--rule-strong)" });
          }
          return { label: formatDay(dayOf(i)), rows, dots };
        }}
      />
    </div>
  );
}

function prevMonth(m: string): string {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(y, mo - 2, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}
