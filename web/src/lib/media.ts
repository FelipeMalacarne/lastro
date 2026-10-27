import { useSyncExternalStore } from "react";

/** Whether a media query matches, kept current. */
export function useMedia(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const m = window.matchMedia(query);
      m.addEventListener("change", onChange);
      return () => m.removeEventListener("change", onChange);
    },
    () => window.matchMedia(query).matches,
  );
}

/** The desktop shell starts at 1024px (Tailwind's lg). */
export const useWide = () => useMedia("(min-width: 1024px)");
