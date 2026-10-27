import { cn } from "@/lib/utils";

type Props = { current: number; total: number; className?: string };

/** One segment per installment: billed ones muted, the current one ink, the rest dashed. */
export function InstallmentProgress({ current, total, className }: Props) {
  return (
    <span role="img" aria-label={`${current} of ${total} installments`} className={cn("flex gap-0.5", className)}>
      {Array.from({ length: total }, (_, i) => (
        <span
          key={i}
          className={cn(
            "h-2 flex-1 rounded-[1px]",
            i + 1 < current ? "bg-ink-muted" : i + 1 === current ? "bg-ink" : "box-border border border-dashed border-rule-strong",
          )}
        />
      ))}
    </span>
  );
}
