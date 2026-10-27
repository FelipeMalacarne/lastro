import { useState, type KeyboardEvent, type PointerEvent } from "react";

import { Amount } from "@/components/Amount";
import { formatAxis, formatDate, niceStep } from "@/lib/format";

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
 * right edge. Hover or arrow keys pick a day; its date and balance float
 * over the chart.
 */
export function BalanceChart({ points, label, onHover }: Props) {
  const [hover, setHoverState] = useState<number | null>(null);
  const setHover = (i: number | null) => {
    setHoverState(i);
    onHover?.(i == null ? null : points[i]);
  };
  if (points.length === 0) return null;

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

  const pick = (e: PointerEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    setHover(Math.round(ratio * (n - 1)));
  };
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const cur = hover ?? n - 1;
    const next = { ArrowLeft: cur - 1, ArrowRight: cur + 1, Home: 0, End: n - 1 }[e.key];
    if (e.key === "Escape") setHover(null);
    if (next === undefined) return;
    e.preventDefault();
    setHover(Math.min(n - 1, Math.max(0, next)));
  };
  const h = hover != null ? { p: points[hover], x: x(hover) / W, y: y(points[hover].cents) / H } : null;
  const shift = h ? (h.x < 0.14 ? "0" : h.x > 0.86 ? "-100%" : "-50%") : "0";

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
      {h && (
        <>
          <div className="pointer-events-none absolute inset-y-0 border-l border-dashed border-ink-muted" style={{ left: `${h.x * 100}%` }} />
          <div
            className="pointer-events-none absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-paper-raised bg-tinta"
            style={{ left: `${h.x * 100}%`, top: `${h.y * 100}%` }}
          />
          <div
            className="pointer-events-none absolute -top-2 z-10 flex flex-col gap-0.5 rounded-md border border-rule bg-paper-raised px-2.5 py-2 whitespace-nowrap shadow-pop"
            style={{ left: `${h.x * 100}%`, transform: `translateX(${shift})` }}
          >
            <span className="type-figure-sm text-ink-muted">{formatDate(h.p.date)}</span>
            <Amount cents={h.p.cents} />
          </div>
        </>
      )}
      <div
        role="img"
        tabIndex={0}
        aria-label={`${label}. Use the arrow keys to read a day.`}
        className="absolute inset-0 cursor-crosshair rounded-xs focus-visible:focus-ring"
        onPointerMove={pick}
        onPointerDown={pick}
        onPointerLeave={(e) => e.pointerType === "mouse" && setHover(null)}
        onKeyDown={onKey}
        onBlur={() => setHover(null)}
      />
      <span className="sr-only" aria-live="polite">
        {h ? `${formatDate(h.p.date)}: ${h.p.cents < 0 ? "minus " : ""}R$ ${formatAxis(Math.abs(h.p.cents))}` : ""}
      </span>
    </div>
  );
}
