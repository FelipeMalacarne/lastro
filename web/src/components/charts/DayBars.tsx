import { useState } from "react";

import { Amount } from "@/components/Amount";
import type { DayView } from "@/lib/api";
import { formatAxis, formatBRL, formatDay, formatWeekday, niceStep } from "@/lib/format";
import { cn } from "@/lib/utils";

/** What the bars measure: spending, money in that counts, or money moved. */
export type Measure = "spent" | "in" | "moved";

type Props = {
  month: string; // "2026-10"
  days: DayView[];
  previous: DayView[];
  today: string; // "2026-10-26"
  selected: string | null;
  onSelect: (day: string | null) => void;
  measure?: Measure;
  /** The bars' colour, a CSS colour: ink unless a filter gives one. */
  color?: string;
};

const verbs: Record<Measure, string> = { spent: "spent", in: "came in", moved: "moved" };

/** What a day adds up to in one measure; zero for a day without any. */
export function dayValue(d: DayView | undefined, m: Measure): number {
  return !d ? 0 : m === "in" ? d.inCents : m === "moved" ? d.movedCents : d.spentCents;
}

/**
 * A month's spending (or money in, or money moved) day by day, the month
 * before's same day faint behind each bar. A day far above the rest (rent)
 * is cut at a dotted line with its figure on top, so the other days stay
 * readable. Click a day to list it.
 */
