import type { ReactNode } from "react";
import { ArrowLeftRight, ChevronLeft, ChevronRight, Clock, CreditCard, Search, X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { InstallmentProgress } from "@/components/InstallmentProgress";
import { Kbd } from "@/components/Kbd";
import { Sparkline } from "@/components/Sparkline";
import { Button } from "@/components/ui/button";
import { typeLabel, useResource, type Transaction, type TransactionDetail as Detail } from "@/lib/api";
import { formatBRL, formatDate, formatDay, formatMonth, formatWeekday } from "@/lib/format";
import { Link, hrefWith } from "@/lib/router";

type Props = {
  id: string;
  onClose: () => void;
  /** The newer and older transactions in the list, when there are. */
  prev?: string;
  next?: string;
  /** A phone sheet has no room for the keyboard hint. */
  compact?: boolean;
};

const accountName = (t: Pick<Transaction, "account">) => `${t.account.institution} ${typeLabel[t.account.type]}`;

/** One transaction in full (canvas: the detail beside the list, and the phone sheet). */
export function TransactionDetail({ id, onClose, prev, next, compact }: Props) {
  const res = useResource<Detail>(`/api/v1/transactions/${id}`);
  const d = res.data;
  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1">
          <Button asChild variant="ghost" size="sm" aria-label="Newer transaction" className={prev ? undefined : "pointer-events-none opacity-40"}>
            <Link to={prev ? hrefWith({ tx: prev }) : hrefWith({})} aria-disabled={!prev}>
              <ChevronLeft aria-hidden />
            </Link>
          </Button>
          <Button asChild variant="ghost" size="sm" aria-label="Older transaction" className={next ? undefined : "pointer-events-none opacity-40"}>
            <Link to={next ? hrefWith({ tx: next }) : hrefWith({})} aria-disabled={!next}>
              <ChevronRight aria-hidden />
            </Link>
          </Button>
          {!compact && (
            <span className="ml-1 inline-flex items-center gap-1 type-caption text-ink-muted max-lg:hidden">
              <Kbd>↑</Kbd>
              <Kbd>↓</Kbd> to move, <Kbd>Esc</Kbd> to close
            </span>
          )}
        </div>
        <Button variant="ghost" size="sm" aria-label="Close" onClick={onClose}>
          <X aria-hidden />
        </Button>
      </div>

      {res.error && !d && <p className="m-0 type-body text-ink-muted">Could not load the transaction: {res.error}.</p>}
      {!d && !res.error && <p className="m-0 type-body text-ink-muted">Loading…</p>}
      {d && <Body d={d} />}
    </div>
  );
}

