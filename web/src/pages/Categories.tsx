import { useMemo, useState, type ReactNode } from "react";
import { ChevronDown, ChevronLeft, Plus, Search } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  send,
  typeLabel,
  useResource,
  type Account,
  type Category,
  type CategoryTotal,
  type NoteColor,
  type PatternKind,
  type Rule,
  type TransactionList,
} from "@/lib/api";
import { useCategories, useWaiting } from "@/lib/categories";
import { formatDay, formatMonth } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { Link } from "@/lib/router";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";

const notes: { color: NoteColor; label: string; short: string }[] = [
  { color: "note-100", label: "R$ 100 note, blue", short: "100" },
  { color: "note-50", label: "R$ 50 note, ochre", short: "50" },
  { color: "note-20", label: "R$ 20 note, gold", short: "20" },
  { color: "note-10", label: "R$ 10 note, carmine", short: "10" },
  { color: "note-5", label: "R$ 5 note, violet", short: "5" },
  { color: "note-2", label: "R$ 2 note, blue-grey", short: "2" },
  { color: "note-1", label: "R$ 1 note, green", short: "1" },
  { color: "note-200", label: "R$ 200 note, sepia", short: "200" },
];

const kinds: { value: PatternKind; label: string }[] = [
  { value: "contains", label: "Contains" },
  { value: "exact", label: "Is exactly" },
];

/** Categories and rules (canvas: "Categories and rules (M2)" and "Edit a rule, phone"). */
export function Categories() {
  const { month } = useMonth();
  const cats = useCategories();
  const rules = useResource<{ rules: Rule[] }>("/api/v1/rules");
  const totals = useResource<{ categories: CategoryTotal[] }>(`/api/v1/transactions/categories?month=${month}`);
  const accounts = useResource<{ accounts: Account[] }>("/api/v1/accounts");
  const list = useResource<TransactionList>(`/api/v1/transactions?month=${month}`);
  const waiting = useWaiting();
  const [creating, setCreating] = useState(false);

  const ruleList = rules.data?.rules ?? [];
  const rulesByCategory = useMemo(() => {
    const m = new Map<string, number>();
    for (const r of ruleList) m.set(r.category, (m.get(r.category) ?? 0) + 1);
    return m;
  }, [ruleList]);

  // Who chose this month's categories: rules, the model, me, or nobody yet.
  const flow = useMemo(() => {
    const f = { rule: 0, llm: 0, manual: 0, none: 0 };
    for (const t of list.data?.transactions ?? []) {
      if (t.kind === "transfer" || t.kind === "bill_payment") continue;
      const src = t.category?.source as keyof typeof f | undefined;
      f[src && src in f ? src : "none"]++;
    }
    return f;
  }, [list.data]);

  const caption = `${cats.live.length} categories · ${ruleList.length} ${ruleList.length === 1 ? "rule" : "rules"}${waiting ? ` · ${waiting} to review` : ""}`;

  return (
    <>
      <div className="flex flex-col gap-1">
        <Link to="/transactions" className="inline-flex items-center gap-1 self-start text-[13px] leading-[18px] font-semibold no-underline">
          <ChevronLeft className="size-3.5" aria-hidden />
          Transactions
        </Link>
        <PageHeader
          title="Categories and rules"
          caption={caption}
         
          actions={
            <Button onClick={() => setCreating(true)}>
              <Plus aria-hidden />
              New category
            </Button>
          }
        />
      </div>

      <FlowPanel flow={flow} month={month} waiting={waiting ?? 0} />

      <RulesPanel rules={ruleList} loading={!rules.data} error={rules.error} categories={cats.live} byId={cats.byId} accounts={accounts.data?.accounts ?? []} />

      <CategoriesPanel
        categories={cats.live}
        rulesByCategory={rulesByCategory}
        totals={totals.data?.categories ?? []}
        month={month}
        creating={creating}
        onCreated={() => setCreating(false)}
        onCreate={() => setCreating(true)}
      />
    </>
  );
}

