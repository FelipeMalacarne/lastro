import { useEffect, useState } from "react";
import { ChevronDown, Plus, X } from "lucide-react";

import { Amount } from "@/components/Amount";
import { CategoryTag } from "@/components/CategoryTag";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { send, useResource, type TagView, type Transaction } from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, inputBRL, parseBRL } from "@/lib/format";
import { toast } from "@/lib/toast";

type Part = { category: string; amount: string };

/**
 * A split across categories (canvas: "Split, note and tags"). The parts are
 * typed as positive amounts and take the transaction's sign when saved.
 */
export function SplitSection({ t, split }: { t: Transaction; split: { category: string; amountCents: number }[] }) {
  const { live, byId } = useCategories();
  const [editing, setEditing] = useState(false);
  const [parts, setParts] = useState<Part[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const total = Math.abs(t.amountCents);
  const sign = t.amountCents < 0 ? -1 : 1;

  useEffect(() => {
    setEditing(false);
    setError(null);
  }, [t.id]);

  function start() {
    if (split.length) setParts(split.map((p) => ({ category: p.category, amount: inputBRL(Math.abs(p.amountCents)) })));
    else setParts([{ category: t.category?.id ?? "", amount: inputBRL(total) }, { category: "", amount: "" }]);
    setError(null);
    setEditing(true);
  }

  const cents = parts.map((p) => parseBRL(p.amount));
  const left = total - cents.reduce<number>((s, c) => s + (c ?? 0), 0);
  const valid = parts.length >= 2 && parts.every((p, i) => p.category && cents[i]) && left === 0 && new Set(parts.map((p) => p.category)).size === parts.length;

  async function run(f: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await f();
      setEditing(false);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const set = (i: number, p: Partial<Part>) => setParts((ps) => ps.map((x, j) => (j === i ? { ...x, ...p } : x)));

  if (!editing) {
    if (!split.length)
      return (
        <Button size="sm" variant="ghost" className="self-start" onClick={start}>
          Split across categories
        </Button>
      );
    return (
      <>
        <div aria-hidden className="flex h-2.5 gap-0.5">
          {split.map((p) => {
            const c = byId.get(p.category);
            return <span key={p.category} className="rounded-xs" style={{ flex: `${Math.abs(p.amountCents)} 1 0`, background: c ? `var(--${c.color})` : "var(--rule-strong)" }} />;
          })}
        </div>
        <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
          {split.map((p) => {
            const c = byId.get(p.category);
            return (
              <li key={p.category} className="flex items-center justify-between gap-3">
                <CategoryTag name={c?.name} color={c?.color} />
                <Amount cents={p.amountCents} size="sm" />
              </li>
            );
          })}
        </ul>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="secondary" onClick={start}>
            Edit the split
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                const parts = split.map((p) => ({ category: p.category, amountCents: p.amountCents }));
                await send("DELETE", `/api/v1/transactions/${t.id}/split`);
                toast({ id: "split", text: `The split of “${t.description}” is gone.`, undo: () => send("PUT", `/api/v1/transactions/${t.id}/split`, { parts }) });
              })
            }
          >
            Remove the split
          </Button>
        </div>
        {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
      </>
    );
  }

  return (
    <form
      className="flex flex-col gap-2.5"
      onSubmit={(e) => {
        e.preventDefault();
        if (!valid) return;
        void run(() => send("PUT", `/api/v1/transactions/${t.id}/split`, { parts: parts.map((p, i) => ({ category: p.category, amountCents: sign * (cents[i] ?? 0) })) }));
      }}
    >
      {parts.map((p, i) => {
        const c = byId.get(p.category);
        return (
          <div key={i} className="grid grid-cols-[minmax(0,1fr)_120px_32px] items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" className="justify-between">
                  {c ? <CategoryTag name={c.name} color={c.color} /> : <span className="text-ink-muted">Category</span>}
                  <ChevronDown aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuRadioGroup value={p.category} onValueChange={(v) => set(i, { category: v })}>
                  {live.map((k) => (
                    <DropdownMenuRadioItem key={k.id} value={k.id} disabled={k.id !== p.category && parts.some((x) => x.category === k.id)}>
                      <CategoryTag name={k.name} color={k.color} />
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <label className="flex h-9 items-center gap-1 rounded-sm border border-rule-strong bg-paper-raised px-2.5 font-figure text-[14px] focus-within:focus-ring">
              <span className="text-ink-muted">R$</span>
              <input
                aria-label={`Amount for ${c?.name ?? `part ${i + 1}`}`}
                inputMode="decimal"
                value={p.amount}
                onChange={(e) => set(i, { amount: e.target.value })}
                className="w-full min-w-0 border-0 bg-transparent text-right text-ink outline-none"
              />
            </label>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={`Remove the ${c?.name ?? `part ${i + 1}`} part`}
              disabled={parts.length <= 2}
              onClick={() => setParts((ps) => ps.filter((_, j) => j !== i))}
            >
              <X aria-hidden />
            </Button>
          </div>
        );
      })}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={parts.length >= 10}
          onClick={() => setParts((ps) => [...ps, { category: "", amount: left > 0 ? inputBRL(left) : "" }])}
        >
          <Plus aria-hidden /> Add a part
        </Button>
        <span className={left === 0 ? "type-figure-sm text-ink-muted" : "type-figure-sm text-warning"}>
          {left === 0 ? "Adds up" : left > 0 ? `${formatBRL(left)} left to split` : `${formatBRL(-left)} too much`}
        </span>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" size="sm" disabled={!valid || busy}>
          Save the split
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
          Cancel
        </Button>
      </div>
      {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
      <p className="m-0 type-caption text-ink-muted">The parts must add up to the amount. Rules and the model never touch a split transaction.</p>
    </form>
  );
}

/** My note on a transaction, saved when the field is left. */
export function NoteSection({ t }: { t: Transaction }) {
  const [note, setNote] = useState(t.note);
  const [msg, setMsg] = useState<string | null>(null);
  useEffect(() => {
    setNote(t.note);
    setMsg(null);
  }, [t.id, t.note]);
  async function save() {
    if (note.trim() === t.note) return;
    try {
      await send("PUT", `/api/v1/transactions/${t.id}/note`, { note });
      setMsg("Saved.");
    } catch (e) {
      setMsg(`Could not save: ${(e as Error).message}.`);
    }
  }
  return (
    <>
      <textarea
        id={`note-${t.id}`}
        aria-label="Note"
        rows={2}
        maxLength={500}
        value={note}
        placeholder="Anything worth remembering: a warranty, who it was for…"
        onChange={(e) => {
          setNote(e.target.value);
          setMsg(null);
        }}
        onBlur={() => void save()}
        className="box-border w-full resize-y rounded-sm border border-rule-strong bg-paper-raised px-2.5 py-2 text-[15px] leading-[22px] text-ink placeholder:text-ink-muted focus-visible:focus-ring"
      />
      <span className="type-caption text-ink-muted">{msg ?? "Saved when you leave the field. Search finds notes too."}</span>
    </>
  );
}

/** Tags that cut across categories ("trip-rio"). */
export function TagsSection({ t }: { t: Transaction }) {
  const all = useResource<{ tags: TagView[] }>("/api/v1/tags").data?.tags ?? [];
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    setText("");
    setError(null);
  }, [t.id]);
  async function save(tags: string[]) {
    setError(null);
    try {
      await send("PUT", `/api/v1/transactions/${t.id}/tags`, { tags });
      setText("");
    } catch (e) {
      setError((e as Error).message);
    }
  }
  const add = (s: string) => {
    const tag = s.trim().replace(/^#/, "").toLowerCase().split(/\s+/).join("-");
    if (tag && !t.tags.includes(tag)) void save([...t.tags, tag]);
    else setText("");
  };
  const used = all.map((x) => x.tag).filter((x) => !t.tags.includes(x)).slice(0, 5);
  return (
    <>
      <div className="flex flex-wrap items-center gap-1.5">
        {t.tags.map((tag) => (
          <span key={tag} className="inline-flex h-7 items-center gap-0.5 rounded-sm border border-rule-strong pr-0.5 pl-2 font-figure text-[12px]">
            #{tag}
            <Button variant="ghost" size="sm" className="h-6 w-6 p-0 pointer-coarse:h-9 pointer-coarse:w-9" aria-label={`Remove the tag ${tag}`} onClick={() => void save(t.tags.filter((x) => x !== tag))}>
              <X aria-hidden />
            </Button>
          </span>
        ))}
        <input
          aria-label="Add a tag"
          placeholder="Add a tag"
          value={text}
          maxLength={33}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === ",") {
              e.preventDefault();
              add(text);
            }
          }}
          onBlur={() => text && add(text)}
          className="h-7 min-w-0 flex-[1_1_120px] rounded-sm border border-dashed border-rule-strong bg-transparent px-2 font-figure text-[12px] text-ink placeholder:text-ink-muted focus-visible:focus-ring"
        />
      </div>
      {used.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 type-caption text-ink-muted">
          <span>Used before:</span>
          {used.map((x) => (
            <button key={x} type="button" onClick={() => add(x)} className="cursor-pointer border-0 bg-transparent p-0 font-figure text-[12px] text-tinta hover:text-tinta-strong focus-visible:focus-ring">
              #{x}
            </button>
          ))}
        </div>
      )}
      {error && <span className="type-caption text-danger">Could not save: {error}.</span>}
    </>
  );
}
