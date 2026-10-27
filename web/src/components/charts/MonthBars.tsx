import type { MonthView } from "@/lib/api";
import { formatBRL, formatMonth } from "@/lib/format";
import { cn } from "@/lib/utils";

type Props = {
  months: MonthView[];
  selected: string;
  onSelect: (month: string) => void;
};

/** Money in (inflow) and out (ink) of an account, month by month. Each month is a button. */
export function MonthBars({ months, selected, onSelect }: Props) {
  const max = Math.max(1, ...months.map((m) => Math.max(m.inCents, -m.outCents)));
  return (
    <div className="grid h-[168px] gap-1 max-lg:h-[132px] max-lg:gap-0.5" style={{ gridTemplateColumns: `repeat(${months.length}, minmax(0, 1fr))` }}>
      {months.map((m) => {
        const on = m.month === selected;
        return (
          <button
            key={m.month}
            type="button"
            aria-pressed={on}
            aria-label={`${formatMonth(m.month)}: in ${formatBRL(m.inCents)}, out ${formatBRL(m.outCents)}`}
            onClick={() => onSelect(m.month)}
            className={cn(
              "flex min-w-0 flex-col justify-end gap-1.5 rounded-sm px-0.5 pt-1.5 pb-1 transition-colors hover:bg-paper-sunken focus-visible:focus-ring",
              on && "bg-paper-sunken",
            )}
          >
            <span className="flex flex-1 items-end justify-center gap-[3px]">
              <span className="w-[38%] max-w-3.5 rounded-t-xs bg-inflow" style={{ height: `${(m.inCents / max) * 100}%` }} />
              <span className="w-[38%] max-w-3.5 rounded-t-xs bg-ink" style={{ height: `${(-m.outCents / max) * 100}%` }} />
            </span>
            <span className={cn("text-center type-figure-sm", on ? "text-ink" : "text-ink-muted")}>{formatMonth(m.month, true).slice(0, 3)}</span>
          </button>
        );
      })}
    </div>
  );
}
