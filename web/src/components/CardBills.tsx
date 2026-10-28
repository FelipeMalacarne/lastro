import { useState } from "react";
import { Check, CircleAlert, Clock, CreditCard, TriangleAlert, type LucideIcon } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { typeLabel, type Account, type CardBill, type Category } from "@/lib/api";
import { formatBRL, formatDay, formatMonth } from "@/lib/format";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";

// A card's bills (canvas: "Card: the open bill, the next one and the paid
// ones", "Card, phone: a closed bill in a sheet").

/** "November bill", with the year when it is not this one's; "Installments to come" for the next ones. */
export function billName(b: CardBill, today: string): string {
  if (b.state === "next") return "Installments to come";
  if (!b.dueDate) return b.lastDate ? `Bill up to ${formatDay(b.lastDate)}` : "Open bill";
  const [month, year] = formatMonth(b.dueDate.slice(0, 7)).split(" ");
  return year === today.slice(0, 4) ? `${month} bill` : `${month} ${year} bill`;
}

function daysBetween(from: string, to: string): number {
  return Math.round((Date.parse(to) - Date.parse(from)) / 86_400_000);
}

function dueIn(due: string, today: string): string {
  const n = daysBetween(today, due);
  return n === 0 ? "Due today" : n === 1 ? "Due tomorrow" : `Due in ${n} days`;
}

/** Where a bill stands, as a badge. */
export function billBadge(b: CardBill, today: string): { text: string; tone: "neutral" | "tinta" | "inflow" | "warning" | "danger"; icon: LucideIcon; dashed?: boolean } {
  switch (b.state) {
    case "next":
      return { text: "Not open yet", tone: "neutral", icon: Clock, dashed: true };
    case "open":
      return { text: "Open", tone: "tinta", icon: CreditCard };
    case "paid":
      return { text: "Paid", tone: "inflow", icon: Check };
    case "partly_paid":
      return { text: "Partly paid", tone: "warning", icon: TriangleAlert };
    case "overdue":
      return { text: "Overdue", tone: "danger", icon: CircleAlert };
    default:
      return { text: b.dueDate ? dueIn(b.dueDate, today) : "Not paid yet", tone: "neutral", icon: Clock };
  }
}

function from(b: CardBill): string {
  return b.paidFrom ? ` from ${b.paidFrom.institution} ${typeLabel[b.paidFrom.type]}` : "";
}

/** The line under a bill's name in the list. */
function billMeta(b: CardBill): string {
  const n = b.transactions.length;
  switch (b.state) {
    case "next":
      return `${n} ${n === 1 ? "installment" : "installments"} so far`;
    case "open":
      return `${n} ${n === 1 ? "transaction" : "transactions"} so far`;
    case "paid":
      return b.paidOn ? `Paid on ${formatDay(b.paidOn)}${from(b)}` : "Paid";
    case "partly_paid":
      return `${formatBRL(b.paidCents)} paid${b.paidOn ? ` on ${formatDay(b.paidOn)}` : ""}`;
    case "overdue":
      return b.dueDate ? `Nothing paid by ${formatDay(b.dueDate)}` : "Nothing paid";
    default:
      return "Not paid yet";
  }
}

