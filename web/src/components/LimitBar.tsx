import { formatBRL } from "@/lib/format";

type Props = {
  limitCents: number;
  /** What is in use besides the installments: unpaid bills and purchases. */
  restCents: number;
  /** Installments still to be billed, which hold the limit meanwhile. */
  futureCents: number;
  availableCents: number;
};

/** A card's limit: what is in use (solid), the installments to come (dashed), what is available. */
export function LimitBar({ limitCents, restCents, futureCents, availableCents }: Props) {
  const pct = (c: number) => `${Math.max(0, Math.min(100, (c / Math.max(limitCents, 1)) * 100))}%`;
  return (
    <div className="flex flex-col gap-2">
      <div
        role="img"
        aria-label={`${formatBRL(restCents + futureCents)} of ${formatBRL(limitCents)} in use: ${formatBRL(futureCents)} in installments to come, ${formatBRL(restCents)} in bills and purchases. ${formatBRL(availableCents)} available.`}
        className="flex h-3.5 gap-0.5 overflow-hidden rounded-xs bg-paper-sunken"
      >
        <span className="bg-ink" style={{ width: pct(restCents) }} />
        <span className="box-border border-[1.5px] border-dashed border-rule-strong bg-paper-raised" style={{ width: pct(futureCents) }} />
      </div>
      <div className="flex flex-wrap gap-x-5 gap-y-1.5 type-caption text-ink-muted">
        <span className="inline-flex items-center gap-1.5">
          <span className="size-2.5 rounded-xs bg-ink" aria-hidden />
          Bills and purchases {formatBRL(restCents)}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="box-border size-2.5 rounded-xs border-[1.5px] border-dashed border-rule-strong" aria-hidden />
          Installments to come {formatBRL(futureCents)}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span className="box-border size-2.5 rounded-xs border border-rule bg-paper-sunken" aria-hidden />
          Available {formatBRL(availableCents)}
        </span>
      </div>
    </div>
  );
}
