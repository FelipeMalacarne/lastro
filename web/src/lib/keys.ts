/**
 * Whether a key press is one of the page's shortcuts: no modifier, not typed
 * in a field or a menu, and nothing open over the page. `over` is what counts
 * as open over it: any dialog or sheet, unless a screen says otherwise. One
 * on its way out no longer counts.
 */
export function isShortcut(e: KeyboardEvent, over = "[role=dialog], [role=alertdialog]"): boolean {
  const t = e.target as HTMLElement;
  const open = Array.from(document.querySelectorAll(over)).some((d) => d.getAttribute("data-state") !== "closed");
  return !(e.metaKey || e.ctrlKey || e.altKey || t.closest("input, textarea, select, [contenteditable], [role=menu]") || open);
}
