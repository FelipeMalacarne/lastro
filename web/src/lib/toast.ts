// What an action did, reported in a toast (Cédula: Toast). A module-level
// store, so anything can report and the one Toaster in the shell shows it.

import { useSyncExternalStore, type ReactNode } from "react";

export type ToastTone = "done" | "error" | "busy";

export type ToastAction = { label: string; run: () => unknown; kbd?: string };

export type ToastItem = { id: string; key: number; tone: ToastTone; text: ReactNode; action?: ToastAction; open: boolean };

type Report = {
  /** A toast with the same id takes the place of the one before it. */
  id?: string;
  tone?: ToastTone;
  /** The fact, past tense; the thing's name in <strong>. */
  text: ReactNode;
  action?: ToastAction;
  /** Puts back what the action did: the toast offers Undo and says how it went. */
  undo?: () => Promise<unknown>;
};

let items: ToastItem[] = [];
let seq = 0;
const listeners = new Set<() => void>();

function emit() {
  items = [...items];
  listeners.forEach((l) => l());
}

/** Shows a toast and returns its id. Three at most: a fourth closes the oldest. */
export function toast(r: Report): string {
  const id = r.id ?? `toast-${++seq}`;
  const action = r.undo ? { label: "Undo", run: () => undoWith(id, r.undo!) } : r.action;
  items = items.filter((t) => t.id !== id);
  const open = items.filter((t) => t.open);
  if (open.length >= 3) closeToast(open[0].key);
  items.push({ id, key: ++seq, tone: r.tone ?? "done", text: r.text, action, open: true });
  emit();
  return id;
}

async function undoWith(id: string, undo: () => Promise<unknown>) {
  toast({ id, tone: "busy", text: "Undoing…" });
  try {
    await undo();
    toast({ id, text: "Undone." });
  } catch (e) {
    toast({ id, tone: "error", text: `Could not undo: ${(e as Error).message}.` });
  }
}

/** Closes one toast, by the key of the toast shown, and forgets it once it has left. */
export function closeToast(key: number) {
  const t = items.find((x) => x.key === key);
  if (!t || !t.open) return;
  t.open = false;
  emit();
  setTimeout(() => {
    items = items.filter((x) => x.key !== key);
    emit();
  }, 200);
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useToasts(): ToastItem[] {
  return useSyncExternalStore(subscribe, () => items);
}
