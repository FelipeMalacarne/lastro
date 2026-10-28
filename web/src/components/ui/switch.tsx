import * as React from "react";
import { Switch as SwitchPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

// shadcn/ui's Switch on Cédula: square corners, like everything else, a
// paper-sunken track with a rule-strong border when off and a tinta one when
// on. For a setting saved the moment it is flipped; inside a form, Checkbox.
function Switch({ className, ...props }: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "relative inline-flex h-5 w-9 flex-none cursor-pointer items-center rounded-sm border border-rule-strong bg-paper-sunken p-0 transition-colors duration-[120ms] ease-out focus-visible:focus-ring disabled:cursor-not-allowed disabled:opacity-50",
        "data-[state=checked]:border-tinta data-[state=checked]:bg-tinta",
        // On touch the target reaches 44px without changing how it looks.
        "before:absolute before:-inset-3 before:content-[''] pointer-fine:before:hidden",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="block size-3.5 translate-x-[2px] rounded-xs border border-rule-strong bg-paper-raised shadow-segment transition-transform duration-[120ms] ease-out data-[state=checked]:translate-x-[18px] data-[state=checked]:border-on-tinta data-[state=checked]:bg-on-tinta" />
    </SwitchPrimitive.Root>
  );
}

export { Switch };