/** The bills, the selected one marked; each opens beside the list, or in a sheet on a phone. */
export function BillsList({ bills, selected, onSelect, today }: { bills: CardBill[]; selected: string | null; onSelect: (id: string) => void; today: string }) {
  return (
    <div className="border-t border-rule">
      <div className="hidden grid-cols-[minmax(0,1fr)_72px_128px_128px] gap-3 border-b border-rule bg-paper-sunken px-5 py-2 type-label text-ink-muted sm:grid" aria-hidden>
        <span>Bill</span>
        <span>Due</span>
        <span className="text-right">Amount</span>
        <span>Status</span>
      </div>
      {bills.map((b) => {
        const on = b.id === selected;
        const badge = billBadge(b, today);
        return (
          <button
            key={b.id}
            type="button"
            aria-pressed={on}
            onClick={() => onSelect(b.id)}
            className={cn(
              "grid min-h-14 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-0 border-b border-rule bg-transparent px-5 py-2 text-left text-ink hover:bg-paper-sunken sm:grid-cols-[minmax(0,1fr)_72px_128px_128px]",
              on && "bg-paper-sunken shadow-[inset_3px_0_0_var(--tinta)]",
            )}
          >
            <span className="flex min-w-0 flex-col">
              <span className="type-body-strong">{billName(b, today)}</span>
              <span className="truncate type-caption text-ink-muted">{billMeta(b)}</span>
            </span>
            <span className="hidden type-figure-sm text-ink-muted sm:block">{b.dueDate ? formatDay(b.dueDate) : "—"}</span>
            <Amount cents={b.totalCents} sign="none" tone={b.state === "next" ? "muted" : "ink"} className="text-right" />
            <span className="flex max-sm:col-span-2">
              <Badge tone={badge.tone} icon={badge.icon} dashed={badge.dashed}>
                {badge.text}
              </Badge>
            </span>
          </button>
        );
      })}
    </div>
  );
}

