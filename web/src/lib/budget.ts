// What the Budget screen's parts share about lines (ADR-0011, ADR-0012, ADR-0014).

import { send, type BudgetLine, type BudgetMonth, type Category, type NoteColor } from "@/lib/api";
import { formatMonth } from "@/lib/format";

/** A line shared by several categories, which goes by a name of its own. */
export const isShared = (l: BudgetLine) => l.byCategory.length > 1;

/** A pot without categories (the emergency fund, a goal): its money leaves only when moved out. */
export const isBare = (l: BudgetLine) => l.byCategory.length === 0;

/** What a line is called: a shared line's or a bare pot's name, else its one category's. */
export function lineName(l: BudgetLine, byId: Map<string, Category>): string {
  if (isShared(l)) return l.name ?? "Shared line";
  if (isBare(l)) return l.name ?? "Pot";
  return byId.get(l.byCategory[0]?.category ?? "")?.name ?? l.name ?? "Archived category";
}

/** The colours of a line's categories, in the order they spent. */
export function lineColors(l: BudgetLine, byId: Map<string, Category>): NoteColor[] {
  return l.byCategory.map((p) => byId.get(p.category)?.color ?? "note-200");
}

/** "October", for "2026-10". */
export const monthName = (m: string) => formatMonth(m).split(" ")[0];

/** A line as the API takes it, for PUT and POST. */
export type LineBody = { id?: string; name: string; categories: string[]; limitCents: number; rollover: boolean; fund: boolean; targetCents: number | null };

/** A line's body with some of it changed: the rest stays as it is. */
export function lineBody(l: BudgetLine, change: Partial<LineBody> = {}): LineBody {
  return {
    name: l.name ?? "",
    categories: l.byCategory.map((p) => p.category),
    limitCents: l.limitCents,
    rollover: l.rollover,
    fund: l.fund,
    targetCents: l.targetCents,
    ...change,
  };
}

/** Every line of a month as it is, ids, names and kinds included: what Undo puts back. */
export function snapshot(v: BudgetMonth): LineBody[] {
  return v.lines.map((l) => ({ ...lineBody(l), id: l.id }));
}

/** Puts a month's lines back as a snapshot had them. */
export function restore(month: string, lines: LineBody[]) {
  return send("PUT", `/api/v1/budget/${month}/limits`, { limits: lines });
}

/** Changes one line from a month on, keeping what it carried. */
export function saveLine(month: string, id: string, body: LineBody) {
  return send("PUT", `/api/v1/budget/${month}/lines/${id}`, body);
}
