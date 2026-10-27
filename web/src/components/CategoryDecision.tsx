import { useEffect, useState } from "react";
import { Sparkles } from "lucide-react";

import { CategoryTag } from "@/components/CategoryTag";
import { Kbd } from "@/components/Kbd";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import type { Category, Pattern, PatternKind } from "@/lib/api";
import { cn } from "@/lib/utils";

type Props = {
  categories: Category[];
  /** The category it has now, shown selected. */
  current?: string | null;
  /** The rule a choice teaches, as suggested; null when the description is too short for one. */
  suggested: Pattern | null;
  /** Called with the choice and the rule to teach (null: this transaction only). */
  onChoose: (category: string, teach: Pattern | null) => Promise<void> | void;
  /** Number keys 1 to 9 pick a category. Only one decision on screen listens. */
  keyboard?: boolean;
  /** "Applies to 41 transactions". */
  reach?: string;
  busy?: boolean;
  /** The local model's answer, offered first. */
  suggestion?: { category: string; confidence: number } | null;
};

const kinds: { value: PatternKind; label: string }[] = [
  { value: "contains", label: "Contains" },
  { value: "exact", label: "Is exactly" },
];

/**
 * Choosing a category: one click (or key) decides, and the rule it teaches is
 * shown and editable before that. A correction always becomes a rule, so the
 * same merchant is never asked again.
 */
export function CategoryDecision({ categories, current, suggested, onChoose, keyboard, reach, busy, suggestion }: Props) {
  const [kind, setKind] = useState<PatternKind>(suggested?.kind ?? "contains");
  const [text, setText] = useState(suggested?.text ?? "");
  useEffect(() => {
    setKind(suggested?.kind ?? "contains");
    setText(suggested?.text ?? "");
  }, [suggested?.kind, suggested?.text]);

  const letters = text.replace(/[^\p{L}\p{N}]/gu, "").length;
  const valid = letters >= 3 && text.length <= 100;
  const teach: Pattern | null = suggested || text ? (valid ? { kind, text } : null) : null;
  const shown = categories.slice(0, 9);
  const offered = suggestion ? categories.find((c) => c.id === suggestion.category) : undefined;

  useEffect(() => {
    if (!keyboard) return;
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      if (e.metaKey || e.ctrlKey || e.altKey || t.closest("input, textarea, select, [contenteditable], [role=menu]")) return;
      // Enter on a focused button presses that button, not the suggestion.
      if (e.key === "Enter" && offered && !busy && !t.closest("button, a")) {
        e.preventDefault();
        void onChoose(offered.id, teach);
        return;
      }
      const n = Number(e.key);
      if (n >= 1 && n <= shown.length && !busy) {
        e.preventDefault();
        void onChoose(shown[n - 1].id, teach);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [keyboard, shown, teach, onChoose, busy, offered]);

  return (
    <div className="flex flex-col gap-4">
      {offered && suggestion && (
        <div className="flex flex-col gap-3 rounded-md border border-rule bg-paper p-3">
          <span className="flex flex-wrap items-center gap-2 text-[14px] leading-5 text-ink-muted">
            <Sparkles className="size-4" aria-hidden />
            The local model suggests <CategoryTag name={offered.name} color={offered.color} />
            <span className="type-figure-sm">{suggestion.confidence}% sure</span>
          </span>
          <Button variant="primary" size="sm" className="self-start" disabled={busy} onClick={() => void onChoose(offered.id, teach)}>
            Accept {offered.name} {keyboard && <Kbd>Enter</Kbd>}
          </Button>
        </div>
      )}
      <div className="flex flex-col gap-2">
        <span className="type-label text-ink-muted">{offered ? "Or choose another" : "Choose a category"}</span>
        <div role="group" aria-label="Category" className="grid grid-cols-2 gap-1.5">
          {categories.map((c, i) => {
            const on = c.id === current;
            return (
              <button
                key={c.id}
                type="button"
                disabled={busy}
                aria-pressed={on}
                onClick={() => void onChoose(c.id, teach)}
                className={cn(
                  "flex h-11 cursor-pointer items-center justify-between gap-2 rounded-sm border px-2.5 text-left transition-colors focus-visible:focus-ring disabled:cursor-wait lg:h-9",
                  on ? "border-tinta bg-tinta-soft" : "border-rule bg-paper-raised hover:bg-paper-sunken",
                )}
              >
                <CategoryTag name={c.name} color={c.color} />
                {keyboard && i < 9 && <Kbd>{i + 1}</Kbd>}
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex flex-col gap-2 rounded-md border border-rule bg-paper p-3">
        <span className="text-[14px] leading-5 font-semibold">It teaches a rule</span>
        <div className="flex flex-wrap items-end gap-2">
          <SegmentedControl label="Match" options={kinds} value={kind} onChange={setKind} />
          <TextField
            aria-label="Description text"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="e.g. IFOOD"
            className="min-w-0 flex-[1_1_160px]"
            error={text && !valid ? "Needs 3 letters or digits" : undefined}
          />
        </div>
        <p className="m-0 type-caption text-ink-muted">
          {kind === "contains" ? "Whole words, in this order." : "The whole description."} Case, accents and symbols are ignored.
          {reach ? ` ${reach}.` : ""} Rules run before the local model; change them in Categories and rules.
        </p>
      </div>
    </div>
  );
}
