import type { ReactNode } from "react";

import { Amount, type Tone } from "@/components/Amount";
import type { Sign } from "@/lib/format";
import { cn } from "@/lib/utils";

type Props = {
  label: string;
  cents: number;
  sign?: Sign;
  tone?: Tone;
  /** A comparison in a state colour, e.g. "R$ 1.120,00 less than by 26 Sep". */
  delta?: { text: string; tone: "inflow" | "warning" | "muted" };
  caption?: ReactNode;
  className?: string;
};

/** A labelled figure: the label, the amount, and one line about it. */
export function Stat({ label, cents, sign = "negative", tone = "ink", delta, caption, className }: Props) {
  return (
    <div className={cn("flex min-w-0 flex-col gap-1", className)}>
      <span className="type-label text-ink-muted">{label}</span>
      <Amount cents={cents} size="lg" sign={sign} tone={tone} />
      {(delta || caption) && (
        <span className="flex flex-wrap gap-x-2 type-caption text-ink-muted">
          {delta && <span className={delta.tone === "inflow" ? "text-inflow" : delta.tone === "warning" ? "text-warning" : undefined}>{delta.text}</span>}
          {caption && <span>{caption}</span>}
        </span>
      )}
    </div>
  );
}