export function DayBars({ month, days, previous, today, selected, onSelect, measure = "spent", color = "var(--ink)" }: Props) {
  const [hover, setHover] = useState<string | null>(null);
  const [y, m] = month.split("-").map(Number);
  const count = new Date(y, m, 0).getDate();
  const byDate = new Map(days.map((d) => [d.date, d]));
  const before = new Map(previous.map((d) => [Number(d.date.slice(8)), dayValue(d, measure)]));
  const all = Array.from({ length: count }, (_, i) => {
    const date = `${month}-${String(i + 1).padStart(2, "0")}`;
    return { date, n: i + 1, v: dayValue(byDate.get(date), measure), count: byDate.get(date)?.purchases ?? 0, prev: before.get(i + 1) ?? 0, future: date > today };
  });

  // The scale: the largest day, unless one day dwarfs the others; then a
  // cap a little above the ninth-largest-in-ten.
  const values = all.flatMap((d) => [d.v, d.prev]).filter((v) => v > 0).sort((a, b) => a - b);
  const max = values[values.length - 1] ?? 1;
  const p90 = values[Math.floor(values.length * 0.9)] ?? max;
  const capped = max > p90 * 2.5;
  const step = niceStep(capped ? p90 * 1.4 : max, 3);
  const top = Math.ceil((capped ? p90 * 1.4 : max) / step) * step || step;
  const grid: number[] = [];
  for (let v = 0; v <= top; v += step) grid.push(v);
  const h = (v: number) => `${(Math.min(v, top) / top) * 100}%`;
  const col = (i: number) => `calc((100% + 3px) * ${(i / count).toFixed(4)} - 1.5px)`;
  const hd = all.find((d) => d.date === hover);
  const hi = hd ? hd.n - 1 : 0;
  const shift = (hi + 0.5) / count < 0.12 ? "0" : (hi + 0.5) / count > 0.88 ? "-100%" : "-50%";
  const todayIndex = all.findIndex((d) => d.date === today);
  const prevName = new Date(y, m - 2, 1).toLocaleString("en", { month: "short" });
  const verb = verbs[measure];

  return (
    <div className="flex flex-col gap-3">
      <div className="relative mt-6 ml-14 h-[168px] max-lg:ml-0 max-lg:h-24" onMouseLeave={() => setHover(null)}>
        {grid.map((v) => (
          <div key={v} className="pointer-events-none absolute inset-x-0" style={{ bottom: `calc(24px + (100% - 24px) * ${(v / top).toFixed(3)})` }}>
            <div className={v === 0 ? "border-t border-rule-strong" : v === top && capped ? "border-t border-dotted border-rule-strong" : "border-t border-rule"} />
            <span className="absolute right-[calc(100%+10px)] translate-y-[-50%] type-figure-sm whitespace-nowrap text-ink-muted max-lg:hidden">{formatAxis(v)}</span>
          </div>
        ))}
        <div className="absolute inset-x-0 top-0 bottom-6 grid items-end gap-[3px] max-lg:gap-0.5" style={{ gridTemplateColumns: `repeat(${count}, minmax(0, 1fr))` }}>
          {all.map((d) => {
            const on = selected === d.date;
            return (
              <button
                key={d.date}
                type="button"
                disabled={d.future}
                aria-pressed={on}
                aria-label={d.future ? `${formatDay(d.date)}: still to come` : `${formatDay(d.date)}: ${formatBRL(d.v, { sign: "none" })} ${verb}`}
                onClick={() => onSelect(on ? null : d.date)}
                onMouseEnter={() => !d.future && setHover(d.date)}
                onFocus={() => !d.future && setHover(d.date)}
                onBlur={() => setHover(null)}
                className={cn("relative h-full rounded-xs focus-visible:focus-ring", !d.future && "cursor-pointer", on && "bg-tinta-soft")}
              >
                <span className="absolute inset-x-0 bottom-0 rounded-t-xs bg-rule" style={{ height: h(d.prev) }} />
                <span
                  className={cn("absolute inset-x-[22%] bottom-0 rounded-t-xs max-lg:inset-x-[15%]", (on || hover === d.date) && "bg-tinta")}
                  style={{ height: h(d.v), background: on || hover === d.date ? undefined : color }}
                />
                {d.v > top && (
                  <span className="pointer-events-none absolute bottom-[calc(100%+2px)] left-1/2 -translate-x-1/2 type-figure-sm whitespace-nowrap text-ink max-lg:left-0 max-lg:translate-x-0">
                    {formatAxis(d.v)}
                  </span>
                )}
              </button>
            );
          })}
        </div>
        <div className="pointer-events-none absolute inset-x-0 bottom-0 grid gap-[3px] max-lg:gap-0.5" style={{ gridTemplateColumns: `repeat(${count}, minmax(0, 1fr))` }}>
          {all.map((d) => (
            <span key={d.date} className="text-center type-figure-sm whitespace-nowrap text-ink-muted">
              {d.n === 1 || d.n % 5 === 0 ? d.n : ""}
            </span>
          ))}
        </div>
        {todayIndex >= 0 && todayIndex < count - 1 && (
          <>
            <div className="pointer-events-none absolute -top-[18px] bottom-6 border-l border-ink" style={{ left: col(todayIndex + 1) }} />
            <span className="pointer-events-none absolute -top-[22px] translate-x-1.5 text-[12px] leading-4 font-semibold" style={{ left: col(todayIndex + 1) }}>
              Today
            </span>
          </>
        )}
        {hd && (
          <div
            className="pointer-events-none absolute bottom-[calc(100%-8px)] z-10 flex flex-col gap-0.5 rounded-md border border-rule bg-paper-raised px-2.5 py-2 whitespace-nowrap shadow-pop"
            style={{ left: `calc((100% + 3px) * ${((hi + 0.5) / count).toFixed(4)} - 1.5px)`, transform: `translateX(${shift})` }}
          >
            <span className="type-caption text-ink-muted">{formatWeekday(hd.date)}</span>
            <Amount
              cents={measure === "spent" ? -hd.v : hd.v}
              sign={measure === "in" ? "always" : measure === "moved" ? "none" : "negative"}
              tone={measure === "moved" ? "muted" : "auto"}
            />
            <span className="type-caption text-ink-muted">
              {measure === "spent" && `${hd.count} ${hd.count === 1 ? "purchase" : "purchases"} · `}
              {hd.n} {prevName}: {formatBRL(hd.prev, { sign: "none" })}
            </span>
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-4 type-caption text-ink-muted">
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-xs" style={{ background: color }} aria-hidden />
          {new Date(y, m - 1, 1).toLocaleString("en", { month: "long" })}, {verb}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-xs bg-rule" aria-hidden />
          Same day in {new Date(y, m - 2, 1).toLocaleString("en", { month: "long" })}
        </span>
      </div>
    </div>
  );
}