function FlowPanel({ flow, month, waiting }: { flow: { rule: number; llm: number; manual: number; none: number }; month: string; waiting: number }) {
  const total = flow.rule + flow.llm + flow.manual + flow.none;
  const steps: { key: string; title: string; n: number; text: ReactNode; swatch: string }[] = [
    { key: "rule", title: "1. Your rules", n: flow.rule, text: "Tried first, from the most specific to the most generic. The first rule that matches wins.", swatch: "bg-ink" },
    { key: "llm", title: "2. The local model", n: flow.llm, text: "Only when no rule matches, and only with a category that exists. From 80% sure.", swatch: "bg-tinta" },
    { key: "manual", title: "3. Your own choices", n: flow.manual, text: "Never overwritten by automation, and each one taught a rule.", swatch: "bg-note-2" },
    {
      key: "none",
      title: "4. To review",
      n: flow.none,
      text: (
        <>
          When nothing is sure. Nothing gets a guessed category.{" "}
          {waiting > 0 && (
            <Link to="/review" className="font-semibold">
              Review all {waiting}
            </Link>
          )}
        </>
      ),
      swatch: "border border-dashed border-warning bg-warning-soft",
    },
  ];
  return (
    <Panel
      title={`How ${formatMonth(month).split(" ")[0]}'s transactions got their category`}
      footer="A category you pick by hand always becomes a rule and is never overwritten by automation."
    >
      <div className="flex flex-col gap-4">
        {total > 0 && (
          <div role="img" aria-label={steps.map((s) => `${s.n} ${s.title.slice(3).toLowerCase()}`).join(", ")} className="flex h-3 gap-0.5">
            {steps
              .filter((s) => s.n > 0)
              .map((s) => (
                <div key={s.key} className={cn("min-w-2 rounded-xs", s.swatch)} style={{ flex: `${s.n} 1 0` }} />
              ))}
          </div>
        )}
        <ol className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-x-8 gap-y-4 p-0">
          {steps.map((s) => (
            <li key={s.key} className="flex flex-col gap-1">
              <span className="flex items-center gap-2 type-body-strong">
                <span aria-hidden className={cn("size-3 flex-none rounded-xs", s.swatch)} />
                <span className="flex-1">{s.title}</span>
                <span className="type-figure">{s.n}</span>
              </span>
              <span className="type-caption text-ink-muted">{s.text}</span>
            </li>
          ))}
        </ol>
      </div>
    </Panel>
  );
}

