import { ChartCursor, type CursorRead } from "@/components/charts/ChartCursor";
import { formatAxis, formatCompact, formatBRL, formatMonth } from "@/lib/format";

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
  /** The amount asked for in "How long until", and when the base scenario gets there. */
  target?: { cents: number; months: number | null; when: string | null };
  width?: number;
};

const T = 16;
const B = 32;

/**
 * The invested money so far, then the projection: the base line, the space
 * between pessimistic and optimistic, and, when asked, where 80% of the
 * simulated paths land.
 */
export function ProjectionChart({ history, low, base, high, bands, startYear, target, width = 1000 }: Props) {
  // A narrow chart keeps its text near its size on a phone: less room for labels.
  const narrow = width < 500;
  const H = narrow ? 260 : 320;
  const L = narrow ? 60 : 80;
  const R = narrow ? 64 : 112;
  const axis = narrow ? formatCompact : formatAxis;
  const years = base.length - 1;
  const past = history.length; // months before today
  const span = past / 12 + years;
  const reach = Math.max(1, ...high, ...(bands?.map((b) => b.p90) ?? []), ...history);
  // A target a little past the fan still gets its line; one far beyond would flatten everything.
  const showTarget = target && target.cents > 0 && target.cents <= reach * 1.25 ? target : undefined;
  const max = Math.max(reach, showTarget?.cents ?? 0);
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

  // Thirty years out, the end values are rounded to the thousand: their precision is not real.
  const rounded = (v: number) => (Math.abs(v) >= 1_000_000 ? Math.round(v / 100_000) * 100_000 : v);
  const label = (v: number, dy: number, tone: string) => (
    <text x={x(years) + 8} y={y(v) + dy} className={`font-figure text-[12px] ${tone}`}>
      {axis(rounded(v))}
    </text>
  );
  const crossing = showTarget && showTarget.months !== null && showTarget.months / 12 <= years ? showTarget.months / 12 : null;
  // Keep the three end labels apart.
  const ys = [y(high[years]), y(base[years]), y(low[years])];
  const dy = [0, 0, 0];
  if (ys[1] - ys[0] < 16) dy[0] = -(16 - (ys[1] - ys[0]));
  if (ys[2] - ys[1] < 16) dy[2] = 16 - (ys[2] - ys[1]);

  // The cursor reads each past month, then today and each year ahead.
  const now = new Date();
  const xs = [...history.map((_, i) => x(-(past - i) / 12)), ...base.map((_, yr) => x(yr))];
  const read = (i: number): CursorRead => {
    if (i < past) {
      const d = new Date(now.getFullYear(), now.getMonth() - (past - i), 1);
      const month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
      return { label: formatMonth(month, true), rows: [{ label: "Invested", cents: history[i], color: "var(--ink)" }], dots: [{ y: y(history[i]), color: "var(--ink)" }] };
    }
    const yr = i - past;
    if (yr === 0) return { label: "Today", rows: [{ label: "Invested", cents: base[0], color: "var(--ink)" }], dots: [{ y: y(base[0]), color: "var(--ink)" }] };
    return {
      label: String(startYear + yr),
      rows: [
        { label: "Optimistic", cents: rounded(high[yr]), color: "var(--ink-muted)", estimate: true },
        { label: "Base", cents: rounded(base[yr]), color: "var(--tinta)", dashed: true, estimate: true },
        { label: "Pessimistic", cents: rounded(low[yr]), color: "var(--ink-muted)", estimate: true },
      ],
      dots: [
        { y: y(high[yr]), color: "var(--ink-muted)" },
        { y: y(base[yr]), color: "var(--tinta)" },
        { y: y(low[yr]), color: "var(--ink-muted)" },
      ],
    };
  };

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${width} ${H}`} className="h-auto w-full" aria-hidden>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={L} x2={width - R + 8} y1={y(t)} y2={y(t)} className="stroke-rule" />
            <text x={L - 8} y={y(t) + 4} textAnchor="end" className="fill-ink-muted font-figure text-[11px]">
              {axis(t)}
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
        {showTarget && (
          <g>
            <line x1={L} x2={x(years)} y1={y(showTarget.cents)} y2={y(showTarget.cents)} className="stroke-ink-muted" strokeDasharray="2 4" />
            <text x={x(0) + 6} y={y(showTarget.cents) - 6} className="fill-ink-muted text-[12px]">
              Target {formatBRL(showTarget.cents).replace(",00", "")}
            </text>
          </g>
        )}
        <path d={`M${pts(base).join(" L")}`} fill="none" className="stroke-tinta" strokeWidth={2.25} strokeDasharray="6 4" />
        <line x1={x(0)} x2={x(0)} y1={T} y2={y(0)} className="stroke-rule-strong" strokeDasharray="2 3" />
        <circle cx={x(0)} cy={y(base[0])} r={4} className="fill-ink" />
        {showTarget && crossing !== null && (
          <g>
            <circle cx={x(crossing)} cy={y(showTarget.cents)} r={4} className="fill-ink stroke-paper-raised" strokeWidth={2} />
            {showTarget.when && (
              <text
                x={x(crossing)}
                y={y(showTarget.cents) + 22}
                textAnchor={x(crossing) > width - R - 40 ? "end" : "middle"}
                className="fill-ink font-figure text-[12px] font-medium"
              >
                {showTarget.when}
              </text>
            )}
          </g>
        )}
        {label(high[years], 4 + dy[0], "fill-ink-muted")}
        {label(base[years], 4 + dy[1], "fill-tinta font-medium")}
        {label(low[years], 4 + dy[2], "fill-ink-muted")}
      </svg>
      <ChartCursor
        W={width}
        H={H}
        plot={{ left: L, right: width - R, top: T, bottom: H - B }}
        xs={xs}
        start={past}
        unit="year"
        read={read}
        label={
          `Projection of invested money in today's money. From ${formatBRL(base[0])} today, the base scenario reaches about ${formatBRL(rounded(base[years]))} in ${startYear + years}, ` +
          `between ${formatBRL(rounded(low[years]))} and ${formatBRL(rounded(high[years]))}.` +
          (showTarget && crossing !== null && showTarget.when ? ` It crosses ${formatBRL(showTarget.cents)} in ${showTarget.when}.` : "")
        }
      />
    </div>
  );
}

/** A step for about three gridlines: 1, 2, 2,5 or 5 times a power of ten. */
function niceStep(max: number): number {
  const raw = max / 3;
  const pow = 10 ** Math.floor(Math.log10(raw));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * pow >= raw) return m * pow;
  return 10 * pow;
}
