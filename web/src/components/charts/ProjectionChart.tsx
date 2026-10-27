import { formatAxis, formatBRL } from "@/lib/format";

type Props = {
  /** The invested total at the end of each past month, oldest first. */
  history: number[];
  /** Today, then each year: pessimistic, base and optimistic. */
  low: number[];
  base: number[];
  high: number[];
  /** Monte Carlo's 10th and 90th percentiles per year, when shown. */
  bands?: { p10: number; p90: number }[];
  startYear: number;
  width?: number;
};

const H = 320;
const L = 80;
const R = 112;
const T = 16;
const B = 32;

/**
 * The invested money so far, then the projection: the base line, the space
 * between pessimistic and optimistic, and, when asked, where 80% of the
 * simulated paths land.
 */
export function ProjectionChart({ history, low, base, high, bands, startYear, width = 1000 }: Props) {
  const years = base.length - 1;
  const past = history.length; // months before today
  const span = past / 12 + years;
  const max = Math.max(1, ...high, ...(bands?.map((b) => b.p90) ?? []), ...history);
  const step = niceStep(max);
  const top = Math.ceil(max / step) * step;
  const x = (yearsFromNow: number) => L + ((yearsFromNow + past / 12) / span) * (width - L - R);
  const y = (c: number) => T + (1 - c / top) * (H - T - B);
  const pts = (vs: number[]) => vs.map((v, i) => `${x(i).toFixed(1)} ${y(v).toFixed(1)}`);
  const fan = `M${pts(high).join(" L")} L${pts(low).reverse().join(" L")} Z`;
  const mc = bands && bands.length ? `M${bands.map((b, i) => `${x(i + 1).toFixed(1)} ${y(b.p90).toFixed(1)}`).join(" L")} L${bands.map((b, i) => `${x(i + 1).toFixed(1)} ${y(b.p10).toFixed(1)}`).reverse().join(" L")} L${x(0)} ${y(base[0])} Z` : null;
  const hist = history.map((v, i) => `${i ? "L" : "M"}${x(-(past - i) / 12).toFixed(1)} ${y(v).toFixed(1)}`).join(" ");
  const ticks = Array.from({ length: Math.floor(top / step) + 1 }, (_, i) => i * step);
  const every = years > 20 ? 10 : 5;
  const yearMarks = Array.from({ length: Math.floor(years / every) + 1 }, (_, i) => i * every).filter((v) => v <= years);
  if (yearMarks.at(-1) !== years) yearMarks.push(years);

  const label = (v: number, dy: number, tone: string) => (
    <text x={x(years) + 8} y={y(v) + dy} className={`font-figure text-[12px] ${tone}`}>
      {formatAxis(v)}
    </text>
  );
  // Keep the three end labels apart.
  const ys = [y(high[years]), y(base[years]), y(low[years])];
  const dy = [0, 0, 0];
  if (ys[1] - ys[0] < 16) dy[0] = -(16 - (ys[1] - ys[0]));
  if (ys[2] - ys[1] < 16) dy[2] = 16 - (ys[2] - ys[1]);

  return (
    <svg
      viewBox={`0 0 ${width} ${H}`}
      className="h-auto w-full"
      role="img"
      aria-label={`Projection of invested money in today's money. From ${formatBRL(base[0])} today, the base scenario reaches ${formatBRL(base[years])} in ${startYear + years}, between ${formatBRL(low[years])} and ${formatBRL(high[years])}.`}
    >
      {ticks.map((t) => (
        <g key={t}>
          <line x1={L} x2={width - R + 8} y1={y(t)} y2={y(t)} className="stroke-rule" />
          <text x={L - 8} y={y(t) + 4} textAnchor="end" className="fill-ink-muted font-figure text-[11px]">
            {formatAxis(t)}
          </text>
        </g>
      ))}
      {yearMarks.map((yr) => (
        <text key={yr} x={x(yr)} y={H - 10} textAnchor={yr === years ? "end" : "middle"} className="fill-ink-muted font-figure text-[11px]">
          {yr === 0 ? "Today" : startYear + yr}
        </text>
      ))}
      {mc && <path d={mc} className="fill-tinta/10" />}
      <path d={fan} className="fill-tinta/15" />
      {history.length > 1 && <path d={hist} fill="none" className="stroke-ink" strokeWidth={2.25} />}
      <path d={`M${pts(base).join(" L")}`} fill="none" className="stroke-tinta" strokeWidth={2.25} strokeDasharray="6 4" />
      <line x1={x(0)} x2={x(0)} y1={T} y2={y(0)} className="stroke-rule-strong" strokeDasharray="2 3" />
      <circle cx={x(0)} cy={y(base[0])} r={4} className="fill-ink" />
      {label(high[years], 4 + dy[0], "fill-ink-muted")}
      {label(base[years], 4 + dy[1], "fill-tinta font-medium")}
      {label(low[years], 4 + dy[2], "fill-ink-muted")}
    </svg>
  );
}

/** A step for about three gridlines: 1, 2, 2,5 or 5 times a power of ten. */
function niceStep(max: number): number {
  const raw = max / 3;
  const pow = 10 ** Math.floor(Math.log10(raw));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * pow >= raw) return m * pow;
  return 10 * pow;
}
