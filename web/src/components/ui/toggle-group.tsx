import * as React from "react";
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

// shadcn/ui's ToggleGroup, styled as Cédula's segmented control: a sunken
// well whose selected segment is raised.
function ToggleGroup({ className, ...props }: React.ComponentProps<typeof ToggleGroupPrimitive.Root>) {
  return (
    <ToggleGroupPrimitive.Root
      data-slot="toggle-group"
      className={cn("inline-flex w-fit gap-0.5 rounded-md border border-rule bg-paper-sunken p-0.5", className)}
      {...props}
    />
  );
}

function ToggleGroupItem({ className, ...props }: React.ComponentProps<typeof ToggleGroupPrimitive.Item>) {
  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      className={cn(
        "h-7 cursor-pointer rounded-sm border-0 bg-transparent px-3 text-[13px] leading-[18px] font-semibold text-ink-muted hover:text-ink focus-visible:focus-ring",
        "data-[state=on]:bg-paper-raised data-[state=on]:text-ink data-[state=on]:shadow-segment",
        className,
      )}
      {...props}
    />
  );
}

export { ToggleGroup, ToggleGroupItem };
