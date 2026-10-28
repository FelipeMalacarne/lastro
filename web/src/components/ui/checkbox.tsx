import * as React from "react";
import { Check } from "lucide-react";
import { Checkbox as CheckboxPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

// shadcn/ui's Checkbox on Cédula: an 18px box with radius-xs and a
// rule-strong border; checked, a tinta fill with an on-tinta check. For a
// choice inside a form that saves with its button.
function Checkbox({ className, ...props }: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "relative inline-flex size-[18px] flex-none cursor-pointer items-center justify-center rounded-xs border border-rule-strong bg-paper-raised p-0 text-on-tinta focus-visible:focus-ring disabled:cursor-not-allowed disabled:opacity-50",
        "data-[state=checked]:border-tinta data-[state=checked]:bg-tinta",
        "before:absolute before:-inset-3 before:content-[''] pointer-fine:before:hidden",
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator>
        <Check className="size-3" strokeWidth={3} aria-hidden />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

export { Checkbox };
