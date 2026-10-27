// Money is integer cents and percentages are basis points, as in the Go money
// kernel. Display follows the Brazilian convention: "R$ 1.234,56", "12,5%".

export const MINUS = "−";
const NBSP = " ";

export type Sign = "negative" | "always" | "none";

export function splitBRL(cents: number): { units: string; cents: string } {
  const abs = Math.abs(Math.round(cents || 0));
  const units = Math.floor(abs / 100);
  const rest = abs % 100;
  return {
    units: String(units).replace(/\B(?=(\d{3})+(?!\d))/g, "."),
    cents: "," + String(rest).padStart(2, "0"),
  };
}

export function signFor(value: number, sign: Sign = "negative"): string {
  if (sign === "none") return "";
  if (value < 0) return MINUS;
  if (value > 0 && sign === "always") return "+";
  return "";
}

/** formatBRL(-4890) → "−R$ 48,90"; formatBRL(1240000, { sign: "always" }) → "+R$ 12.400,00" */
export function formatBRL(cents: number, opts: { sign?: Sign } = {}): string {
  const p = splitBRL(cents);
  return signFor(cents, opts.sign) + "R$" + NBSP + p.units + p.cents;
}

/** formatPercent(1250) → "12,5%" */
export function formatPercent(bp: number, opts: { sign?: Sign; decimals?: number } = {}): string {
  const v = Math.abs(bp || 0) / 100;
  return signFor(bp, opts.sign) + v.toFixed(opts.decimals ?? 1).replace(".", ",") + "%";
}

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

/** A month as the API speaks it: "2026-10". */
export type Month = string;

export function monthOf(d: Date): Month {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

export function addMonths(m: Month, n: number): Month {
  const [y, mo] = m.split("-").map(Number);
  return monthOf(new Date(y, mo - 1 + n, 1));
}

/** "October 2026", or "Oct 2026" with short. */
export function formatMonth(m: Month, short = false): string {
  const [y, mo] = m.split("-").map(Number);
  const name = MONTHS[mo - 1] ?? "";
  return `${short ? name.slice(0, 3) : name} ${y}`;
}

/** A calendar day as the API speaks it ("2026-10-26") → "26 Oct". */
export function formatDay(day: string): string {
  const [, mo, d] = day.split("-").map(Number);
  return `${d} ${(MONTHS[mo - 1] ?? "").slice(0, 3)}`;
}

/** "Today, 26 Oct" for today, "26 Oct" otherwise. */
export function dayLabel(day: string, today: string): string {
  return day === today ? `Today, ${formatDay(day)}` : formatDay(day);
}

/** Today in the browser's time zone, as "2026-10-26". */
export function todayISO(now = new Date()): string {
  return `${monthOf(now)}-${String(now.getDate()).padStart(2, "0")}`;
}

const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

/** "2026-10-26" → "26 Oct 2026". */
export function formatDate(day: string): string {
  return `${formatDay(day)} ${day.slice(0, 4)}`;
}

/** "2026-10-22" → "Thursday, 22 Oct". */
export function formatWeekday(day: string): string {
  const [y, mo, d] = day.split("-").map(Number);
  return `${WEEKDAYS[new Date(y, mo - 1, d).getDay()]}, ${formatDay(day)}`;
}

/** An axis label in whole reais: 821430 → "8.214". */
export function formatAxis(cents: number): string {
  const v = Math.round(cents / 100);
  return (v < 0 ? MINUS : "") + String(Math.abs(v)).replace(/\B(?=(\d{3})+(?!\d))/g, ".");
}

/** A step for about n gridlines up to max, in cents: 1, 2, 2,5 or 5 times a power of ten. */
export function niceStep(max: number, n = 4): number {
  const raw = Math.max(max, 1) / n;
  const pow = 10 ** Math.floor(Math.log10(raw));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * pow >= raw) return m * pow;
  return 10 * pow;
}

/** parseBRL("1.234,56") → 123456; also "1234.56", "R$ 50". null when it is not an amount. */
export function parseBRL(s: string): number | null {
  let t = s.replace(/R\$|\s/g, "");
  if (!t) return null;
  if (/,\d{1,2}$/.test(t)) t = t.replace(/\./g, "").replace(",", ".");
  else if (/^\d{1,3}(\.\d{3})+$/.test(t)) t = t.replace(/\./g, "");
  if (!/^\d+(\.\d{1,2})?$/.test(t)) return null;
  return Math.round(Number(t) * 100);
}

/** 123456 → "1.234,56", for an amount field. */
export function inputBRL(cents: number): string {
  const p = splitBRL(cents);
  return p.units + p.cents;
}
