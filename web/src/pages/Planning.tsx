import { useEffect, useState } from "react";

import { Amount } from "@/components/Amount";
import { CategoryTag } from "@/components/CategoryTag";
import { ProjectionChart } from "@/components/charts/ProjectionChart";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { useResource, type AssetClass, type BudgetMonth, type PortfolioOverview } from "@/lib/api";
import { formatBRL, formatPercent, inputBRL, monthOf, parseBRL } from "@/lib/format";
import { useWide } from "@/lib/media";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";
import { classOf } from "@/pages/Portfolio";

type Projection = {
  startCents: number;
  monthlyCents: number;
  months: number;
  targetCents: number;
  scenarios: { name: "pessimistic" | "base" | "optimistic"; rateBp: number; finalCents: number; yearlyCents: number[] }[];
  when: { name: string; rateBp: number; months: number | null }[];
  levers: { monthlyCents: number; months: number | null }[];
  mix: { class: AssetClass; weightBp: number; meanBp: number; stdevBp: number }[];
  monteCarlo: { paths: number; reachedBp: number; bands: { month: number; p10Cents: number; p50Cents: number; p90Cents: number }[] };
};

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** "Feb 2037", n months from now. */
function monthsFromNow(n: number): string {
  const d = new Date();
  d.setDate(1);
  d.setMonth(d.getMonth() + n);
  return `${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}

/** "in 10 years 4 months". */
function span(n: number): string {
  const y = Math.floor(n / 12);
  const m = n % 12;
  const parts = [y ? `${y} ${y === 1 ? "year" : "years"}` : "", m ? `${m} ${m === 1 ? "month" : "months"}` : ""].filter(Boolean);
  return parts.length ? `in ${parts.join(" ")}` : "now";
}

/** Rounds a projection to a thousand reais: these are estimates. */
const round = (cents: number) => Math.round(cents / 100000) * 100000;

/** Debounces a typed amount into cents. */
function useAmount(initial: string | null, fallback: number): [string, (s: string) => void, number | null] {
  const [text, setText] = useState<string | null>(initial);
  const shown = text ?? (fallback ? inputBRL(fallback) : "");
  const parsed = parseBRL(shown);
  const [cents, setCents] = useState<number | null>(parsed);
  useEffect(() => {
    const t = setTimeout(() => setCents(parsed), 300);
    return () => clearTimeout(t);
  }, [parsed]);
  return [shown, setText, cents];
}

/** Planning (canvas: "Planning"). F9: projections in today's money. */
export function Planning() {
  const budget = useResource<BudgetMonth>(`/api/v1/budget?month=${monthOf(new Date())}`).data;
  const overview = useResource<PortfolioOverview>("/api/v1/portfolio").data;
  const wide = useWide();
  const [monthly, setMonthly, monthlyCents] = useAmount(null, budget?.payYourselfFirst.plannedCents ?? 0);
  const [yearsText, setYears] = useState("15");
  const [target, setTarget, targetCents] = useAmount("1.000.000,00", 0);
  const [view, setView] = useState<"scenarios" | "simulation">("scenarios");
  const years = Math.max(1, Math.min(60, Number(yearsText) || 15));
  const url = monthlyCents !== null ? `/api/v1/planning?monthly=${monthlyCents}&years=${years}&target=${targetCents ?? 0}` : null;
  const res = useResource<Projection>(url);
  const p = res.data;

  const by = (n: string) => p?.scenarios.find((s) => s.name === n);
  const base = by("base");
  const low = by("pessimistic");
  const high = by("optimistic");
  const history = (overview?.growth ?? []).slice(0, -1).map((g) => g.valueCents);
  const baseWhen = p?.when.find((w) => w.name === "base");

  return (
    <>
      <PageHeader title="Planning" caption="Every value in today's money · invested money only, the emergency fund stays out" monthless />

      <Panel className="lg:p-6" footer="Estimates, not promises. Real returns are after inflation and before tax.">
        <div className="flex flex-col gap-6">
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
            <div className="flex flex-col gap-1">
              <span className="type-label text-ink-muted">In {years} {years === 1 ? "year" : "years"}, base scenario</span>
              {base ? <Amount cents={round(base.finalCents)} size="hero" sign="none" estimate /> : <span className="type-body text-ink-muted">…</span>}
              {low && high && (
                <span className="type-caption text-ink-muted">
                  {formatBRL(round(low.finalCents))} if returns are pessimistic, {formatBRL(round(high.finalCents))} if optimistic
                </span>
              )}
            </div>
            <div className="flex flex-wrap gap-3">
              <TextField
                label="Every month"
                prefix="R$"
                numeric
                inputMode="decimal"
                value={monthly}
                onChange={(e) => setMonthly(e.target.value)}
                className="w-[180px]"
                error={monthly && parseBRL(monthly) === null ? "An amount" : undefined}
              />
              <TextField label="For" suffix="years" numeric inputMode="numeric" value={yearsText} onChange={(e) => setYears(e.target.value)} className="w-[120px]" />
            </div>
          </div>

          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <SegmentedControl
                label="Show"
                value={view}
                onChange={setView}
                options={[
                  { value: "scenarios", label: "Scenarios" },
                  { value: "simulation", label: "Simulation" },
                ]}
              />
              {view === "simulation" && p && (
                <span className="type-caption text-ink-muted">8 in 10 of {p.monteCarlo.paths.toLocaleString("pt-BR")} simulated paths land in the darker band</span>
              )}
            </div>
            {p && base && low && high ? (
              <ProjectionChart
                history={history}
                low={low.yearlyCents}
                base={base.yearlyCents}
                high={high.yearlyCents}
                bands={view === "simulation" ? p.monteCarlo.bands.filter((b) => b.month % 12 === 0).map((b) => ({ p10: b.p10Cents, p90: b.p90Cents })) : undefined}
                startYear={new Date().getFullYear()}
                width={wide ? 1000 : 340}
              />
            ) : (
              <div className="h-[200px]" />
            )}
            <div className="flex flex-wrap gap-x-5 gap-y-1 type-caption text-ink-muted">
              <span className="inline-flex items-center gap-1.5">
                <span className="h-0.5 w-4 bg-ink" aria-hidden />
                Invested so far
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="w-4 border-t-2 border-dashed border-tinta" aria-hidden />
                Base, {base ? formatPercent(base.rateBp, { decimals: 0 }) : "4%"} a year
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="h-2.5 w-4 rounded-xs bg-tinta/15" aria-hidden />
                Pessimistic {low ? formatPercent(low.rateBp, { decimals: 0 }) : "2%"} to optimistic {high ? formatPercent(high.rateBp, { decimals: 0 }) : "6%"} a year
              </span>
              {view === "simulation" && (
                <span className="inline-flex items-center gap-1.5">
                  <span className="h-2.5 w-4 rounded-xs bg-tinta/25" aria-hidden />
                  Simulated, 10th to 90th percentile
                </span>
              )}
            </div>
          </div>

          {p && (
            <div className="flex flex-col gap-1.5 border-t border-rule pt-4">
              <span className="type-label text-ink-muted">Assumptions</span>
              <ul className="m-0 flex list-disc flex-col gap-0.5 pl-5 type-caption text-ink-muted">
                <li>Starts from {formatBRL(p.startCents)} invested today.</li>
                <li>{formatBRL(p.monthlyCents)} every month, raised with inflation.</li>
                <li>
                  Real return of {p.scenarios.map((s) => formatPercent(s.rateBp, { decimals: 0 })).join(", ")} a year
                  {view === "simulation" && "; the simulation draws each class's year from its long-run mean and spread, below"}.
                </li>
                <li>Nothing is withdrawn before the end.</li>
              </ul>
            </div>
          )}
        </div>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Panel
          title={
            baseWhen?.months != null && targetCents
              ? `${formatBRL(targetCents).replace(",00", "")} by ${monthsFromNow(baseWhen.months)} in the base scenario`
              : "How long until"
          }
          footer="Months are estimates in today's money."
        >
          <div className="grid gap-6 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="flex flex-col gap-3">
              <TextField label="How long until I have" prefix="R$" numeric inputMode="decimal" value={target} onChange={(e) => setTarget(e.target.value)} />
              <div role="table" aria-label="When the target is reached" className="flex flex-col">
                {p?.when.map((w) => (
                  <div key={w.name} role="row" className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 border-t border-rule py-2">
                    <span role="cell" className={cn("text-[14px] leading-5", w.name === "base" && "font-semibold text-tinta")}>
                      {w.name.charAt(0).toUpperCase() + w.name.slice(1)}, {formatPercent(w.rateBp, { decimals: 0 })} a year
                    </span>
                    <span role="cell" className={cn("text-right type-figure", w.name === "base" && "text-tinta")}>
                      {w.months === null ? "not within 100 years" : monthsFromNow(w.months)}
                    </span>
                    <span role="cell" className="col-span-2 type-caption text-ink-muted">
                      {w.months === null ? "" : span(w.months)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <div className="flex flex-col gap-3">
              {p && baseWhen?.months != null && p.levers.length > 0 && (
                <>
                  <p className="m-0 type-body">{leverInsight(p, baseWhen.months)}</p>
                  <div className="flex flex-col">
                    <span className="type-label text-ink-muted">If the monthly amount changes, base scenario</span>
                    {p.levers.map((l) => {
                      const d = l.months === null ? null : l.months - baseWhen.months!;
                      return (
                        <div key={l.monthlyCents} className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 border-t border-rule py-2">
                          <span className="text-[14px] leading-5">{formatBRL(l.monthlyCents)} a month</span>
                          <span className="text-right type-figure">{l.months === null ? "—" : monthsFromNow(l.months)}</span>
                          <span className={cn("col-span-2 type-caption", d !== null && d < 0 ? "text-inflow" : "text-ink-muted")}>
                            {d === null ? "" : d === 0 ? "the same month" : `${Math.abs(d)} ${Math.abs(d) === 1 ? "month" : "months"} ${d < 0 ? "sooner" : "later"}`}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </>
              )}
            </div>
          </div>
        </Panel>

        {p && <Simulation p={p} years={years} />}
      </div>
    </>
  );
}

/** Which moves the date more: R$ 500 a month, or two points of return? */
function leverInsight(p: Projection, base: number): string {
  const more = p.levers.find((l) => l.monthlyCents > p.monthlyCents)?.months;
  const opt = p.when.find((w) => w.name === "optimistic")?.months;
  if (more == null || opt == null) return "The monthly amount is the lever you control.";
  return base - more >= base - opt
    ? `R$ 500 more a month moves the date as much as two extra points of return you cannot control.`
    : `R$ 500 more a month brings it ${base - more} months closer; returns matter more here, but they are not yours to set.`;
}

function Simulation({ p, years }: { p: Projection; years: number }) {
  const last = p.monteCarlo.bands.at(-1);
  const reached = p.monteCarlo.reachedBp / 100;
  return (
    <Panel
      title={p.targetCents ? `${Math.round(reached)}% of simulated paths reach ${formatBRL(p.targetCents).replace(",00", "")} in ${years} years` : "Simulation"}
      footer={`${p.monteCarlo.paths.toLocaleString("pt-BR")} paths of monthly returns, each class drawn on its own from its long-run real mean and spread, rebalanced to the targets every month. Long-run figures, not forecasts.`}
    >
      <div className="flex flex-col gap-4">
        {last && (
          <div className="grid gap-2 border-b border-rule pb-4 sm:grid-cols-3 sm:gap-3">
            {[
              { label: "1 in 10 below", cents: last.p10Cents },
              { label: "Middle", cents: last.p50Cents },
              { label: "1 in 10 above", cents: last.p90Cents },
            ].map((x) => (
              <div key={x.label} className="flex items-baseline justify-between gap-2 sm:flex-col sm:items-start sm:gap-0.5">
                <span className="type-caption text-ink-muted">{x.label}</span>
                <Amount cents={round(x.cents)} sign="none" size="sm" estimate />
              </div>
            ))}
          </div>
        )}
        <div role="table" aria-label="What the simulation assumes per class" className="flex flex-col">
          <div role="row" className="grid h-8 grid-cols-[minmax(0,1fr)_64px_72px_72px] items-center gap-x-3 border-y border-rule type-label text-ink-muted">
            <span role="columnheader">Class</span>
            <span role="columnheader" className="text-right">
              Weight
            </span>
            <span role="columnheader" className="text-right">
              Real a year
            </span>
            <span role="columnheader" className="text-right">
              Spread
            </span>
          </div>
          {p.mix.map((m) => {
            const k = classOf(m.class);
            return (
              <div key={m.class} role="row" className="grid min-h-10 grid-cols-[minmax(0,1fr)_64px_72px_72px] items-center gap-x-3 border-b border-rule">
                <span role="cell">
                  <CategoryTag name={k.name} color={k.color} />
                </span>
                <span role="cell" className="text-right type-figure-sm">
                  {formatPercent(m.weightBp, { decimals: 0 })}
                </span>
                <span role="cell" className="text-right type-figure-sm">
                  {formatPercent(m.meanBp)}
                </span>
                <span role="cell" className="text-right type-figure-sm text-ink-muted">
                  ±{formatPercent(m.stdevBp)}
                </span>
              </div>
            );
          })}
        </div>
      </div>
    </Panel>
  );
}
