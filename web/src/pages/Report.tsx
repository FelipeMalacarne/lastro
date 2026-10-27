import { Amount } from "@/components/Amount";
import { CategoryTag } from "@/components/CategoryTag";
import { Panel } from "@/components/Panel";
import { useResource } from "@/lib/api";
import { useCategories } from "@/lib/categories";
import { formatBRL, formatMonth, formatPercent } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { Link } from "@/lib/router";
import { PageHeader } from "@/shell/AppShell";

type ReportView = {
  month: string;
  over: boolean;
  sentence: string;
  incomeCents: number;
  spentCents: number;
  contributionCents: number;
  savingsRateBp: number;
  prevSpentCents: number | null;
  spendingChangeBp: number | null;
  categories: { category: string; name: string; spentCents: number; availableCents: number }[];
  notes: string[];
  netWorthStartCents: number | null;
  netWorthEndCents: number | null;
  putInCents: number | null;
  dividendsCents: number;
};

/** A month in review (F10; canvas: "Month in review"). */
export function Report() {
  const { month, isCurrent } = useMonth();
  const r = useResource<ReportView>(`/api/v1/report?month=${month}`);
  const { byId } = useCategories();
  const d = r.data;
  const name = formatMonth(month);
  const prevName = formatMonth(prevOf(month)).split(" ")[0];
  return (
    <>
      <PageHeader
        title={`${name} in review`}
        caption={isCurrent ? "So far: the month is not over, so these numbers will still move" : "From the closed month · sent to Telegram from the 2nd of the next month, when set up"}
      />
      {r.error && !d && <p className="m-0 type-body text-ink-muted">Could not load the report: {r.error}.</p>}
      {!d && !r.error && <p className="m-0 type-body text-ink-muted">Loading…</p>}
      {d && (
        <>
          <Panel className="lg:p-6" footer="Kept: income minus everything that counts as spending, over income. Pay yourself first is part of what was kept.">
            <div className="flex flex-col gap-5">
              {d.sentence && <p className="m-0 max-w-[70ch] font-display text-[20px] leading-7 font-medium">{d.sentence}</p>}
              <div className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-4">
                <Figure label="Income" cents={d.incomeCents} sign="always" />
                <Figure
                  label="Spending"
                  cents={-d.spentCents}
                  sign="always"
                  vs={d.spendingChangeBp === null ? undefined : `${formatPercent(d.spendingChangeBp, { sign: "always" })} vs ${prevName}`}
                />
                <Figure label="Put aside" cents={d.contributionCents} vs={d.incomeCents ? `${formatPercent(d.savingsRateBp)} kept` : undefined} />
                {d.netWorthEndCents !== null && (
                  <Figure
                    label="Net worth"
                    cents={d.netWorthEndCents}
                    vs={d.netWorthStartCents !== null ? `${formatBRL(d.netWorthEndCents - d.netWorthStartCents, { sign: "always" })} in the month` : undefined}
                  />
                )}
              </div>
            </div>
          </Panel>

          <div className="flex flex-wrap items-start gap-4 lg:gap-6">
            <Panel title="Against the limits" className="min-w-0 flex-[7_1_460px]" footer="Spent over what was available, rollover included. The fullest first.">
              {d.categories.length === 0 ? (
                <p className="m-0 type-body text-ink-muted">
                  No limits this month. <Link to="/budget">Set them on the Budget</Link>.
                </p>
              ) : (
                <div className="flex flex-col gap-2.5">
                  {d.categories.map((c) => {
                    const cat = byId.get(c.category);
                    const over = c.spentCents > c.availableCents;
                    return (
                      <div key={c.category} className="grid grid-cols-[minmax(0,140px)_minmax(0,1fr)] items-center gap-x-3 gap-y-1 sm:grid-cols-[150px_minmax(0,1fr)_190px]">
                        <CategoryTag name={cat?.name ?? c.name} color={cat?.color} />
                        <span className="relative h-2.5 rounded-xs bg-paper-sunken">
                          <span
                            className="absolute inset-y-0 left-0 rounded-xs"
                            style={{ width: `${Math.min(100, c.availableCents > 0 ? (c.spentCents / c.availableCents) * 100 : 100)}%`, background: cat ? `var(--${cat.color})` : "var(--ink-muted)" }}
                          />
                        </span>
                        <span className={over ? "col-span-2 text-right type-figure-sm text-danger sm:col-span-1" : "col-span-2 text-right type-figure-sm sm:col-span-1"}>
                          {formatBRL(c.spentCents)} of {formatBRL(c.availableCents)}
                        </span>
                      </div>
                    );
                  })}
                </div>
              )}
            </Panel>

            <div className="flex min-w-0 flex-[5_1_340px] flex-col gap-4 lg:gap-6">
              <Panel title="Worth a look">
                {d.notes.length === 0 ? (
                  <p className="m-0 type-body text-ink-muted">Nothing unusual: no category over its limit, no new or changed charge, nothing waiting.</p>
                ) : (
                  <ul className="m-0 flex flex-col gap-2 pl-[18px] text-[14px] leading-5">
                    {d.notes.map((n) => (
                      <li key={n}>{n}</li>
                    ))}
                  </ul>
                )}
              </Panel>
              <Panel title={`Investments in ${name.split(" ")[0]}`}>
                <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-[14px] leading-5">
                  <dt className="text-ink-muted">Put in</dt>
                  <dd className="m-0 text-right font-figure">{d.putInCents === null ? "—" : formatBRL(d.putInCents)}</dd>
                  <dt className="text-ink-muted">Net worth change</dt>
                  <dd className="m-0 text-right font-figure">
                    {d.netWorthStartCents === null || d.netWorthEndCents === null ? "—" : formatBRL(d.netWorthEndCents - d.netWorthStartCents, { sign: "always" })}
                  </dd>
                  <dt className="text-ink-muted">Dividends paid</dt>
                  <dd className="m-0 text-right font-figure">{formatBRL(d.dividendsCents)}</dd>
                </dl>
              </Panel>
            </div>
          </div>
        </>
      )}
    </>
  );
}

function Figure({ label, cents, sign = "none", vs }: { label: string; cents: number; sign?: "none" | "always"; vs?: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="type-caption text-ink-muted">{label}</span>
      <Amount cents={cents} sign={sign} />
      {vs && <span className="type-caption text-ink-muted">{vs}</span>}
    </div>
  );
}

function prevOf(m: string): string {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(y, mo - 2, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}
