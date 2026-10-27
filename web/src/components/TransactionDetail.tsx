import { useEffect, useState, type ReactNode } from "react";
import { ArrowLeftRight, ChevronLeft, ChevronRight, Clock, CreditCard, Search, X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryDecision } from "@/components/CategoryDecision";
import { CategoryTag } from "@/components/CategoryTag";
import { InstallmentProgress } from "@/components/InstallmentProgress";
import { Kbd } from "@/components/Kbd";
import { Sparkline } from "@/components/Sparkline";
import { NoteSection, SplitSection, TagsSection } from "@/components/TransactionNotes";
import { Button } from "@/components/ui/button";
import { send, typeLabel, useResource, type Change, type Pattern, type Transaction, type TransactionDetail as Detail, type Undo } from "@/lib/api";
import { useCategories } from "@/lib/categories";
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
        ) : d.split.length ? (
          <span className="type-caption text-ink-muted">Split, chosen by you. The budget counts each part in its category.</span>
        ) : (
          <CategorySection t={t} />
        )}
      </Section>

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
        {t.account.type === "cash" && <RemoveEntry id={t.id} />}
      </div>
    </>
  );
}

/** A wallet entry I added can be taken back; a synced transaction never. */
function RemoveEntry({ id }: { id: string }) {
  const [sure, setSure] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!sure)
    return (
      <Button variant="ghost" size="sm" onClick={() => setSure(true)}>
        Remove this entry
      </Button>
    );
  return (
    <span className="flex flex-wrap items-center gap-2 type-caption text-ink-muted">
      Its amount goes back to the wallet.
      <Button
        size="sm"
        onClick={() =>
          void send("DELETE", `/api/v1/transactions/${id}`).catch((e: Error) => {
            setError(e.message);
          })
        }
      >
        Remove
      </Button>
      {error && <span className="text-danger">Could not remove: {error}.</span>}
    </span>
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
  const [msg, setMsg] = useState<string | null>(null);
  const [undo, setUndo] = useState<Undo | null>(null);
  const current = t.category ? byId.get(t.category.id) : undefined;
  useEffect(() => setOpen(!t.category), [t.id, t.category]);
  // A choice's message and its Undo stay until another transaction opens,
  // even as the saved category comes back from the server.
  useEffect(() => {
    setMsg(null);
    setUndo(null);
  }, [t.id]);

  async function takeBack() {
    if (!undo) return;
    setBusy(true);
    try {
      await send("POST", "/api/v1/undo", undo);
      setUndo(null);
      setMsg("Undone.");
    } catch (e) {
      setMsg(`Could not undo: ${(e as Error).message}.`);
    } finally {
      setBusy(false);
    }
  }

  async function choose(category: string, teach: Pattern | null) {
    setBusy(true);
    setMsg(null);
    try {
      const r = await send<Change>("PUT", `/api/v1/transactions/${t.id}/category`, { category, teach });
      setOpen(false);
      setUndo(r.undo);
      setMsg(r.categorized ? `And ${r.categorized} more like it.` : "Saved.");
    } catch (e) {
      setMsg(`Could not save: ${(e as Error).message}.`);
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
      {msg && (
        <p className="m-0 flex flex-wrap items-center gap-x-2 type-caption text-ink-muted">
          {msg}
          {undo && (
            <Button variant="ghost" size="sm" onClick={() => void takeBack()} disabled={busy}>
              Undo
            </Button>
          )}
        </p>
      )}
      {open && <CategoryDecision categories={live} current={t.category?.id} suggested={suggestion.data?.pattern ?? null} onChoose={choose} busy={busy} />}
    </>
  );
}
