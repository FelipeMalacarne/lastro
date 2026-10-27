import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  title?: ReactNode;
  action?: ReactNode;
  footer?: ReactNode;
  className?: string;
  bodyClassName?: string;
  children?: ReactNode;
};

/** A surface on the page. Panels never nest and never cast shadows. */
export function Panel({ title, action, footer, className, bodyClassName, children }: Props) {
  return (
    <section className={cn("flex min-w-0 flex-col gap-4 overflow-hidden rounded-md border border-rule bg-paper-raised p-5", className)}>
      {(title || action) && (
        <header className="flex min-h-7 items-center justify-between gap-3">
          {title && <h2 className="m-0 type-heading text-balance">{title}</h2>}
          {action && <div>{action}</div>}
        </header>
      )}
      <div className={cn("min-w-0 flex-auto", bodyClassName)}>{children}</div>
      {footer && <footer className="border-t border-rule pt-3 type-caption text-ink-muted">{footer}</footer>}
    </section>
  );
}
