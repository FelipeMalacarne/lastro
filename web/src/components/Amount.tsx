import { signFor, splitBRL, type Sign } from "@/lib/format";
import { cn } from "@/lib/utils";

export type Tone = "auto" | "ink" | "muted" | "inflow" | "warning" | "danger";

type Props = {
  cents: number;
  size?: "hero" | "lg" | "md" | "sm";
  /** "negative" shows only a minus; "always" adds + to inflows; "none" shows neither. */
  sign?: Sign;
  /** "auto": inflow green when the sign is shown and positive, ink otherwise. */
  tone?: Tone;
  /** Not settled yet: muted, with a dashed underline. */
  pending?: boolean;
  /** An estimate: "≈" in front. */
  estimate?: boolean;
  className?: string;
};

export const toneClass: Record<Exclude<Tone, "auto">, string> = {
  ink: "text-ink",
  muted: "text-ink-muted",
  inflow: "text-inflow",
  warning: "text-warning",
  danger: "text-danger",
};

const sizeClass = {
  hero: "type-amount-hero",
  lg: "type-amount-lg",
  md: "type-figure",
  sm: "type-figure-sm",
};

/**
 * Money, always from cents. Large sizes set the reais wide, like the numeral
 * on a note, with a small raised R$ and smaller cents.
 */
export function Amount({ cents, size = "md", sign = "negative", tone = "auto", pending, estimate, className }: Props) {
  let t: Exclude<Tone, "auto"> = tone === "auto" ? (sign === "always" && cents > 0 ? "inflow" : "ink") : tone;
  if (pending) t = "muted";
  const p = splitBRL(cents);
  const s = signFor(cents, sign);
  const big = size === "hero" || size === "lg";
  const raised = big ? "align-[0.62em] text-[0.5em] font-semibold tracking-normal [font-stretch:100%]" : "";
  const underline = pending ? "underline decoration-rule-strong decoration-dashed decoration-1 underline-offset-4" : "";
  return (
    <span
      className={cn("whitespace-nowrap tabular-nums", sizeClass[size], toneClass[t], className)}
      title={pending ? "Pending: may still change" : estimate ? "Estimate" : undefined}
    >
      {estimate && <span>≈&nbsp;</span>}
      {s && <span className={cn(raised, big && "mr-px")}>{s}</span>}
      <span className={cn(raised, big && t !== "inflow" && "text-ink-muted")}>R$&nbsp;</span>
      <span className={underline}>{p.units}</span>
      {/* A rounded estimate claims no cents. */}
      {!(estimate && Math.round(cents) % 100 === 0) && <span className={cn(underline, big && "text-[0.6em] tracking-normal")}>{p.cents}</span>}
    </span>
  );
}