/** What a bill holds: where it stands, what it is made of, by category, and its transactions. */
export function BillDetail({ b, a, byId, today }: { b: CardBill; a: Account; byId: Map<string, Category>; today: string }) {
  const [all, setAll] = useState(false);
  const badge = billBadge(b, today);
  const bank = a.institution;
  const caption = (() => {
    switch (b.state) {
      case "next":
        return "Already set to go into the bills to come, each in the month it is dated.";
      case "open":
        return `${b.dueDate ? `Due on ${formatDay(b.dueDate)}. ` : ""}Still open: it grows with each purchase until ${bank} closes it.`;
      case "paid":
        return b.paidOn ? `Paid in full on ${formatDay(b.paidOn)}${from(b)}.` : "Paid in full.";
      case "partly_paid":
        return `${formatBRL(b.paidCents)} paid${b.paidOn ? ` on ${formatDay(b.paidOn)}` : ""}${from(b)}.`;
      case "overdue":
        return b.dueDate ? `Closed. It was due on ${formatDay(b.dueDate)}.` : "Closed and not paid.";
      default:
        return b.dueDate ? `Closed. Due on ${formatDay(b.dueDate)}, ${dueIn(b.dueDate, today).slice(4).toLowerCase()}.` : "Closed, not paid yet.";
    }
  })();
  const warn =
    b.state === "partly_paid"
      ? `${formatBRL(b.totalCents - b.paidCents)} was left unpaid and goes into the next bill, with ${bank}’s interest.`
      : b.state === "overdue"
        ? `Nothing came in to pay it. ${bank} charges interest on what is left; pay it in the ${bank} app.`
        : null;
  const parts: { label: string; cents: number; credit?: boolean }[] = [];
  if (b.carriedCents > 0) parts.push({ label: "From the bill before", cents: b.carriedCents });
  if (b.carriedCents < 0) parts.push({ label: "Payments and adjustments", cents: -b.carriedCents, credit: true });
  if (b.chargesCents > 0) parts.push({ label: "Interest and fees", cents: b.chargesCents });
  if (b.purchases.count > 0) parts.push({ label: `Purchases (${b.purchases.count})`, cents: b.purchases.cents });
  if (b.installments.count > 0) parts.push({ label: `Installments (${b.installments.count})`, cents: b.installments.cents });
  if (b.refunds.count > 0) parts.push({ label: `Refunds (${b.refunds.count})`, cents: b.refunds.cents, credit: true });
  const cats = b.byCategory.filter((c) => c.cents > 0);
  const max = Math.max(1, ...cats.map((c) => c.cents));
  // The largest first, or every one newest first; the next bills, by date.
  const sorted = b.state === "next" ? [...b.transactions].reverse() : [...b.transactions].sort((x, y) => x.amountCents - y.amountCents);
  const shown = all ? b.transactions : sorted.slice(0, 5);
  const n = b.transactions.length;

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-1.5">
        <span className="flex">
          <Badge tone={badge.tone} icon={badge.icon} dashed={badge.dashed}>
            {badge.text}
          </Badge>
        </span>
        <Amount cents={b.totalCents} size="lg" sign="none" />
        <span className="type-caption text-ink-muted">
          {caption}
          {b.minimumCents != null && b.state !== "paid" && ` Minimum ${formatBRL(b.minimumCents)}.`}
        </span>
      </div>

      {warn && (
        <p role="note" className={cn("m-0 flex gap-2 rounded-md p-3 text-[14px] leading-5", b.state === "overdue" ? "bg-danger-soft" : "bg-warning-soft")}>
          {b.state === "overdue" ? <CircleAlert className="mt-0.5 size-3.5 flex-none text-danger" aria-hidden /> : <TriangleAlert className="mt-0.5 size-3.5 flex-none text-warning" aria-hidden />}
          <span>{warn}</span>
        </p>
      )}

      {parts.length > 0 && (
        <dl className="m-0 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2 text-[14px] leading-5">
          {parts.map((p) => (
            <div key={p.label} className="contents">
              <dt className="text-ink-muted">{p.label}</dt>
              <dd className="m-0 text-right">
                <Amount cents={p.credit ? p.cents : -p.cents} sign={p.credit ? "always" : "none"} size="sm" />
              </dd>
            </div>
          ))}
          <div className="col-span-2 flex justify-between gap-4 border-t border-ink pt-2">
            <dt className="font-semibold">{b.state === "open" || b.state === "next" ? "So far" : "Total"}</dt>
            <dd className="m-0 text-right">
              <Amount cents={b.totalCents} sign="none" size="sm" />
            </dd>
          </div>
        </dl>
      )}

      {cats.length > 0 && b.state !== "next" && (
        <div className="flex flex-col gap-3 border-t border-rule pt-4">
          <span className="type-label text-ink-muted">By category</span>
          {cats.map((c) => {
            const cat = c.category ? byId.get(c.category) : undefined;
            return (
              <div key={c.category ?? "none"} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1">
                <CategoryTag name={c.category ? (cat?.name ?? "Archived category") : undefined} color={cat?.color} review={!c.category} />
                <Amount cents={c.cents} sign="none" size="sm" />
                <div className="col-span-2 h-1.5 rounded-xs bg-paper-sunken">
                  <div className="h-full rounded-xs" style={{ width: `${(c.cents / max) * 100}%`, background: `var(--${cat?.color ?? "note-200"})` }} />
                </div>
              </div>
            );
          })}
        </div>
      )}

      {n > 0 && (
        <div className="flex flex-col border-t border-rule pt-4">
          <span className="mb-1 type-label text-ink-muted">{all ? `All ${n}, newest first` : b.state === "next" ? "Installments already in them" : "Largest in it"}</span>
          {shown.map((t) => (
            <Link
              key={t.id}
              to={`/transactions?month=${t.date.slice(0, 7)}&account=${a.id}&tx=${t.id}`}
              className="grid min-h-11 grid-cols-[48px_minmax(0,1fr)_auto] items-center gap-3 border-b border-rule text-ink no-underline hover:bg-paper-sunken hover:text-ink"
            >
              <span className="type-figure-sm text-ink-muted">{formatDay(t.date)}</span>
              <span className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="truncate text-[14px] leading-5 font-semibold">{t.description}</span>
                {t.installment && (
                  <Badge figure>
                    {t.installment.number} of {t.installment.total}
                  </Badge>
                )}
                {t.pending && (
                  <Badge dashed icon={Clock}>
                    Pending
                  </Badge>
                )}
              </span>
              <Amount cents={t.amountCents} sign="always" size="sm" pending={t.pending} />
            </Link>
          ))}
          {n > 5 && (
            <button type="button" onClick={() => setAll((v) => !v)} className="mt-2 cursor-pointer self-start border-0 bg-transparent p-0 text-[14px] leading-5 font-semibold text-tinta hover:text-tinta-strong">
              {all ? "Show the largest" : `Show all ${n}`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
