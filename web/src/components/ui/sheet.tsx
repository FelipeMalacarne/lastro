import { createContext, useContext, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { Dialog as SheetPrimitive } from "radix-ui";

import { Button } from "@/components/ui/button";

// Cédula's Sheet on Radix's Dialog: the phone's way to open one thing in full
// over the list it came from. It rises from the bottom edge, and dragging the
// head down past a third of its height, or flicking it, closes it.

const FooterContext = createContext<HTMLElement | null>(null);

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** What is open, in the label style: "Transaction", "Restaurants · October". */
  label: string;
  /** Buttons before ✕ in the head, such as newer and older. */
  actions?: ReactNode;
  /** Where focus goes on close; by default the element focused before it opened. */
  onCloseAutoFocus?: (e: Event) => void;
  children: ReactNode;
};

export function Sheet({ open, onOpenChange, label, actions, onCloseAutoFocus, children }: Props) {
  const content = useRef<HTMLDivElement>(null);
  const drag = useRef<{ y: number; t: number; dy: number } | null>(null);
  const [footer, setFooter] = useState<HTMLDivElement | null>(null);

  function settle() {
    const el = content.current;
    if (!el) return;
    el.style.transition = "transform 160ms ease-out";
    el.style.transform = "";
  }
  function down(e: PointerEvent<HTMLDivElement>) {
    if (e.button !== 0 || (e.target as HTMLElement).closest("button, a")) return;
    drag.current = { y: e.clientY, t: e.timeStamp, dy: 0 };
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function move(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current;
    const el = content.current;
    if (!d || !el) return;
    d.dy = Math.max(0, e.clientY - d.y);
    el.style.transition = "none";
    el.style.transform = `translateY(${d.dy}px)`;
  }
  function up(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current;
    drag.current = null;
    if (!d || !content.current) return;
    const speed = d.dy / Math.max(1, e.timeStamp - d.t);
    if (d.dy > content.current.offsetHeight / 3 || (d.dy > 32 && speed > 0.6)) onOpenChange(false);
    else settle();
  }

  return (
    <SheetPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <SheetPrimitive.Portal>
        <SheetPrimitive.Overlay className="fixed inset-0 z-30 bg-(--scrim) data-[state=closed]:animate-[lt-fade-out_200ms_ease-in] data-[state=open]:animate-[lt-fade_160ms_ease-out]" />
        <SheetPrimitive.Content
          ref={content}
          aria-describedby={undefined}
          onCloseAutoFocus={onCloseAutoFocus}
          // Esc in a field with its suggestions open closes the suggestions, not the sheet.
          onEscapeKeyDown={(e) => {
            if ((e.target as Element | null)?.closest?.('[role=combobox][aria-expanded="true"]')) e.preventDefault();
          }}
          className="fixed inset-x-0 bottom-0 z-30 flex max-h-[92dvh] flex-col rounded-t-lg bg-paper-raised text-ink shadow-pop outline-none data-[state=closed]:animate-[lt-sheet-out_200ms_ease-in] data-[state=open]:animate-[lt-sheet-in_240ms_cubic-bezier(0.2,0,0,1)]"
        >
          <div
            onPointerDown={down}
            onPointerMove={move}
            onPointerUp={up}
            onPointerCancel={() => {
              drag.current = null;
              settle();
            }}
            className="flex flex-none cursor-grab touch-none flex-col items-center gap-2 px-4 pt-2 select-none"
          >
            <span aria-hidden className="h-1 w-9 rounded-[2px] bg-rule-strong" />
            <div className="flex items-center gap-1 self-stretch">
              <SheetPrimitive.Title className="m-0 min-w-0 flex-1 truncate type-label text-ink-muted">{label}</SheetPrimitive.Title>
              {actions}
              <SheetPrimitive.Close asChild>
                <Button variant="ghost" size="icon" aria-label="Close" className="-mr-2.5 size-11 text-ink [&_svg:not([class*='size-'])]:size-5">
                  <X aria-hidden />
                </Button>
              </SheetPrimitive.Close>
            </div>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-6">
            <FooterContext.Provider value={footer}>{children}</FooterContext.Provider>
          </div>
          <div
            ref={setFooter}
            className="hidden flex-none flex-wrap items-center gap-2 border-t border-rule bg-paper-raised px-4 pt-3 pb-[calc(12px+env(safe-area-inset-bottom))] has-[>*]:flex [&>*]:h-11 [&>*]:flex-auto"
          />
        </SheetPrimitive.Content>
      </SheetPrimitive.Portal>
    </SheetPrimitive.Root>
  );
}

/**
 * A form's actions: in a sheet they move to its footer, which stays above the
 * keyboard; anywhere else they render in place. A submit button in it names
 * its form with the form attribute, since in a sheet it leaves the form.
 */
export function SheetActions({ className, children }: { className?: string; children: ReactNode }) {
  const footer = useContext(FooterContext);
  return footer ? createPortal(children, footer) : <div className={className}>{children}</div>;
}
