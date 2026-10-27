import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

type Props = {
  tone?: "neutral" | "tinta" | "inflow" | "warning" | "danger";
  /** Dashed outline: for things that may still change, like Pending. */
  dashed?: boolean;
  /** Set in the figure font: counts and "3 of 10". */
  figure?: boolean;
  icon?: LucideIcon;
  className?: string;
  children: ReactNode;
};

const tones = {
  neutral: "bg-paper-sunken text-ink-muted",
  tinta: "bg-tinta-soft text-tinta",
  inflow: "bg-inflow-soft text-inflow",
  warning: "bg-warning-soft text-warning",
  danger: "bg-danger-soft text-danger",
};

/** A short label next to what it qualifies. A state colour always comes with a word. */
export function Badge({ tone = "neutral", dashed, figure, icon: Icon, className, children }: Props) {
  return (
    <span
      className={cn(
        "inline-flex h-5 items-center gap-1 rounded-sm border border-transparent px-1.5 align-middle whitespace-nowrap",
        figure ? "font-figure text-[11px] leading-4 font-medium" : "text-[12px] leading-4 font-semibold",
        tones[tone],
        dashed && "border-dashed border-rule-strong bg-transparent",
        className,
      )}
    >
      {Icon && <Icon className="size-3" strokeWidth={2.25} aria-hidden />}
      {children}
    </span>
  );
}
