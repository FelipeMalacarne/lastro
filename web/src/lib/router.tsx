import { useSyncExternalStore, type AnchorHTMLAttributes, type MouseEvent } from "react";

import { updateWaiting } from "./update";

// A router small enough to read: the location is the state, pushState changes
// it, and every subscriber re-renders. Screens are few and flat.

const listeners = new Set<() => void>();
const notify = () => listeners.forEach((l) => l());
window.addEventListener("popstate", notify);

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The current path and query, e.g. "/transactions?month=2026-10". */
export function useLocation(): { path: string; query: URLSearchParams } {
  const href = useSyncExternalStore(subscribe, () => location.pathname + location.search);
  const url = new URL(href, location.origin);
  return { path: url.pathname, query: url.searchParams };
}

export function navigate(to: string, opts: { replace?: boolean } = {}) {
  if (to === location.pathname + location.search) return;
  // With a newer version out, going to another screen loads it. A replace
  // (a filter, a redirect) stays put, so nothing typed is lost.
  if (!opts.replace && updateWaiting()) {
    location.assign(to);
    return;
  }
  if (opts.replace) history.replaceState(null, "", to);
  else history.pushState(null, "", to);
  notify();
}

/** Replaces one query parameter (or removes it with null), keeping the rest. */
export function setQuery(key: string, value: string | null) {
  const q = new URLSearchParams(location.search);
  if (value === null) q.delete(key);
  else q.set(key, value);
  const s = q.toString();
  navigate(location.pathname + (s ? "?" + s : ""), { replace: true });
}

type LinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & { to: string };

/** A link inside the app. The month in view travels with it. */
export function Link({ to, onClick, ...rest }: LinkProps) {
  const month = new URLSearchParams(location.search).get("month");
  const [path, hash] = to.split("#");
  const href = month && !path.includes("?") ? `${path}?month=${month}${hash ? "#" + hash : ""}` : to;
  function go(e: MouseEvent<HTMLAnchorElement>) {
    onClick?.(e);
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    navigate(href);
  }
  return <a href={href} onClick={go} {...rest} />;
}

/** The current location with some query parameters replaced (null removes one). */
export function hrefWith(changes: Record<string, string | null>): string {
  const q = new URLSearchParams(location.search);
  for (const [k, v] of Object.entries(changes)) {
    if (v === null) q.delete(k);
    else q.set(k, v);
  }
  const s = q.toString();
  return location.pathname + (s ? "?" + s : "");
}
