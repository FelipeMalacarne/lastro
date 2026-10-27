import type { ReactNode } from "react";
import { CircleAlert, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/utils";

type Props = {
  tone: "warning" | "danger";
  /** The fact, in bold: "Itaú last synced 3 days ago." */
  title: string;
  children?: ReactNode;
  /** One ghost action, e.g. a link to Meu Pluggy. */
  action?: ReactNode;
};

/** A slim bar under the page header for a stale or broken connection. One at a time. */
export function Notice({ tone, title, children, action }: Props) {
  const Icon = tone === "danger" ? CircleAlert : TriangleAlert;
  return (
    <div
      role="status"
      className={cn(
        "flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md py-2.5 pr-3 pl-3.5 text-[14px] leading-5 text-ink lg:py-1.5 lg:pr-2",
        tone === "danger" ? "bg-danger-soft" : "bg-warning-soft",
      )}
    >
      <Icon className={cn("size-4 flex-none", tone === "danger" ? "text-danger" : "text-warning")} aria-hidden />
      <span className="min-w-0 flex-[1_1_240px] lg:flex-[1_1_360px]">
        <strong className="font-semibold">{title}</strong> {children}
      </span>
      {action}
    </div>
  );
}
