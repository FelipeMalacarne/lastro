import { useEffect, useState, type ReactNode } from "react";
import { ArrowLeftRight, ChevronLeft, ChevronRight, Clock, CreditCard, Search, TriangleAlert, X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryDecision } from "@/components/CategoryDecision";
import { CategoryTag } from "@/components/CategoryTag";
import { InstallmentProgress } from "@/components/InstallmentProgress";
import { Kbd } from "@/components/Kbd";
import { Sparkline } from "@/components/Sparkline";
import { NoteSection, SplitSection, TagsSection } from "@/components/TransactionNotes";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { send, typeLabel, useResource, type Change, type Pattern, type Transaction, type TransactionDetail as Detail } from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, formatDate, formatDay, formatMonth, formatWeekday } from "@/lib/format";
import { Link, hrefWith, navigate } from "@/lib/router";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

type Props = {
  id: string;
  onClose: () => void;
  /** The newer and older transactions in the list, when there are. */
  prev?: string;
  next?: string;
  /** In a phone sheet, whose head already has the arrows and ✕. */
  inSheet?: boolean;
};

const accountName = (t: Pick<Transaction, "account">) => `${t.account.institution} ${typeLabel[t.account.type]}`;

/** Newer and older: the arrows that walk the list without closing the detail. */
export function DetailSteps({ prev, next, size = "sm" }: { prev?: string; next?: string; size?: "sm" | "icon" }) {
  const step = size === "icon" ? "size-11 text-ink [&_svg:not([class*='size-'])]:size-5" : undefined;
  return (
    <>
      <Button asChild variant="ghost" size={size} aria-label="Newer transaction" className={cn(step, !prev && "pointer-events-none opacity-40")}>
        <Link to={prev ? hrefWith({ tx: prev }) : hrefWith({})} aria-disabled={!prev}>
          <ChevronLeft aria-hidden />
        </Link>
      </Button>
      <Button asChild variant="ghost" size={size} aria-label="Older transaction" className={cn(step, !next && "pointer-events-none opacity-40")}>
        <Link to={next ? hrefWith({ tx: next }) : hrefWith({})} aria-disabled={!next}>
          <ChevronRight aria-hidden />
        </Link>
      </Button>
    </>
  );
}

/** One transaction in full (canvas: the detail beside the list, and the phone sheet). */
export function TransactionDetail({ id, onClose, prev, next, inSheet }: Props) {
  const res = useResource<Detail>(`/api/v1/transactions/${id}`);
  const d = res.data;
  return (
    <div className="flex flex-col gap-5">
      {!inSheet && (
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-1">
            <DetailSteps prev={prev} next={next} />
            <span className="ml-1 inline-flex items-center gap-1 type-caption text-ink-muted max-lg:hidden">
              <Kbd>↑</Kbd>
              <Kbd>↓</Kbd> to move, <Kbd>Esc</Kbd> to close
            </span>
          </div>
          <Button variant="ghost" size="sm" aria-label="Close" onClick={onClose}>
            <X aria-hidden />
          </Button>
        </div>
      )}

      {res.error && !d && <p className="m-0 type-body text-ink-muted">Could not load the transaction: {res.error}.</p>}
      {!d && !res.error && (
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <Skeleton className="w-2/3" />
            <Skeleton className="h-8 w-36" />
          </div>
          <Skeleton className="h-24" />
          <Skeleton className="h-16" />
        </div>
      )}
      {d && <Body d={d} onClose={onClose} />}
    </div>
  );
}