function Body({ d }: { d: Detail }) {
  const t = d.transaction;
  const neutral = t.kind === "transfer" || t.kind === "bill_payment";
  const facts: [string, string][] = [
    ["Date", `${formatWeekday(t.date)} ${t.date.slice(0, 4)}`],
    ["Account", accountName(t)],
    ["Status", t.status === "pending" ? "Pending, may still change" : "Posted"],
    [
      "Counts as",
      t.kind === "bill_payment"
        ? "Card bill, not spending"
        : t.kind === "transfer"
          ? "Transfer, not spending"
          : t.kind === "income"
            ? "Income"
            : t.amountCents > 0
              ? "Refund, lowers spending"
              : "Spending",
    ],
  ];
  const p = d.purchase;
  const other = d.counterpartTransaction;
  const months = d.history.months;
  return (
    <>
      <div className="flex flex-col gap-1.5">
        <h2 className="m-0 type-heading">{t.description}</h2>
        <Amount cents={t.amountCents} size="lg" sign={neutral ? "none" : "always"} tone={neutral ? "muted" : "auto"} pending={t.status === "pending"} />
        <div className="flex flex-wrap gap-1.5">
          {t.status === "pending" && (
            <Badge dashed icon={Clock}>
              Pending
            </Badge>
          )}
          {t.kind === "transfer" && <Badge icon={ArrowLeftRight}>Transfer</Badge>}
          {t.kind === "bill_payment" && <Badge icon={CreditCard}>Card bill</Badge>}
          {t.installment && (
            <Badge figure>
              {t.installment.number} of {t.installment.total}
            </Badge>
          )}
        </div>
      </div>

      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-[14px] leading-5">
        {facts.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-ink-muted">{k}</dt>
            <dd className="m-0 text-right font-semibold [overflow-wrap:anywhere]">{v}</dd>
          </div>
        ))}
      </dl>

      <Section title="Category">
        {neutral ? (
          <p className="m-0 text-[14px] leading-5 text-ink-muted">
            {t.kind === "bill_payment"
              ? "Card bills need no category: the purchases on the card already have theirs."
              : "Money moving between your accounts, or to and from your own investments, needs no category."}
          </p>
        ) : (
          <>
            <span className="text-[14px] leading-5 text-ink-muted">Uncategorized</span>
            <p className="m-0 type-caption text-ink-muted">Choosing one will teach a rule for every “{t.description}”. It arrives with the review queue.</p>
          </>
        )}
      </Section>

      {p && (
        <Section title={`Installment ${t.installment?.number ?? p.current} of ${p.total}`}>
          <InstallmentProgress current={t.installment?.number ?? p.current} total={p.total} className="[&>span]:h-2.5" />
          <div className="flex justify-between type-figure-sm text-ink-muted">
            <span>{formatMonth(p.firstDate.slice(0, 7), true)}</span>
            <span>{formatMonth(p.lastDate.slice(0, 7), true)}</span>
          </div>
          <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-[14px] leading-5">
            {(
              [
                ["Per month", formatBRL(p.amountCents)],
                ["In all", formatBRL(p.amountCents * p.total)],
                ["Still to pay", p.remainingCount ? `${formatBRL(p.remainingCents)} in ${p.remainingCount}` : "Nothing"],
                ["Last installment", formatDate(p.lastDate)],
              ] as [string, string][]
            ).map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-ink-muted">{k}</dt>
                <dd className="m-0 text-right font-semibold">{v}</dd>
              </div>
            ))}
          </dl>
          <Link to={`/accounts/${t.account.id}`} className="text-[14px] leading-5 font-semibold">
            Every installment on the card
          </Link>
        </Section>
      )}

      {other && (
        <Section title="Paired with">
          <Link
            to={hrefWith({ tx: other.id, day: null })}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-0.5 rounded-md border border-rule-strong px-3 py-2.5 text-ink no-underline hover:bg-paper-sunken hover:text-ink focus-visible:focus-ring"
          >
            <span className="type-body-strong">{other.description}</span>
            <Amount cents={other.amountCents} sign="none" tone="muted" />
            <span className="type-caption text-ink-muted">
              {accountName(other)} · {formatDay(other.date)}
            </span>
          </Link>
          <p className="m-0 type-caption text-ink-muted">
            Paired because the amounts match{other.date === t.date ? " and both are on the same day" : ", a few days apart"}. Neither side counts as spending.
          </p>
        </Section>
      )}

      {!neutral && d.history.count > 1 && (
        <Section title="The last 6 months">
          <div className="flex items-center justify-between gap-4">
            <span className="text-[14px] leading-5">
              “{t.description}” {d.history.count} times, {formatBRL(Math.abs(d.history.totalCents))} in all
            </span>
            <Sparkline values={months.map((m) => Math.abs(m.cents))} label={`By month: ${months.map((m) => `${formatMonth(m.month, true)} ${formatBRL(Math.abs(m.cents))}`).join(", ")}`} />
          </div>
        </Section>
      )}

      <div className="flex flex-wrap gap-2 border-t border-rule pt-4">
        <Button asChild size="sm">
          <Link to={hrefWith({ q: t.description, day: null })}>
            <Search aria-hidden />
            All “{t.description.length > 28 ? t.description.slice(0, 27) + "…" : t.description}”
          </Link>
        </Button>
        <Button asChild variant="ghost" size="sm">
          <Link to={`/accounts/${t.account.id}`}>Open {accountName(t)}</Link>
        </Button>
      </div>
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2.5 border-t border-rule pt-4">
      <span className="type-label text-ink-muted">{title}</span>
      {children}
    </div>
  );
}
