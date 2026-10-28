import { useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";

import { Amount } from "@/components/Amount";
import { formatBRL, type Sign } from "@/lib/format";

export type CursorRow = {
  label: string;
  cents: number;
  /** The series' colour, as a CSS value. */
  color: string;
  /** The comparison or the projection. */
  dashed?: boolean;
  sign?: Sign;
  estimate?: boolean;
};

export type CursorRead = {
  /** The point: "23 Oct", "Mar 2026". */
  label: string;
  rows: CursorRow[];
  /** One dot per line, y in the chart's units. */
  dots: { y: number; color: string }[];
};

type Props = {
  /** The chart's drawing size: the container is this box, scaled. */
  W: number;
  H: number;
  /** Where the points are, in the same units. */
  plot: { left: number; right: number; top: number; bottom: number };
  /** Each point's x, in order. */
  xs: number[];
  read: (i: number) => CursorRead;
  /** What the chart shows, for screen readers. */
  label: string;
  /** What one point is: "day", "month". */
  unit: string;
  /** Where the arrow keys start: usually today. */
  start?: number;
  onPick?: (i: number | null) => void;
};

/**
 * Reads a line chart at one point (Cédula: ChartCursor): a dashed line, a dot
 * on each series and a box with the values, beside the line on the side with
 * room. Mouse, touch and drag, or the arrow keys once the chart has focus;
 * it snaps to real points. Goes inside a relative box the size of the chart.
 */
export function ChartCursor({ W, H, plot, xs, read, label, unit, start, onPick }: Props) {
  const [at, setAtState] = useState<number | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const [boxLeft, setBoxLeft] = useState(0);
  const n = xs.length;
  const setAt = (i: number | null) => {
    setAtState(i);
    onPick?.(i);
  };

  const r = at === null ? null : read(at);
  const x = at === null ? 0 : xs[at];

  // 12px to the right of the line if it fits, else to the left, else as close as the edges allow.
  useLayoutEffect(() => {
    if (at === null || !root.current || !box.current) return;
    const cw = root.current.offsetWidth;
    const bw = box.current.offsetWidth;
    const px = (x / W) * cw;
    setBoxLeft(px + 12 + bw <= cw ? px + 12 : px - 12 - bw >= 0 ? px - 12 - bw : Math.min(Math.max(px - bw / 2, 0), cw - bw));
  }, [at, x, W]);

  function pick(e: PointerEvent<HTMLDivElement>) {
    const b = e.currentTarget.getBoundingClientRect();
    const vx = plot.left + ((e.clientX - b.left) / b.width) * (plot.right - plot.left);
    let best = 0;
    for (let i = 1; i < n; i++) if (Math.abs(xs[i] - vx) < Math.abs(xs[best] - vx)) best = i;
    if (best !== at) setAt(best);
  }
  function key(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Escape") return setAt(null);
    const cur = at ?? start ?? n - 1;
    const next = { ArrowLeft: cur - 1, ArrowRight: cur + 1, Home: 0, End: n - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    setAt(Math.min(n - 1, Math.max(0, at === null ? cur : next)));
  }

  const pct = (v: number, of: number) => `${(v / of) * 100}%`;
  return (
    <div ref={root} className="pointer-events-none absolute inset-0">
      {r && (
        <div aria-hidden>
          <div className="absolute border-l border-dashed border-ink-muted" style={{ left: pct(x, W), top: pct(plot.top, H), bottom: pct(H - plot.bottom, H) }} />
          {r.dots.map((d, i) => (
            <span
              key={i}
              className="absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-paper-raised"
              style={{ left: pct(x, W), top: pct(d.y, H), background: d.color }}
            />
          ))}
          <div
            ref={box}
            className="absolute z-10 flex flex-col gap-0.5 rounded-md border border-rule bg-paper-raised px-2.5 py-2 whitespace-nowrap shadow-pop"
            style={{ left: boxLeft, top: pct(plot.top, H) }}
          >
            <span className="type-figure-sm text-ink-muted">{r.label}</span>
            {r.rows.map((row) => (
              <span key={row.label} className="flex items-center justify-between gap-4">
                <span className="inline-flex items-center gap-1.5 type-caption text-ink-muted">
                  <span className="w-3 border-t-2" style={{ borderColor: row.color, borderTopStyle: row.dashed ? "dashed" : "solid" }} />
                  {row.label}
                </span>
                <Amount cents={row.cents} sign={row.sign ?? "none"} estimate={row.estimate} />
              </span>
            ))}
          </div>
        </div>
      )}
      <div
        role="img"
        tabIndex={0}
        aria-label={`${label} Use the arrow keys to read a ${unit}.`}
        className="pointer-events-auto absolute cursor-crosshair touch-pan-y rounded-xs outline-none focus-visible:focus-ring"
        style={{ left: pct(plot.left, W), right: pct(W - plot.right, W), top: pct(plot.top, H), bottom: pct(H - plot.bottom, H) }}
        onPointerMove={pick}
        onPointerDown={pick}
        onPointerLeave={(e) => e.pointerType === "mouse" && setAt(null)}
        onKeyDown={key}
        onBlur={() => setAt(null)}
      />
      <span className="sr-only" aria-live="polite">
        {r ? `${r.label}: ${r.rows.map((row) => `${row.label} ${formatBRL(row.cents, { sign: row.sign ?? "none" })}`).join(", ")}` : ""}
      </span>
    </div>
  );
}
