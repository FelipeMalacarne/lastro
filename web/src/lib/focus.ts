/**
 * Gives focus back after something closes, when closing left it on the page's
 * body: to the first of selectors on the page (a copy hidden at this width
 * does not count). What opened it may be gone, a row deleted or a form
 * closed, so the selectors after it stand in.
 */
export function focusBack(...selectors: string[]) {
  const at = document.activeElement;
  if (at && at !== document.body) return;
  for (const s of selectors) {
    const el = Array.from(document.querySelectorAll<HTMLElement>(s)).find((e) => e.getClientRects().length > 0);
    if (el) {
      el.focus();
      return;
    }
  }
}
