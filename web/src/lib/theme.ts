// Light or dark: the device's, unless a choice is saved in this browser.
// tokens.css switches every token on [data-theme="dark"], so the attribute on
// <html> is the whole mechanism; index.html sets it before the first paint.

import { useSyncExternalStore } from "react";

export type ThemePreference = "system" | "light" | "dark";

const key = "lastro.theme";
const media = matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();

function read(): ThemePreference {
  try {
    const v = localStorage.getItem(key);
    if (v === "light" || v === "dark") return v;
  } catch {
    // Storage can be blocked; the device's theme still applies.
  }
  return "system";
}

let preference = read();

function apply() {
  const dark = preference === "dark" || (preference === "system" && media.matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  // The installed app's status bar follows the paper of the theme in use.
  const paper = getComputedStyle(document.documentElement).getPropertyValue("--paper").trim();
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", paper);
}

media.addEventListener("change", () => {
  apply();
  listeners.forEach((l) => l());
});

/** Saves the choice in this browser and repaints. */
export function setTheme(p: ThemePreference) {
  preference = p;
  try {
    if (p === "system") localStorage.removeItem(key);
    else localStorage.setItem(key, p);
  } catch {
    // Not saved, but this session still follows it.
  }
  apply();
  listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The saved preference and the theme it resolves to now. */
export function useTheme(): { preference: ThemePreference; dark: boolean } {
  const p = useSyncExternalStore(subscribe, () => preference);
  const dark = useSyncExternalStore(subscribe, () => document.documentElement.dataset.theme === "dark");
  return { preference: p, dark };
}
