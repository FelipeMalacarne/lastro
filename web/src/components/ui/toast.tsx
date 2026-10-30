import { useRef } from "react";
import { Check, CircleAlert, RefreshCw, X } from "lucide-react";
import { Toast as ToastPrimitive } from "radix-ui";

import { Kbd } from "@/components/Kbd";
import { useWide } from "@/lib/media";
import { closeToast, useToasts, type ToastItem } from "@/lib/toast";
import { cn } from "@/lib/utils";

// Cédula's Toast on Radix's Toast. On desktop the stack sits bottom right; on a
// phone it spans the gutters just above the tab bar. Hover or focus pauses it.

const tones = {
  done: { icon: Check, className: "text-inflow" },
  error: { icon: CircleAlert, className: "text-danger" },
  busy: { icon: RefreshCw, className: "animate-spin text-ink-muted [animation-duration:1.2s]" },
};

/** 4s for a report, 8s with an action; an error, and something still running, stay. */
function duration(t: ToastItem): number {
  if (t.tone !== "done") return Infinity;
  return t.action ? 8000 : 4000;
}

export function Toaster() {
  const items = useToasts();
  const wide = useWide();
  // The toast focus is in, by id. Pressing Undo replaces that toast with one
  // that says how it went, and the focus moves on to it instead of the body.
  const focused = useRef<string | null>(null);
  return (
    <ToastPrimitive.Provider swipeDirection="right" label="Notification">
      {items.map((t) => {
        const tone = tones[t.tone];
        return (
          <ToastPrimitive.Root
            key={t.key}
            ref={(el) => {
              if (el && focused.current === t.id && document.activeElement === document.body) el.focus();
            }}
            onFocus={() => (focused.current = t.id)}
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget)) focused.current = null;
            }}
            open={t.open}
            onOpenChange={(o) => !o && closeToast(t.key)}
            duration={duration(t)}
            type={t.tone === "error" ? "foreground" : "background"}
            className={cn(
              "pointer-events-auto flex min-h-11 items-center gap-3 rounded-md border border-rule bg-paper-raised py-2 pr-2 pl-4 text-[14px] leading-5 text-ink shadow-pop outline-none focus-visible:focus-ring",
              "data-[state=closed]:animate-[lt-fade-out_120ms_ease-in] data-[state=open]:animate-[lt-pop_160ms_ease-out]",
              "data-[swipe=move]:translate-x-(--radix-toast-swipe-move-x) data-[swipe=cancel]:translate-x-0 data-[swipe=cancel]:transition-transform data-[swipe=end]:animate-[lt-toast-out_120ms_ease-out]",
            )}
          >
            <tone.icon className={cn("size-4 flex-none", tone.className)} aria-hidden />
            <ToastPrimitive.Description className="min-w-0 flex-1 py-0.5 [&_strong]:font-semibold">{t.text}</ToastPrimitive.Description>
            {t.action && (
              <button
                type="button"
                onClick={() => void t.action!.run()}
                className="inline-flex h-7 flex-none cursor-pointer items-center gap-1.5 rounded-sm border-0 bg-transparent px-3 text-[13px] leading-[18px] font-semibold text-tinta hover:bg-tinta-soft hover:text-tinta-strong focus-visible:focus-ring pointer-coarse:h-11"
              >
                {t.action.label}
                {wide && t.action.kbd && <Kbd>{t.action.kbd}</Kbd>}
              </button>
            )}
            {t.tone !== "busy" && (
              <ToastPrimitive.Close
                aria-label="Dismiss"
                className="inline-flex size-7 flex-none cursor-pointer items-center justify-center rounded-sm border-0 bg-transparent p-0 text-ink-muted hover:bg-paper-sunken hover:text-ink focus-visible:focus-ring pointer-coarse:size-11"
              >
                <X className="size-3.5" aria-hidden />
              </ToastPrimitive.Close>
            )}
          </ToastPrimitive.Root>
        );
      })}
      {/* An open sheet or dialog hides the rest of the page from screen readers,
          except live regions: aria-live keeps the toasts reachable, and "off"
          leaves the announcing to Radix's own region. */}
      <ToastPrimitive.Viewport
        aria-live="off"
        className="fixed z-50 m-0 flex list-none flex-col gap-2 p-0 outline-none max-lg:inset-x-4 max-lg:bottom-[calc(72px+env(safe-area-inset-bottom))] lg:right-6 lg:bottom-6 lg:w-[400px]" />
    </ToastPrimitive.Provider>
  );
}
