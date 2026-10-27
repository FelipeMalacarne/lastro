import { ArrowLeftRight, Clock, CreditCard } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag, type NoteColor } from "@/components/CategoryTag";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";

export type RowKind = "expense" | "income" | "transfer" | "bill_payment";

type Props = {
  date: string;
  description: string;
  /** "Nubank · credit card", "Nubank checking → Itaú checking". */
  meta?: string;
  kind: RowKind;
  amountCents: number;
  pending?: boolean;
  /** "3 of 10". */
  installment?: string;
  /**
   * The category; null means none yet. Until categorization exists (M2),
   * pass categorized={false} and the row says "Uncategorized" quietly
   * instead of asking for a review.
   */
  category?: { name: string; color: NoteColor } | null;
  categorized?: boolean;
  onCategoryClick?: () => void;
  selected?: boolean;
  /** Where the row leads (its detail): the whole row becomes a link. */
  to?: string;
  className?: string;
};

/**
 * One movement. Transfers between my accounts and card bill payments carry
 * no sign and no category: they are listed but never counted. Below 640px
 * the row stacks into two lines.
 */
export function TransactionRow(p: Props) {
  const neutral = p.kind === "transfer" || p.kind === "bill_payment";
  let cat;
  if (neutral) {
    cat = <span className="type-caption text-ink-muted">{p.kind === "transfer" ? "Between your accounts" : "Not spending"}</span>;
  } else if (p.categorized === false) {
    cat = <span className="type-caption text-ink-muted">Uncategorized</span>;
  } else {
    cat = <CategoryTag name={p.category?.name} color={p.category?.color} onClick={p.onCategoryClick} />;
  }
  const className = cn(
    "grid min-h-11 items-center gap-x-4 border-b border-rule bg-paper-raised px-4 py-2 sm:px-5",
        "grid-cols-[minmax(0,1fr)_auto] gap-y-0.5 [grid-template-areas:'main_amount''main_date''cat_cat']",
        "sm:grid-cols-[52px_minmax(0,1fr)_minmax(0,168px)_minmax(112px,auto)] sm:[grid-template-areas:'date_main_cat_amount']",
    p.selected && "bg-tinta-soft shadow-[inset_2px_0_0_var(--tinta)]",
    p.to && "text-ink no-underline transition-colors hover:text-ink focus-visible:focus-ring",
    p.to && !p.selected && "hover:bg-paper-sunken",
    p.className,
  );
  const body = (
    <>
      <span className="type-figure-sm self-end text-right text-ink-muted [grid-area:date] sm:self-center sm:text-left">{p.date}</span>
      <div className="min-w-0 [grid-area:main]">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="min-w-0 truncate type-body-strong">{p.description}</span>
          {p.pending && (
            <Badge dashed icon={Clock}>
              Pending
            </Badge>
          )}
          {p.kind === "transfer" && <Badge icon={ArrowLeftRight}>Transfer</Badge>}
          {p.kind === "bill_payment" && <Badge icon={CreditCard}>Card bill</Badge>}
          {p.installment && <Badge figure>{p.installment}</Badge>}
        </div>
        {p.meta && <div className="type-caption text-ink-muted">{p.meta}</div>}
      </div>
      {/* On a phone the meta line already says it; the column only adds noise. */}
      <div className={cn("flex min-w-0 [grid-area:cat]", (neutral || p.categorized === false) && "max-sm:hidden")}>{cat}</div>
      <div className="self-start text-right [grid-area:amount] sm:self-center">
        <Amount cents={p.amountCents} sign={neutral ? "none" : "always"} tone={neutral ? "muted" : "auto"} pending={p.pending} />
      </div>
    </>
  );
  if (p.to) {
    return (
      <Link to={p.to} aria-current={p.selected ? "true" : undefined} className={className}>
        {body}
      </Link>
    );
  }
  return (
    <div aria-selected={p.selected || undefined} className={className}>
      {body}
    </div>
  );
}
