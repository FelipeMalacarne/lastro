import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";

import { cn } from "@/lib/utils";

// shadcn/ui's Button with Cédula's variants: secondary (the default),
// primary (one per view) and ghost.
const buttonVariants = cva(
  "inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md border font-text font-semibold whitespace-nowrap transition-colors duration-[120ms] ease-out focus-visible:focus-ring disabled:cursor-not-allowed disabled:border-rule disabled:bg-paper-sunken disabled:text-ink-muted [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        secondary: "border-rule-strong bg-paper-raised text-ink hover:bg-paper-sunken",
        primary: "border-tinta bg-tinta text-on-tinta hover:border-tinta-strong hover:bg-tinta-strong",
        ghost: "border-transparent bg-transparent text-ink-muted hover:bg-paper-sunken hover:text-ink",
      },
      size: {
        // On a touch screen every control is at least 44px tall (Cédula's touch target).
        md: "h-9 pointer-coarse:h-11 gap-2 px-4 text-[14px] leading-5 [&_svg:not([class*='size-'])]:size-4",
        sm: "h-7 pointer-coarse:h-11 gap-1.5 px-3 text-[13px] leading-5 [&_svg:not([class*='size-'])]:size-3.5",
        icon: "size-9 pointer-coarse:size-11 [&_svg:not([class*='size-'])]:size-4",
        "icon-sm": "size-7 pointer-coarse:size-11 [&_svg:not([class*='size-'])]:size-3.5",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<"button"> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      data-slot="button"
      type={asChild ? undefined : (props.type ?? "button")}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}

export { Button, buttonVariants };
