import { useEffect, type ReactNode } from "react";
import { ChartPie, LayoutDashboard, List, TrendingUp, Wallet, type LucideIcon } from "lucide-react";

import { Kbd } from "@/components/Kbd";
import { MonthSwitcher } from "@/components/MonthSwitcher";
import { formatMonth } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { Link, navigate, useLocation } from "@/lib/router";
import { cn } from "@/lib/utils";

type Destination = { to: string; label: string; icon: LucideIcon; phone: boolean };

const destinations: Destination[] = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard, phone: true },
  { to: "/transactions", label: "Transactions", icon: List, phone: true },
  { to: "/budget", label: "Budget", icon: Wallet, phone: true },
  { to: "/portfolio", label: "Portfolio", icon: ChartPie, phone: true },
  { to: "/planning", label: "Planning", icon: TrendingUp, phone: false },
];

function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("font-display text-[22px] leading-7 font-[640] tracking-[-0.02em] text-ink [font-stretch:108%]", className)}>
      lastro
    </span>
  );
}

type Props = {
  /** The accounts list in the sidebar (desktop). */
  accounts?: ReactNode;
  children: ReactNode;
};

/**
 * The frame of every screen. From 1024px: a 232px sidebar with the wordmark,
 * the navigation and the accounts. Below: a top bar with the month and a
 * bottom tab bar with four destinations.
 */
export function AppShell({ accounts, children }: Props) {
  const { path } = useLocation();
  const { month, prev, next, isCurrent } = useMonth();

  // "/" opens search from anywhere: on Transactions it focuses the field.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const t = e.target as HTMLElement;
      if (e.metaKey || e.ctrlKey || e.altKey || t.closest("input, textarea, select, [contenteditable], [role=menu]")) return;
      // R opens the review queue from anywhere.
      if (e.key === "r" && location.pathname !== "/review") {
        e.preventDefault();
        navigate("/review");
        return;
      }
      if (e.key !== "/") return;
      e.preventDefault();
      if (location.pathname !== "/transactions") navigate("/transactions" + location.search);
      requestAnimationFrame(() => document.querySelector<HTMLInputElement>("[data-search]")?.focus());
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="min-h-dvh bg-paper font-text text-ink lg:flex">
      <aside className="sticky top-0 hidden h-dvh w-[232px] flex-none flex-col gap-7 overflow-y-auto border-r border-rule px-4 py-6 lg:flex">
        <div className="flex items-center justify-between px-2">
          <Wordmark />
          <span className="flex items-center gap-1.5 text-[13px] text-ink-muted">
            Search <Kbd>/</Kbd>
          </span>
        </div>
        <nav aria-label="Main" className="flex flex-col gap-0.5">
          {destinations.map((d) => {
            const here = path === d.to;
            return (
              <Link
                key={d.to}
                to={d.to}
                aria-current={here ? "page" : undefined}
                className={cn(
                  "flex h-9 items-center gap-3 rounded-sm px-2 text-[14px] font-semibold no-underline hover:bg-paper-sunken",
                  here ? "bg-tinta-soft text-tinta hover:bg-tinta-soft hover:text-tinta" : "text-ink hover:text-ink",
                )}
              >
                <d.icon className="size-4" aria-hidden />
                {d.label}
              </Link>
            );
          })}
        </nav>
        {accounts}
        <p className="m-0 mx-2 mt-auto type-caption text-ink-muted">Synced from Meu Pluggy four times a day.</p>
      </aside>

      <div className="min-w-0 flex-1">
        <header className="sticky top-0 z-10 flex h-14 items-center justify-between gap-2 border-b border-rule bg-paper px-4 lg:hidden">
          <Wordmark />
          <MonthSwitcher label={formatMonth(month, true)} onPrev={prev} onNext={next} nextDisabled={isCurrent} />
        </header>
        <main className="px-4 pt-4 pb-24 lg:px-8 lg:pt-8 lg:pb-14">
          <div className="mx-auto flex max-w-[1180px] flex-col gap-4 lg:gap-6">{children}</div>
        </main>
        <nav aria-label="Main" className="fixed inset-x-0 bottom-0 z-10 grid h-16 grid-cols-4 border-t border-rule bg-paper-raised lg:hidden">
          {destinations
            .filter((d) => d.phone)
            .map((d) => {
              const here = path === d.to;
              return (
                <Link
                  key={d.to}
                  to={d.to}
                  aria-current={here ? "page" : undefined}
                  className={cn(
                    "flex flex-col items-center justify-center gap-1 text-[12px] leading-4 font-semibold no-underline",
                    here ? "text-tinta hover:text-tinta" : "text-ink-muted hover:text-ink",
                  )}
                >
                  <d.icon className="size-5" aria-hidden />
                  {d.label}
                </Link>
              );
            })}
        </nav>
      </div>
    </div>
  );
}

/** The page name with one caption line, and the month and actions on the right. */
export function PageHeader({ title, caption, actions, monthless }: { title: string; caption?: ReactNode; actions?: ReactNode; monthless?: boolean }) {
  const { month, prev, next, isCurrent } = useMonth();
  return (
    <header className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex flex-col gap-0.5">
        <h1 className="m-0 type-title">{title}</h1>
        {caption && <p className="m-0 type-caption text-ink-muted">{caption}</p>}
      </div>
      <div className="hidden items-center gap-2 lg:flex">
        {!monthless && <MonthSwitcher label={formatMonth(month)} onPrev={prev} onNext={next} nextDisabled={isCurrent} />}
        {actions}
      </div>
    </header>
  );
}
