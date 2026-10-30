import { useEffect, type ReactNode } from "react";
import { ChartPie, LayoutDashboard, List, Moon, Search, Settings as SettingsIcon, Sun, Tag, TrendingUp, Wallet, type LucideIcon } from "lucide-react";

import { Kbd } from "@/components/Kbd";
import { MonthSwitcher } from "@/components/MonthSwitcher";
import { Toaster } from "@/components/ui/toast";
import { Tooltip, TooltipProvider } from "@/components/ui/tooltip";
import { useResource } from "@/lib/api";
import { useWaiting } from "@/lib/categories";
import { formatMonth } from "@/lib/format";
import { isShortcut } from "@/lib/keys";
import { useMonth } from "@/lib/month";
import { Link, navigate, useLocation } from "@/lib/router";
import { setTheme, useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { ErrorBoundary } from "@/shell/ErrorBoundary";

type Destination = { to: string; label: string; icon: LucideIcon; phone: boolean };

const destinations: Destination[] = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard, phone: true },
  { to: "/transactions", label: "Transactions", icon: List, phone: true },
  { to: "/budget", label: "Budget", icon: Wallet, phone: true },
  { to: "/portfolio", label: "Portfolio", icon: ChartPie, phone: true },
  { to: "/planning", label: "Planning", icon: TrendingUp, phone: true },
];

// Screens that do not look at a month: the phone's top bar leaves the
// switcher out, as their desktop headers do.
const monthless = new Set(["/review", "/portfolio", "/contribute", "/planning", "/settings"]);

function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("font-display text-[22px] leading-7 font-[640] tracking-[-0.02em] text-ink [font-stretch:108%]", className)}>
      lastro
    </span>
  );
}

/** Opens search: Transactions with its field focused. */
function openSearch() {
  if (location.pathname !== "/transactions") navigate("/transactions" + location.search);
  requestAnimationFrame(() => document.querySelector<HTMLInputElement>("[data-search]")?.focus());
}

/** One row of the sidebar: icon, label and an optional count, with the current page marked by an ink edge. */
function NavItem({ to, label, icon: Icon, here, count, countLabel }: { to: string; label: string; icon: LucideIcon; here: boolean; count?: number; countLabel?: string }) {
  return (
    <Link
      to={to}
      aria-current={here ? "page" : undefined}
      className={cn(
        "flex h-9 items-center gap-3 rounded-sm px-2 text-[14px] leading-5 font-semibold no-underline transition-colors",
        here ? "bg-tinta-soft text-tinta shadow-[inset_2px_0_0_var(--tinta)] hover:text-tinta" : "text-ink hover:bg-paper-sunken hover:text-ink",
      )}
    >
      <Icon className={cn("size-4 flex-none", here ? "text-tinta" : "text-ink-muted")} aria-hidden />
      <span className="flex-1">{label}</span>
      {count ? (
        <Tooltip label={countLabel} side="right">
          <span className="inline-flex h-5 min-w-[22px] items-center justify-center rounded-sm bg-warning-soft px-1.5 font-figure text-[12px] leading-4 font-medium text-warning">
            {count}
            <span className="sr-only"> {countLabel}</span>
          </span>
        </Tooltip>
      ) : null}
    </Link>
  );
}

function ThemeToggle() {
  const { dark } = useTheme();
  return (
    <Tooltip label={dark ? "Switch to the light theme" : "Switch to the dark theme"}>
      <button
        type="button"
        onClick={() => setTheme(dark ? "light" : "dark")}
        aria-label={dark ? "Switch to the light theme" : "Switch to the dark theme"}
        className="inline-flex size-8 cursor-pointer items-center justify-center rounded-sm border border-transparent bg-transparent text-ink-muted hover:bg-paper-sunken hover:text-ink focus-visible:focus-ring"
      >
        {dark ? <Sun className="size-4" aria-hidden /> : <Moon className="size-4" aria-hidden />}
      </button>
    </Tooltip>
  );
}

type Props = {
  /** The accounts list in the sidebar (desktop). */
  accounts?: ReactNode;
  children: ReactNode;
};

/**
 * The frame of every screen (canvas: "Sidebar"). From 1024px: a 232px sidebar
 * with the wordmark, search, the navigation with the review count, the
 * accounts with their sync state, then Categories and rules, Settings, the
 * build and the theme. Below: a top bar with the month and Settings, and a
 * bottom tab bar with five destinations.
 */
