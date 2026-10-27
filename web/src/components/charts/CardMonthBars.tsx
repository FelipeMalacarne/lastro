import { useState } from "react";

import { Amount } from "@/components/Amount";
import type { MonthView } from "@/lib/api";
import { formatAxis, formatMonth, niceStep } from "@/lib/format";
import { cn } from "@/lib/utils";

type Props = { months: MonthView[]; current: string };

/**
 * A card's spending by month: solid bars for the months billed or open,
 * dashed ones for the installments already committed to the coming months,
 * and today between them. Hover or focus a month for its figure.
 */
export function CardMonthBars({ months, current }: Props) {
  const [hover, setHover] = useState<number | null>(null);
  if (months.length === 0) return null;
  const n = months.length;
  const max = Math.max(1, ...months.map((m) => m.spentCents));
  const step = niceStep(max, 3);
  const top = Math.ceil(max / step) * step;
  const grid: number[] = [];
  for (let v = 0; v <= top; v += step) grid.push(v);
  const firstFuture = months.findIndex((m) => m.future);
  const col = (i: number) => `calc((100% + 6px) * ${(i / n).toFixed(4)} - 3px)`;
  const m = hover != null ? months[hover] : null;
  const shift = hover == null ? "0" : (hover + 0.5) / n < 0.12 ? "0" : (hover + 0.5) / n > 0.88 ? "-100%" : "-50%";

  return (
    <div className="relative mt-5 ml-14 h-[200px] max-lg:ml-0 max-lg:h-[150px]" onMouseLeave={() => setHover(null)}>
      {grid.map((v) => (
        <div key={v} className="pointer-events-none absolute inset-x-0" style={{ bottom: `calc(24px + (100% - 24px) * ${(v / top).toFixed(3)})` }}>
          <div className={v === 0 ? "border-t border-rule-strong" : "border-t border-rule"} />
          <span className="absolute right-[calc(100%+10px)] translate-y-[-50%] type-figure-sm whitespace-nowrap text-ink-muted max-lg:hidden">{formatAxis(v)}</span>
        </div>
      ))}
      <div className="absolute inset-x-0 top-0 bottom-6 grid items-end gap-1.5" style={{ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` }}>
        {months.map((mo, i) => (
          <button
            key={mo.month}
            type="button"
            aria-label={`${formatMonth(mo.month)}: ${formatAxis(mo.spentCents)} reais${mo.future ? " committed" : ""}`}
            onMouseEnter={() => setHover(i)}
            onFocus={() => setHover(i)}
            onBlur={() => setHover(null)}
            onClick={() => setHover(hover === i ? null : i)}
            className="flex h-full items-end rounded-xs focus-visible:focus-ring"
          >
            <span
              className={cn(
                "box-border w-full rounded-t-xs",
                mo.future ? cn("border-[1.5px] border-dashed", hover === i ? "border-tinta" : "border-rule-strong") : hover === i ? "bg-tinta" : "bg-ink",
              )}
              style={{ height: `${(mo.spentCents / top) * 100}%` }}
            />
          </button>
        ))}
      </div>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 grid gap-1.5" style={{ gridTemplateColumns: `repeat(${n}, minmax(0, 1fr))` }}>
        {months.map((mo, i) => {
          const [y, mm] = mo.month.split("-").map(Number);
          const show = i === 0 || mm === 1 || i % (n > 14 ? 3 : 1) === 0;
          return (
            <span key={mo.month} className="text-center type-figure-sm whitespace-nowrap text-ink-muted">
              {show && formatMonth(mo.month, true).slice(0, 3)}
              {/* On a phone the columns have no room for the year. */}
              {show && (mm === 1 || i === 0) && <span className="max-sm:hidden"> {String(y).slice(2)}</span>}
            </span>
          );
        })}
      </div>
      {firstFuture > 0 && (
        <>
          <div className="pointer-events-none absolute -top-[18px] bottom-6 border-l border-ink" style={{ left: col(firstFuture) }} />
          <span className="pointer-events-none absolute -top-[22px] translate-x-1.5 text-[12px] leading-4 font-semibold" style={{ left: col(firstFuture) }}>
            Today
          </span>
        </>
      )}
      {m && hover != null && (
        <div
          className="pointer-events-none absolute top-0 z-10 flex flex-col gap-0.5 rounded-md border border-rule bg-paper-raised px-2.5 py-2 whitespace-nowrap shadow-pop"
          style={{ left: `calc((100% + 6px) * ${((hover + 0.5) / n).toFixed(4)} - 3px)`, transform: `translateX(${shift})` }}
        >
          <span className="type-caption text-ink-muted">
            {formatMonth(m.month)}
            {m.month === current ? ", to date" : ""}
          </span>
          <Amount cents={m.spentCents} sign="none" />
          <span className="type-caption text-ink-muted">
            {m.future ? "Committed in installments" : m.month === current ? "Open bill" : `${m.purchases} purchases`}
          </span>
        </div>
      )}
    </div>
  );
}
