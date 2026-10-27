import { useId, type InputHTMLAttributes, type ReactNode } from "react";
import { CircleAlert } from "lucide-react";
import { cn } from "@/lib/utils";

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, "prefix"> & {
  label?: string;
  hint?: string;
  error?: string;
  /** Inside the field, before the text: an icon or "R$". */
  prefix?: ReactNode;
  suffix?: ReactNode;
  /** Right-aligned figures and a decimal keyboard on phones. */
  numeric?: boolean;
};

/** A labelled input. The label is always visible; the hint or error sits under it. */
export function TextField({ label, hint, error, prefix, suffix, numeric, className, id, ...rest }: Props) {
  const auto = useId();
  const inputId = id ?? auto;
  const message = error || hint;
  return (
    <div className={cn("grid min-w-0 gap-1.5", className)}>
      {label && (
        <label htmlFor={inputId} className="text-[13px] leading-[18px] font-semibold text-ink">
          {label}
        </label>
      )}
      <div
        className={cn(
          "flex h-9 min-w-0 items-center gap-2 rounded-sm border bg-paper-raised px-3 focus-within:border-tinta focus-within:focus-ring",
          error ? "border-danger" : "border-rule-strong",
        )}
      >
        {prefix && <span className="flex items-center font-figure text-[14px] leading-5 text-ink-muted">{prefix}</span>}
        <input
          id={inputId}
          inputMode={numeric ? "decimal" : undefined}
          aria-invalid={error ? true : undefined}
          aria-describedby={message ? inputId + "-hint" : undefined}
          className={cn(
            "h-full min-w-0 flex-1 border-0 bg-transparent p-0 text-[15px] leading-[22px] text-ink outline-none placeholder:text-ink-muted focus-visible:shadow-none",
            numeric && "text-right font-figure font-medium",
          )}
          {...rest}
        />
        {suffix && <span className="flex items-center font-figure text-[14px] leading-5 text-ink-muted">{suffix}</span>}
      </div>
      {message && (
        <p id={inputId + "-hint"} className={cn("m-0 flex items-center gap-1 type-caption", error ? "text-danger" : "text-ink-muted")}>
          {error && <CircleAlert className="size-3.5" aria-hidden />}
          {message}
        </p>
      )}
    </div>
  );
}
