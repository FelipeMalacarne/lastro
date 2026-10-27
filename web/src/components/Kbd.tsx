import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** A key, shown next to the action it triggers. */
export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        "inline-block min-w-[18px] rounded-sm border border-b-2 border-rule-strong bg-paper-raised px-[5px] text-center font-figure text-[11px] leading-4 font-medium text-ink-muted",
        className,
      )}
    >
      {children}
    </kbd>
  );
}