export function AppShell({ accounts, children }: Props) {
  const { path } = useLocation();
  const { month, prev, next, isCurrent } = useMonth();
  const waiting = useWaiting();
  const me = useResource<{ version: string; env: string }>("/api/v1/me");

  // "/" opens search from anywhere; R opens the review queue. Neither reaches
  // under an open dialog or sheet.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!isShortcut(e)) return;
      if (e.key.toLowerCase() === "r" && location.pathname !== "/review") {
        e.preventDefault();
        navigate("/review");
        return;
      }
      if (e.key !== "/") return;
      e.preventDefault();
      openSearch();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const build = !me.data ? "" : me.data.version === "dev" ? "dev build" : `${me.data.version.slice(0, 7)} · ${me.data.env}`;

  return (
    <TooltipProvider>
      <div className="min-h-dvh bg-paper font-text text-ink lg:flex">
        <aside className="sticky top-0 hidden h-dvh w-[232px] flex-none flex-col gap-6 overflow-y-auto border-r border-rule px-3 pt-5 pb-4 lg:flex">
          <div className="flex flex-col gap-4">
            <Link to="/dashboard" aria-label="lastro, go to Dashboard" className="self-start px-2 py-0.5 no-underline">
              <Wordmark />
            </Link>
            <button
              type="button"
              onClick={openSearch}
              className="flex h-9 w-full cursor-pointer items-center gap-2 rounded-md border border-rule-strong bg-paper-raised pr-2 pl-2.5 text-left text-[14px] leading-5 text-ink-muted hover:text-ink focus-visible:focus-ring"
            >
              <Search className="size-4" aria-hidden />
              <span className="flex-1">Search</span>
              <Kbd>/</Kbd>
            </button>
          </div>
          <nav aria-label="Main" className="flex flex-col gap-0.5">
            {destinations.map((d) => (
              <NavItem
                key={d.to}
                to={d.to}
                label={d.label}
                icon={d.icon}
                here={path === d.to || (d.to === "/transactions" && path === "/review") || (d.to === "/portfolio" && path === "/contribute")}
                count={d.to === "/transactions" ? waiting : undefined}
                countLabel={waiting ? `${waiting} to review` : undefined}
              />
            ))}
          </nav>
          {accounts}
          <div className="mt-auto flex flex-col gap-0.5 border-t border-rule pt-3">
            <NavItem to="/categories" label="Categories and rules" icon={Tag} here={path === "/categories"} />
            <NavItem to="/settings" label="Settings" icon={SettingsIcon} here={path === "/settings"} />
            <div className="mt-1 flex h-9 items-center justify-between gap-2 pl-2">
              <span className="type-figure-sm text-ink-muted">{build}</span>
              <ThemeToggle />
            </div>
          </div>
        </aside>

        <div className="min-w-0 flex-1">
          <header className="sticky top-0 z-10 flex h-14 items-center justify-between gap-2 border-b border-rule bg-paper px-4 lg:hidden">
            <Link to="/dashboard" aria-label="lastro, go to Dashboard" className="no-underline">
              <Wordmark />
            </Link>
            <div className="flex items-center gap-1">
              {!monthless.has(path) && !(path === "/transactions" && new URLSearchParams(location.search).get("scope") === "all") && <MonthSwitcher label={formatMonth(month, true)} onPrev={prev} onNext={next} nextDisabled={isCurrent} />}
              <Link
                to="/settings"
                aria-label="Settings"
                aria-current={path === "/settings" ? "page" : undefined}
                className={cn(
                  "inline-flex size-11 items-center justify-center rounded-sm no-underline",
                  path === "/settings" ? "text-tinta" : "text-ink-muted hover:text-ink",
                )}
              >
                <SettingsIcon className="size-5" aria-hidden />
              </Link>
            </div>
          </header>
          <main className="px-4 pt-4 pb-24 lg:px-8 lg:pt-8 lg:pb-14">
            <div className="mx-auto flex max-w-[1180px] flex-col gap-4 lg:gap-6">
              {/* Keyed by the path, so going to another screen leaves a failed one behind. */}
              <ErrorBoundary key={path}>{children}</ErrorBoundary>
            </div>
          </main>
          <nav aria-label="Main" className="fixed inset-x-0 bottom-0 z-10 grid h-16 grid-cols-5 border-t border-rule bg-paper-raised pb-[env(safe-area-inset-bottom)] lg:hidden">
            {destinations
              .filter((d) => d.phone)
              .map((d) => {
                const here = path === d.to || (d.to === "/transactions" && path === "/review") || (d.to === "/portfolio" && path === "/contribute");
                return (
                  <Link
                    key={d.to}
                    to={d.to}
                    aria-current={here ? "page" : undefined}
                    className={cn(
                      "relative flex flex-col items-center justify-center gap-1 text-[12px] leading-4 font-semibold no-underline",
                      here ? "text-tinta hover:text-tinta" : "text-ink-muted hover:text-ink",
                    )}
                  >
                    <d.icon className="size-5" aria-hidden />
                    {d.label}
                    {d.to === "/transactions" && waiting ? (
                      <span className="absolute top-2 left-[calc(50%+6px)] inline-flex h-4 min-w-4 items-center justify-center rounded-sm bg-warning-soft px-1 font-figure text-[11px] leading-4 font-medium text-warning">
                        {waiting > 99 ? "99+" : waiting}
                        <span className="sr-only"> to review</span>
                      </span>
                    ) : null}
                  </Link>
                );
              })}
          </nav>
        </div>
        <Toaster />
      </div>
    </TooltipProvider>
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
      {/* On a phone the month is in the top bar; the actions stay with the page. */}
      {actions && <div className="flex flex-wrap items-center gap-2 lg:hidden">{actions}</div>}
    </header>
  );
}
