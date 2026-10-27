import type { CSSProperties } from "react";
import { ChevronDown } from "lucide-react";
import type { NoteColor } from "@/lib/api";
import { cn } from "@/lib/utils";

/** The eight note colours; a category's colour is stored with it. */
export type { NoteColor };

type Props = {
  name?: string;
  color?: NoteColor;
  /** No category yet and waiting in the review queue. Also when name is empty. */
  review?: boolean;
  onClick?: () => void;
  className?: string;
};

/** A category: its note colour as a small swatch, then its name. */
export function CategoryTag({ name, color = "note-200", review, onClick, className }: Props) {
  const toReview = review || !name;
  const style = { "--cat": `var(--${color})` } as CSSProperties;
  const inner = (
    <>
      <span
        aria-hidden
        className={cn(
          "h-2 w-3 flex-none rounded-xs",
          toReview ? "border border-dashed border-warning bg-transparent" : "bg-(--cat)",
        )}
      />
      <span className="truncate">{toReview ? "To review" : name}</span>
      {onClick && (
        <ChevronDown
          aria-hidden
          className={cn("size-3.5 text-ink-muted transition-opacity", !toReview && "opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100")}
        />
      )}
    </>
  );
  const cls = cn(
    "inline-flex min-w-0 items-center gap-2 text-[14px] leading-5",
    toReview ? "font-semibold text-warning" : "text-ink",
    className,
  );
  if (!onClick) {
    return (
      <span className={cls} style={style}>
        {inner}
      </span>
    );
  }
  return (
    <button
      type="button"
      style={style}
      onClick={onClick}
      aria-label={toReview ? "Choose a category" : `Category: ${name}. Change`}
      className={cn(
        cls,
        "group -mx-2 h-7 cursor-pointer rounded-sm border border-transparent bg-transparent px-2 hover:border-rule hover:bg-paper-sunken focus-visible:border-rule focus-visible:bg-paper-sunken",
      )}
    >
      {inner}
    </button>
  );
}