function Body({ d, onClose }: { d: Detail; onClose: () => void }) {
  const t = d.transaction;
  const neutral = t.kind === "transfer" || t.kind === "bill_payment";
  const facts: [string, string][] = [
    ["Date", `${formatWeekday(t.date)} ${t.date.slice(0, 4)}`],
    ["Account", t.manual && t.account.type !== "cash" ? `${accountName(t)}, added by you` : accountName(t)],
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

      {d.duplicates.length > 0 && <SameTransaction t={t} others={d.duplicates} />}

      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-[14px] leading-5">
        {facts.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-ink-muted">{k}</dt>
            <dd className="m-0 text-right font-semibold [overflow-wrap:anywhere]">{v}</dd>
          </div>
        ))}
      </dl>

      <Section title="Category">
        {t.markedTransfer ? (
          <TransferMark t={t} />
        ) : neutral ? (
          <p className="m-0 text-[14px] leading-5 text-ink-muted">
            {t.kind === "bill_payment"
              ? "Card bills need no category: the purchases on the card already have theirs."
              : "Money moving between your accounts, or to and from your own investments, needs no category."}
          </p>
        ) : d.split.length ? (
          <span className="type-caption text-ink-muted">Split, chosen by you. The budget counts each part in its category.</span>
        ) : (
          <>
            <CategorySection t={t} />
            {!t.counterpartId && <TransferMark t={t} />}
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
                ["Still to pay", p.remainingCount ? `${formatBRL(p.remainingCents)} in ${p.remainingCount} ${p.remainingCount === 1 ? "installment" : "installments"}` : "Nothing"],
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

      {!neutral && (
        <Section title={d.split.length ? `Split in ${d.split.length}` : "Split"}>
          <SplitSection t={t} split={d.split} />
        </Section>
      )}

      <Section title="Note">
        <NoteSection t={t} />
      </Section>

      <Section title="Tags">
        <TagsSection t={t} />
      </Section>

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
        {t.manual && <RemoveEntry t={t} onRemoved={onClose} />}
      </div>
    </>
  );
}

