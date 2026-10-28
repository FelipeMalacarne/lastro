import type { ReactElement, ReactNode } from "react";
import { Tooltip as TooltipPrimitive } from "radix-ui";

// shadcn/ui's Tooltip on Cédula: the one inverted surface, paper on ink, in
// the caption size. It names an icon-only control or explains a marker after
// 400ms of hover, or at once on keyboard focus. Touch never shows it, so a
// tooltip only repeats or names what the screen already says.

/** Once, around the app: the delays every tooltip shares. */
export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={400} skipDelayDuration={300}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

type Props = {
  /** A few words: "Sync every account now", "Pending: may still change." */
  label: ReactNode;
  /** The trigger, one element that takes focus (a button or link) or an element given tabIndex 0. */
  children: ReactElement;
  side?: "top" | "right" | "bottom" | "left";
};

export function Tooltip({ label, children, side = "top" }: Props) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          sideOffset={6}
          collisionPadding={8}
          className="z-50 max-w-60 rounded-sm bg-ink px-2 py-1 type-caption text-paper data-[state=closed]:animate-[lt-fade-out_120ms_ease-in] data-[state=delayed-open]:animate-[lt-fade_120ms_ease-out] data-[state=instant-open]:animate-[lt-fade_120ms_ease-out]"
        >
          {label}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
