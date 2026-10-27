import { ChevronLeft, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

type Props = {
  label: string;
  onPrev: () => void;
  onNext: () => void;
  nextDisabled?: boolean;
  className?: string;
};

const step =
  "inline-flex w-[34px] cursor-pointer items-center justify-center bg-transparent text-ink hover:bg-paper-sunken focus-visible:focus-ring disabled:cursor-not-allowed disabled:bg-transparent disabled:text-rule-strong";

/** The month every screen is looking at. Months, never free date ranges. */
export function MonthSwitcher({ label, onPrev, onNext, nextDisabled, className }: Props) {
  return (
    <div
      role="group"
      aria-label="Month"
      className={cn("inline-flex h-9 items-stretch rounded-md border border-rule-strong bg-paper-raised", className)}
    >
      <button type="button" className={cn(step, "rounded-l-md border-r border-rule")} onClick={onPrev} aria-label="Previous month">
        <ChevronLeft className="size-4" aria-hidden />
      </button>
      <span aria-live="polite" className="inline-flex min-w-[112px] items-center justify-center px-3 text-[14px] leading-5 font-semibold">
        {label}
      </span>
      <button
        type="button"
        className={cn(step, "rounded-r-md border-l border-rule")}
        onClick={onNext}
        disabled={nextDisabled}
        aria-label="Next month"
      >
        <ChevronRight className="size-4" aria-hidden />
      </button>
    </div>
  );
}
