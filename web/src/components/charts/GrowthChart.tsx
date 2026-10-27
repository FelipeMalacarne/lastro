import { formatAxis, formatBRL, formatMonth, niceStep } from "@/lib/format";

type Point = { date: string; valueCents: number; basisCents: number };

const W = 640;
const H = 300;
const L = 56;
const R = 112;
const T = 20;
const B = 32;

/**
 * Invested money over time, split in two: what was put in (the muted area)
 * and what it earned on top (the inflow band up to the line). A loss shows
 * as the line dipping into the put-in area.
 */
export function GrowthChart({ points, width = W }: { points: Point[]; width?: number }) {
  if (points.length < 2) return <p className="m-0 type-body text-ink-muted">The chart starts after the first month of valuations.</p>;
  const Wd = width;
  const max = Math.max(1, ...points.map((p) => Math.max(p.valueCents, p.basisCents)));
  const step = niceStep(max, 3);
  const top = Math.ceil(max / step) * step;
  const x = (i: number) => L + (i / (points.length - 1)) * (Wd - L - R);
  const y = (c: number) => T + (1 - c / top) * (H - T - B);
  const line = (key: "valueCents" | "basisCents") => points.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(p[key]).toFixed(1)}`).join(" ");
  const basisArea = `${line("basisCents")} L${x(points.length - 1)} ${y(0)} L${x(0)} ${y(0)} Z`;
  const returnsArea =
    line("valueCents") +
    " " +
    [...points]
      .map((p, i) => ({ p, i }))
      .reverse()
      .map(({ p, i }) => `L${x(i).toFixed(1)} ${y(p.basisCents).toFixed(1)}`)
      .join(" ") +
    " Z";
  const first = points[0];
  const last = points[points.length - 1];
  const growth = last.valueCents - first.valueCents;
  const putIn = last.basisCents - first.basisCents;
  const ticks = Array.from({ length: Math.floor(top / step) + 1 }, (_, i) => i * step);
  // On a narrow chart only the ends are named; otherwise every third month.
  const every = Wd < 500 ? Infinity : 3;
  const labels = points
    .map((p, i) => ({ i, m: p.date.slice(0, 7) }))
    .filter(({ i }) => i === 0 || i === points.length - 1 || (i % every === 0 && points.length - 1 - i >= 2));

  return (
    <svg
      viewBox={`0 0 ${Wd} ${H}`}
      className="h-auto w-full"
      role="img"
      aria-label={`Invested money went from ${formatBRL(first.valueCents)} to ${formatBRL(last.valueCents)}: ${formatBRL(putIn)} put in and ${formatBRL(growth - putIn)} in returns.`}
    >
      {ticks.map((t) => (
        <g key={t}>
          <line x1={L} x2={Wd - R + 8} y1={y(t)} y2={y(t)} className="stroke-rule" />
          <text x={L - 8} y={y(t) + 4} textAnchor="end" className="fill-ink-muted font-figure text-[11px]">
            {formatAxis(t)}
          </text>
        </g>
      ))}
      {labels.map(({ i, m }) => (
        <text key={i} x={x(i)} y={H - 10} textAnchor={i === 0 ? "start" : i === points.length - 1 ? "end" : "middle"} className="fill-ink-muted font-figure text-[11px]">
          {i === 0 || i === points.length - 1 ? formatMonth(m, true) : formatMonth(m, true).slice(0, 3)}
        </text>
      ))}
      <path d={basisArea} className="fill-rule-strong/45" />
      <path d={returnsArea} className="fill-inflow/25" />
      <path d={line("basisCents")} fill="none" className="stroke-ink-muted" strokeWidth={1.5} strokeDasharray="4 3" />
      <path d={line("valueCents")} fill="none" className="stroke-ink" strokeWidth={2.25} strokeLinejoin="round" />
      <circle cx={x(points.length - 1)} cy={y(last.valueCents)} r={4} className="fill-ink" />
      <text x={x(points.length - 1) + 10} y={y(last.valueCents) + 4} className="fill-ink font-figure text-[12px] font-medium">
        {formatAxis(last.valueCents)}
      </text>
      <text x={x(points.length - 1) + 10} y={Math.max(y(last.basisCents) + 18, y(last.valueCents) + 22)} className="fill-ink-muted font-figure text-[11px]">
        {formatAxis(last.basisCents)}
      </text>
      <text x={x(points.length - 1) + 10} y={Math.max(y(last.basisCents) + 32, y(last.valueCents) + 36)} className="fill-ink-muted text-[11px]">
        put in
      </text>
    </svg>
  );
}