/** "A transfer between my accounts", for money to or from one lastro does not have. */
function TransferMark({ t }: { t: Transaction }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function set(on: boolean) {
    setBusy(true);
    setError(null);
    try {
      await send("PUT", "/api/v1/transactions/transfer", { ids: [t.id], transfer: on });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="flex flex-col items-start gap-1.5">
      {t.markedTransfer && (
        <p className="m-0 text-[14px] leading-5 text-ink-muted">Marked by you as a transfer to or from an account of yours that lastro does not have. It counts neither as spending nor as income.</p>
      )}
      <Button variant="ghost" size="sm" disabled={busy} onClick={() => void set(!t.markedTransfer)}>
        <ArrowLeftRight aria-hidden />
        {t.markedTransfer ? "Not a transfer: count it again" : "A transfer between my accounts"}
      </Button>
      {error && <span className="type-caption text-danger">Could not save: {error}.</span>}
    </div>
  );
}

/**
 * A transaction the bank may have sent too, after I added it (canvas: "A
 * transaction the bank never sent"). lastro never merges them on its own.
 */
function SameTransaction({ t, others }: { t: Transaction; others: Transaction[] }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function answer(o: Transaction, same: boolean) {
    const [manual, synced] = t.manual ? [t, o] : [o, t];
    setBusy(true);
    setError(null);
    try {
      await send("POST", `/api/v1/transactions/${manual.id}/duplicate`, { of: synced.id, same });
      // Merged, mine is gone: show the one that stays.
      if (same && t.manual) navigate(hrefWith({ tx: synced.id, day: null }), { replace: true });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="flex flex-col gap-3 rounded-md bg-warning-soft p-4">
      <span className="inline-flex items-center gap-2 type-body-strong">
        <TriangleAlert className="size-4 flex-none text-warning" aria-hidden />
        Is this the same transaction?
      </span>
      {others.map((o) => (
        <div key={o.id} className="flex flex-col gap-2">
          <Link
            to={hrefWith({ tx: o.id, day: null })}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-0.5 rounded-sm border border-rule bg-paper-raised px-3 py-2 text-ink no-underline hover:bg-paper-sunken hover:text-ink focus-visible:focus-ring"
          >
            <span className="type-body-strong">{o.description}</span>
            <Amount cents={o.amountCents} sign="always" />
            <span className="col-span-2 type-caption text-ink-muted">
              {formatDay(o.date)} · {o.manual ? "added by you" : `sent by ${o.account.institution}`}, {apart(t.date, o.date)}
            </span>
          </Link>
          <div className="flex flex-wrap gap-2">
            <Button variant="primary" size="sm" disabled={busy} onClick={() => void answer(o, true)}>
              Same transaction
            </Button>
            <Button size="sm" disabled={busy} onClick={() => void answer(o, false)}>
              Different
            </Button>
          </div>
        </div>
      ))}
      <span className="type-caption text-ink-muted">
        Same: yours is removed and the bank&apos;s keeps your category, note and tags. Different: both stay and lastro stops asking about this pair.
      </span>
      {error && <span className="type-caption text-danger">Could not save: {error}.</span>}
    </div>
  );
}

/** "2 days after yours", from the open transaction's point of view. */
function apart(mine: string, other: string): string {
  const days = Math.round((Date.parse(other) - Date.parse(mine)) / 86_400_000);
  if (days === 0) return "the same day";
  const n = Math.abs(days);
  return `${n} ${n === 1 ? "day" : "days"} ${days > 0 ? "after" : "before"} this one`;
}

/** A transaction I added can be taken back; a synced transaction never. Its note and tags go with it, so it asks first. */
function RemoveEntry({ t, onRemoved }: { t: Transaction; onRemoved: () => void }) {
  const wallet = t.account.type === "cash";
  return (
    <ConfirmDialog
      trigger={
        <Button variant="ghost" size="sm">
          {wallet ? "Remove this entry" : "Remove this transaction"}
        </Button>
      }
      title={wallet ? "Remove this entry?" : "Remove this transaction?"}
      confirmLabel={wallet ? "Remove entry" : "Remove transaction"}
      onConfirm={async () => {
        await send("DELETE", `/api/v1/transactions/${t.id}`);
        onRemoved();
        toast({ text: <><strong>{t.description}</strong> removed.</> });
      }}
    >
      <p>
        “{t.description}”, {formatBRL(t.amountCents, { sign: "always" })} on {formatDay(t.date)}, goes with its note and tags.
      </p>
      <p className="type-caption text-ink-muted">{wallet ? "Its amount goes back to the wallet." : "The bank's balance does not change."}</p>
    </ConfirmDialog>
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

const sourceLabel: Record<string, string> = { rule: "by a rule", llm: "suggested by the local model", manual: "chosen by you" };

/** The category, who chose it, and the choice that changes it (and teaches a rule). */
function CategorySection({ t }: { t: Transaction }) {
  const { live, byId } = useCategories();
  const suggestion = useResource<{ pattern: Pattern | null }>(`/api/v1/patterns/suggest?description=${encodeURIComponent(t.description)}`);
  const [open, setOpen] = useState(!t.category);
  const [busy, setBusy] = useState(false);
  const current = t.category ? byId.get(t.category.id) : undefined;
  useEffect(() => setOpen(!t.category), [t.id, t.category]);

  async function choose(category: string, teach: Pattern | null) {
    setBusy(true);
    try {
      const r = await send<Change>("PUT", `/api/v1/transactions/${t.id}/category`, { category, teach });
      setOpen(false);
      const name = byId.get(category)?.name ?? "the category";
      toast({
        id: "category",
        text: (
          <>
            Now in <strong>{name}</strong>
            {r.categorized ? `, with ${r.categorized} more like it.` : "."}
          </>
        ),
        undo: () => send("POST", "/api/v1/undo", r.undo),
      });
    } catch (e) {
      toast({ id: "category", tone: "error", text: `Could not save: ${(e as Error).message}.` });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex flex-col gap-0.5">
          <CategoryTag name={current?.name} color={current?.color} />
          {t.category && <span className="type-caption text-ink-muted">{sourceLabel[t.category.source] ?? ""}</span>}
        </span>
        {t.category && (
          <Button size="sm" variant={open ? "ghost" : "secondary"} onClick={() => setOpen((o) => !o)}>
            {open ? "Cancel" : "Change"}
          </Button>
        )}
      </div>
      {open && <CategoryDecision categories={live} current={t.category?.id} suggested={suggestion.data?.pattern ?? null} onChoose={choose} busy={busy} />}
    </>
  );
}
