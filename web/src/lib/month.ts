import { addMonths, monthOf, type Month } from "./format";
import { setQuery, useLocation } from "./router";

// The month every screen is looking at lives in the URL (?month=2026-10), so a
// reload or a shared link shows the same month. Months, never free ranges.

const MONTH_RE = /^\d{4}-(0[1-9]|1[0-2])$/;

export function useMonth(): {
  month: Month;
  current: Month;
  setMonth: (m: Month) => void;
  prev: () => void;
  next: () => void;
  isCurrent: boolean;
} {
  const { query } = useLocation();
  const current = monthOf(new Date());
  const asked = query.get("month");
  const month = asked && MONTH_RE.test(asked) && asked <= current ? asked : current;
  const setMonth = (m: Month) => setQuery("month", m === current ? null : m);
  return {
    month,
    current,
    setMonth,
    prev: () => setMonth(addMonths(month, -1)),
    next: () => setMonth(addMonths(month, 1) > current ? current : addMonths(month, 1)),
    isCurrent: month === current,
  };
}
