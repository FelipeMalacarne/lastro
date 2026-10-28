import type { CSSProperties } from "react";

import { cn } from "@/lib/utils";

/**
 * Cédula's Skeleton: holds the place of what is loading, at its size, so
 * nothing moves when it arrives. Never a stand-in figure.
 */
export function Skeleton({ className, style }: { className?: string; style?: CSSProperties }) {
  return (
    <span
      aria-hidden
      className={cn("block h-4 max-w-full rounded-xs bg-paper-sunken [animation:lt-show_0s_150ms_backwards,lt-breathe_1.6s_150ms_ease-in-out_infinite]", className)}
      style={style}
    />
  );
}

/** Rows of a list while it loads: a line and a caption on the left, a figure on the right. */
export function SkeletonRows({ count = 5, className }: { count?: number; className?: string }) {
  const widths = ["58%", "44%", "66%", "50%", "38%", "62%", "48%"];
  return (
    <div aria-busy className={cn("flex flex-col", className)}>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex min-h-14 items-center justify-between gap-4 border-b border-rule px-4 py-2.5 last:border-b-0 sm:px-5">
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            <Skeleton style={{ width: widths[i % widths.length] }} />
            <Skeleton className="h-3 w-32" />
          </div>
          <Skeleton className="w-20" />
        </div>
      ))}
    </div>
  );
}

/** A labelled figure while it loads: label, amount and the line under it. */
export function SkeletonStat({ hero, className }: { hero?: boolean; className?: string }) {
  return (
    <div aria-busy className={cn("flex flex-col gap-1.5", className)}>
      <Skeleton className="h-3 w-16" />
      <Skeleton className={hero ? "h-12 w-60" : "h-8 w-32"} />
      <Skeleton className="h-3.5 w-28" />
    </div>
  );
}
