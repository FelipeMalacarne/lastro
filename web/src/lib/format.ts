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

/** "Today, 26 Oct" for today, "26 Oct" this year, "24 Oct 2025" in another. */
export function dayLabel(day: string, today: string): string {
  if (day === today) return `Today, ${formatDay(day)}`;
  return day.slice(0, 4) === today.slice(0, 4) ? formatDay(day) : `${formatDay(day)} ${day.slice(0, 4)}`;
}

/** Today in the browser's time zone, as "2026-10-26". */
export function todayISO(now = new Date()): string {
  return `${monthOf(now)}-${String(now.getDate()).padStart(2, "0")}`;
}

/** Today in São Paulo, where the server turns the days of what I type in: "2026-10-26". */
export function todaySaoPaulo(now = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" }).format(now);
}

/** The first day a transaction added by hand may carry, as the server checks it. */
export const FIRST_MANUAL_DAY = "2000-01-01";

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

/**
 * A number typed the Brazilian way, in hundredths: "1.234,56" → 123456,
 * "1.000" is a thousand, "12,5" → 1250; "1234.56" too. null for anything
 * else: a sign, letters, "0x10", "1e1", a third decimal.
 */
function parseHundredths(s: string): number | null {
  const t = s.replace(/\s/g, "");
  const m =
    /^(\d+|\d{1,3}(?:\.\d{3})+)(?:,(\d{1,2}))?$/.exec(t) ?? // 1.234,56 · 1.000 · 12,5
    /^(\d+)\.(\d{1,2})$/.exec(t); // 1234.56
  if (!m) return null;
  const n = Number(m[1].replace(/\./g, "")) * 100 + Number((m[2] ?? "").padEnd(2, "0"));
  return Number.isSafeInteger(n) ? n : null;
}

/** parseBRL("1.234,56") → 123456; also "1234.56", "R$ 50". null when it is not an amount. */
export function parseBRL(s: string): number | null {
  return parseHundredths(s.replace(/R\$/g, ""));
}

/** parsePercent("12,5") → 1250 basis points; "12,5%" too. null when it is not a percentage. */
export function parsePercent(s: string): number | null {
  return parseHundredths(s.replace(/%\s*$/, ""));
}

/** parseWhole("15") → 15. null for anything but digits. */
export function parseWhole(s: string): number | null {
  const t = s.trim();
  return /^\d{1,9}$/.test(t) ? Number(t) : null;
}

/** The one way a field says its amount is wrong: nothing while it is empty or readable (and, with aboveZero, more than zero). */
export function amountError(text: string, example = "3.000,00", aboveZero = false): string | undefined {
  const cents = parseBRL(text);
  return text.trim() && (cents === null || (aboveZero && cents === 0)) ? `Type an amount, like ${example}` : undefined;
}

/** amountError for a percentage. */
export function percentError(text: string, example = "12,5"): string | undefined {
  return text.trim() && parsePercent(text) === null ? `Type a percentage, like ${example}` : undefined;
}

/** 123456 → "1.234,56", for an amount field. */
export function inputBRL(cents: number): string {
  const p = splitBRL(cents);
  return p.units + p.cents;
}

/** "NETFLIX" → "Netflix", "PAO DE ACUCAR" → "Pao de Acucar": bank text read as a name. */
export function titleCase(s: string): string {
  return s
    .toLowerCase()
    .split(" ")
    .map((w, i) => (i > 0 && ["de", "da", "do", "e"].includes(w) ? w : w.charAt(0).toUpperCase() + w.slice(1)))
    .join(" ");
}

/** "up to 40 characters" → "Up to 40 characters.": a clause the server wrote, shown on its own. */
export function asSentence(s: string): string {
  const t = s.trim();
  return t.charAt(0).toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".");
}

/** Keeps "R$" with its number in prose the server wrote ("R$ 2.953,78" never breaks after R$). */
export function keepAmounts(s: string): string {
  return s.replace(/R\$ /g, "R$\u00a0");
}

/** formatCompact(150000000) → "1,5 mi"; 50000000 → "500 mil": axis labels on a phone. */
export function formatCompact(cents: number): string {
  const v = Math.round(cents / 100);
  const a = Math.abs(v);
  const sign = v < 0 ? MINUS : "";
  if (a >= 1_000_000) return `${sign}${(a / 1_000_000).toFixed(a >= 10_000_000 ? 0 : 1).replace(".", ",").replace(",0", "")} mi`;
  if (a >= 1_000) return `${sign}${Math.round(a / 1_000)} mil`;
  return sign + String(a);
}