function RulesPanel({
  rules,
  loading,
  error,
  categories,
  byId,
  accounts,
}: {
  rules: Rule[];
  loading: boolean;
  error?: string;
  categories: Category[];
  byId: Map<string, Category>;
  accounts: Account[];
}) {
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const shown = rules.filter((r) => {
    if (!q) return true;
    const s = q.toLowerCase();
    return r.pattern.text.toLowerCase().includes(s) || (byId.get(r.category)?.name.toLowerCase().includes(s) ?? false);
  });
  const rule = rules.find((r) => r.id === selected) ?? null;
  const accountName = (id: string | null) => {
    const a = accounts.find((x) => x.id === id);
    return a ? `${a.institution} ${typeLabel[a.type]}` : null;
  };
  return (
    <Panel title={`Rules · ${rules.length}`} bodyClassName="-mx-5 -mb-5 border-t border-rule">
      <div className="flex flex-wrap">
        <div className="flex min-w-0 flex-[1_1_520px] flex-col">
          <div className="px-5 py-4">
            <TextField
              aria-label="Search rules"
              placeholder="Text or category, e.g. UBER"
              prefix={<Search className="size-4" aria-hidden />}
              value={q}
              onChange={(e) => setQ(e.target.value)}
              type="search"
            />
          </div>
          {error && <p className="m-0 px-5 pb-4 type-body text-ink-muted">Could not load the rules: {error}.</p>}
          {loading && !error && <p className="m-0 px-5 pb-4 type-body text-ink-muted">Loading the rules…</p>}
          {!loading && rules.length === 0 && (
            <p className="m-0 px-5 pb-5 type-body text-ink-muted">
              No rules yet. Each choice in{" "}
              <Link to="/review" className="font-semibold">
                To review
              </Link>{" "}
              teaches one.
            </p>
          )}
          {shown.length > 0 && (
            <div>
              <div role="table" aria-label="Rules, in the order they are tried">
                <div
                  role="row"
                  className="grid h-8 grid-cols-[minmax(0,1fr)_auto_auto] sm:grid-cols-[28px_minmax(0,1fr)_140px_96px] items-center gap-x-3 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted"
                >
                  <span role="columnheader" className="hidden sm:block">
                    #
                  </span>
                  <span role="columnheader">When the description</span>
                  <span role="columnheader">Then</span>
                  <span role="columnheader" className="text-right">
                    Decides
                  </span>
                </div>
                {shown.map((r) => {
                  const c = byId.get(r.category);
                  const on = r.id === selected;
                  return (
                    <button
                      key={r.id}
                      type="button"
                      role="row"
                      aria-selected={on}
                      onClick={() => setSelected(on ? null : r.id)}
                      className={cn(
                        "grid min-h-14 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto_auto] sm:grid-cols-[28px_minmax(0,1fr)_140px_96px] items-center gap-x-3 border-0 border-b border-rule px-5 py-2 text-left focus-visible:focus-ring",
                        on ? "bg-tinta-soft shadow-[inset_3px_0_0_var(--tinta)]" : "bg-paper-raised hover:bg-paper-sunken",
                      )}
                    >
                      <span role="cell" className="hidden type-figure-sm text-ink-muted sm:block">
                        {r.order}
                      </span>
                      <span role="cell" className="flex min-w-0 flex-col gap-0.5">
                        <span className="type-caption text-ink-muted">{r.pattern.kind === "exact" ? "is exactly" : "contains"}</span>
                        <span className="flex flex-wrap items-center gap-2">
                          <span className="font-figure text-[14px] leading-5 font-medium [overflow-wrap:anywhere]">{r.pattern.text}</span>
                          {r.account && <Badge>{accountName(r.account) ?? "one account"}</Badge>}
                        </span>
                      </span>
                      <span role="cell" className="flex min-w-0">
                        <CategoryTag name={c?.name ?? "Archived"} color={c?.color} />
                      </span>
                      <span role="cell" className="flex flex-col items-end gap-0.5">
                        <span className="type-figure">{r.decides}</span>
                        <span className="type-caption text-ink-muted">{r.last ? `last ${formatDay(r.last)}` : "none yet"}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
          <p className="m-0 px-5 py-3.5 type-caption text-ink-muted">
            In the order they are tried: a rule tied to an account first, then exact before contains, then more words, then more letters, then the latest
            correction. Decides counts what each rule settles; a rule it shadows may meet more.
          </p>
        </div>

        {rule && (
          <RuleEditor key={rule.id} rule={rule} rules={rules} categories={categories} accounts={accounts} onClose={() => setSelected(null)} />
        )}
      </div>
    </Panel>
  );
}

function RuleEditor({
  rule,
  rules,
  categories,
  accounts,
  onClose,
}: {
  rule: Rule;
  rules: Rule[];
  categories: Category[];
  accounts: Account[];
  onClose: () => void;
}) {
  const [kind, setKind] = useState<PatternKind>(rule.pattern.kind);
  const [text, setText] = useState(rule.pattern.text);
  const [account, setAccount] = useState(rule.account ?? "");
  const [category, setCategory] = useState(rule.category);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const before = rules.find((r) => r.order > rule.order && rule.pattern.text.includes(r.pattern.text) && r.id !== rule.id);
  const current = accounts.find((a) => a.id === account);

  async function save() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await send<{ categorized: number }>("PATCH", `/api/v1/rules/${rule.id}`, { pattern: { kind, text }, account, category });
      setMsg(r.categorized ? `Saved. ${r.categorized} transactions moved.` : "Saved.");
    } catch (e) {
      setMsg(`Could not save: ${(e as Error).message}.`);
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    setBusy(true);
    try {
      await send("DELETE", `/api/v1/rules/${rule.id}`);
      onClose();
    } catch (e) {
      setMsg(`Could not delete: ${(e as Error).message}.`);
      setBusy(false);
    }
  }

  return (
    <section
      aria-label={`Edit rule ${rule.order}`}
      className="flex max-w-full min-w-0 flex-[1_1_360px] flex-col gap-5 border-rule bg-paper px-5 pt-5 pb-6 max-[880px]:border-t min-[881px]:border-l lg:px-6"
    >
      <div className="flex flex-col gap-1">
        <span className="type-figure-sm text-ink-muted">
          Rule {rule.order} · taught {formatDay(rule.createdAt.slice(0, 10))} · decides {rule.decides}, meets {rule.meets}
        </span>
        <h3 className="m-0 type-heading">Edit rule</h3>
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-[13px] leading-[18px] font-semibold">Match</span>
        <SegmentedControl label="Match" options={kinds} value={kind} onChange={setKind} className="self-start" />
      </div>
      <TextField label="Description text" value={text} onChange={(e) => setText(e.target.value)} hint="Case, accents and symbols are ignored." />
      <div className="flex flex-col gap-1.5">
        <span className="text-[13px] leading-[18px] font-semibold">Account</span>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button className="self-start">
              {current ? `${current.institution} ${typeLabel[current.type]}` : "Any account"}
              <ChevronDown aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuRadioGroup value={account} onValueChange={setAccount}>
              <DropdownMenuRadioItem value="">Any account</DropdownMenuRadioItem>
              {accounts.map((a) => (
                <DropdownMenuRadioItem key={a.id} value={a.id}>
                  {a.institution} {typeLabel[a.type]}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="flex flex-col gap-2">
        <span className="text-[13px] leading-[18px] font-semibold">Then categorize as</span>
        <div role="radiogroup" aria-label="Category" className="grid grid-cols-2 gap-1.5">
          {categories.map((c) => (
            <button
              key={c.id}
              type="button"
              role="radio"
              aria-checked={c.id === category}
              onClick={() => setCategory(c.id)}
              className={cn(
                "flex h-11 cursor-pointer items-center rounded-sm border px-2.5 focus-visible:focus-ring lg:h-9",
                c.id === category ? "border-tinta bg-tinta-soft" : "border-rule bg-paper-raised hover:bg-paper-sunken",
              )}
            >
              <CategoryTag name={c.name} color={c.color} />
            </button>
          ))}
        </div>
      </div>
      {before && (
        <p className="m-0 type-caption text-ink-muted">
          Tried before rule {before.order}, <strong className="font-semibold text-ink">contains {before.pattern.text}</strong>, because it is more specific.
        </p>
      )}
      {msg && (
        <p role="status" className="m-0 type-caption text-ink-muted">
          {msg}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2 border-t border-rule pt-4">
        <Button variant="primary" onClick={save} disabled={busy}>
          Save rule
        </Button>
        <Button variant="ghost" onClick={onClose}>
          Close
        </Button>
        <Button variant="ghost" className="ml-auto" onClick={remove} disabled={busy}>
          Delete rule
        </Button>
      </div>
      <p className="m-0 type-caption text-ink-muted">
        Saving runs the rules again over what automation categorized; your own choices never move. Deleting keeps the categories it gave.
      </p>
    </section>
  );
}

function CategoriesPanel({
  categories,
  rulesByCategory,
  totals,
  month,
  creating,
  onCreated,
  onCreate,
}: {
  categories: Category[];
  rulesByCategory: Map<string, number>;
  totals: CategoryTotal[];
  month: string;
  creating: boolean;
  onCreated: () => void;
  onCreate: () => void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const out = new Map(totals.map((t) => [t.category, t]));
  const monthName = formatMonth(month).split(" ")[0];
  return (
    <Panel
      title={`Categories · ${categories.length}`}
      action={
        <Button size="sm" variant="ghost" onClick={onCreate}>
          <Plus aria-hidden />
          New
        </Button>
      }
      bodyClassName="-mx-5 border-t border-rule"
      footer="Colours come from the Real notes and stay with the category, never assigned by chart order. A category is archived, never deleted: its transactions keep it."
    >
      <div>
        <div role="table" aria-label="Categories">
          <div
            role="row"
            className="grid h-8 grid-cols-[minmax(0,1fr)_auto_auto] sm:grid-cols-[minmax(0,1fr)_120px_60px_140px_72px] items-center gap-x-4 border-b border-rule bg-paper-sunken px-5 type-label text-ink-muted"
          >
            <span role="columnheader">Category</span>
            <span role="columnheader" className="hidden sm:block">
              Budget
            </span>
            <span role="columnheader" className="hidden text-right sm:block">
              Rules
            </span>
            <span role="columnheader" className="text-right">
              {monthName}
            </span>
            <span role="columnheader">
              <span className="sr-only">Actions</span>
            </span>
          </div>
          {creating && <CategoryForm onDone={onCreated} />}
          {categories.map((c) => {
            const t = out.get(c.id);
            const net = t ? (c.budgeted ? -(t.outCents - t.inCents) : t.inCents - t.outCents) : 0;
            return (
              <div key={c.id}>
                <div
                  role="row"
                  className={cn(
                    "grid min-h-12 grid-cols-[minmax(0,1fr)_auto_auto] sm:grid-cols-[minmax(0,1fr)_120px_60px_140px_72px] items-center gap-x-4 border-b border-rule px-5 py-1.5",
                    editing === c.id ? "bg-tinta-soft" : "bg-paper-raised",
                  )}
                >
                  <span role="cell" className="flex min-w-0">
                    <CategoryTag name={c.name} color={c.color} />
                  </span>
                  <span role="cell" className={cn("hidden text-[14px] leading-5 sm:block", !c.budgeted && "text-ink-muted")}>
                    {c.budgeted ? "Has a limit" : "Not counted"}
                  </span>
                  <span role="cell" className="hidden text-right type-figure sm:block">
                    {rulesByCategory.get(c.id) ?? 0}
                  </span>
                  <span role="cell" className="text-right">
                    <Amount cents={net} sign={c.budgeted ? "negative" : "always"} />
                  </span>
                  <span role="cell" className="flex justify-end">
                    <Button variant="ghost" size="sm" aria-label={`Edit ${c.name}`} onClick={() => setEditing(editing === c.id ? null : c.id)}>
                      {editing === c.id ? "Close" : "Edit"}
                    </Button>
                  </span>
                </div>
                {editing === c.id && <CategoryForm category={c} rules={rulesByCategory.get(c.id) ?? 0} onDone={() => setEditing(null)} />}
              </div>
            );
          })}
        </div>
      </div>
    </Panel>
  );
}

function CategoryForm({ category, rules, onDone }: { category?: Category; rules?: number; onDone: () => void }) {
  const [name, setName] = useState(category?.name ?? "");
  const [color, setColor] = useState<NoteColor>(category?.color ?? "note-200");
  const [budgeted, setBudgeted] = useState(category?.budgeted ?? true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      if (category) await send("PATCH", `/api/v1/categories/${category.id}`, { name, color, budgeted });
      else await send("POST", "/api/v1/categories", { name, color, budgeted });
      onDone();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function archive() {
    if (!category) return;
    setBusy(true);
    try {
      await send("DELETE", `/api/v1/categories/${category.id}`);
      onDone();
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }

  return (
    <section
      aria-label={category ? `Edit ${category.name}` : "New category"}
      className="flex flex-wrap items-start gap-x-8 gap-y-5 border-b border-rule bg-paper px-5 pt-4 pb-5 shadow-[inset_3px_0_0_var(--tinta)]"
    >
      <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} error={error ?? undefined} className="max-w-60 min-w-0 flex-[1_1_180px]" />
      <div className="flex flex-col gap-1.5">
        <span id={`colour-${category?.id ?? "new"}`} className="text-[13px] leading-[18px] font-semibold">
          Colour
        </span>
        <div role="radiogroup" aria-labelledby={`colour-${category?.id ?? "new"}`} className="flex flex-wrap gap-1">
          {notes.map((n) => (
            <button
              key={n.color}
              type="button"
              role="radio"
              aria-checked={n.color === color}
              aria-label={n.label}
              onClick={() => setColor(n.color)}
              className={cn(
                "flex size-11 cursor-pointer flex-col items-center justify-center gap-[3px] rounded-sm bg-paper-raised p-0 focus-visible:focus-ring",
                n.color === color ? "border-2 border-tinta" : "border border-rule",
              )}
            >
              <span aria-hidden className="h-3 w-5 rounded-xs" style={{ background: `var(--${n.color})` }} />
              <span className="font-figure text-[11px] leading-3 text-ink-muted">{n.short}</span>
            </button>
          ))}
        </div>
      </div>
      <label className="flex cursor-pointer items-start gap-2.5 pt-[26px]">
        <input type="checkbox" checked={budgeted} onChange={(e) => setBudgeted(e.target.checked)} className="mt-0.5 size-4 accent-tinta" />
        <span className="flex flex-col gap-0.5">
          <span className="text-[14px] leading-5 font-semibold">Counts toward the budget</span>
          <span className="type-caption text-ink-muted">Has a monthly limit on the Budget screen.</span>
        </span>
      </label>
      <div className="flex basis-full flex-wrap items-center gap-2">
        <Button onClick={save} disabled={busy || !name.trim()}>
          {category ? "Save category" : "Create category"}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        {category && (
          <>
            <span className="flex-auto" />
            <span className="type-caption text-ink-muted">
              {rules ? `Archiving deletes its ${rules} ${rules === 1 ? "rule" : "rules"}. ` : ""}Transactions already in it keep it.
            </span>
            <Button variant="ghost" onClick={archive} disabled={busy}>
              Archive {category.name}
            </Button>
          </>
        )}
      </div>
    </section>
  );
}
