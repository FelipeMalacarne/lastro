import { useState, type ReactNode } from "react";
import { Check, ChevronDown, ChevronRight, ChevronUp, Clock, PiggyBank, ShieldCheck, TriangleAlert } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { GrowthChart } from "@/components/charts/GrowthChart";
import { PriceChart } from "@/components/charts/PriceChart";
import { Dividends, PriceAlert, Valuation, ValuationLine } from "@/components/PortfolioIncome";
import { Notice } from "@/components/Notice";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { Skeleton, SkeletonRows, SkeletonStat } from "@/components/ui/skeleton";
import { assetClasses, send, useResource, type AssetClass, type PortfolioOverview, type PositionGroup, type Reserve, type Thesis } from "@/lib/api";
import { formatBRL, formatDay, formatMonth, formatPercent, inputBRL, parseBRL } from "@/lib/format";
import { useWide } from "@/lib/media";
import { Link } from "@/lib/router";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";

export const classOf = (c: AssetClass) => assetClasses.find((x) => x.id === c)!;

/** "since 30 Sep 2025". */
function since(d: string | null) {
  if (!d) return "";
  const [y] = d.split("-");
  return `${formatDay(d)} ${y}`;
}

/** Portfolio (canvas: "Portfolio"). */
export function Portfolio() {
  const res = useResource<PortfolioOverview>("/api/v1/portfolio");
  const v = res.data;
  if (res.error && !v) return <PageHeader title="Portfolio" caption={`Could not load the portfolio: ${res.error}.`} monthless />;
  if (!v)
    return (
      <>
        <PageHeader title="Portfolio" monthless />
        <Panel>
          <div className="flex flex-col gap-4">
            <SkeletonStat hero />
            <Skeleton className="h-20" />
          </div>
        </Panel>
        <Panel title="Positions" bodyClassName="-mx-5">
          <SkeletonRows count={5} className="border-t border-rule" />
        </Panel>
      </>
    );

  const caption = [v.asOf ? `Prices as of ${formatDay(v.asOf)}` : null, `${v.positions} ${v.positions === 1 ? "position" : "positions"}${v.where?.length ? ` at ${v.where.join(" and ")}` : ""}`]
    .filter(Boolean)
    .join(" · ");

  if (v.positions === 0) {
    return (
      <>
        <PageHeader title="Portfolio" caption={caption} monthless />
        <Panel title="No investments yet">
          <p className="m-0 max-w-[60ch] type-body text-ink-muted">
            Investments come in with the banks' sync: CDBs, Tesouro Direto, stocks and ETFs held at a connected bank or broker. Once they arrive, this page shows them
            against your targets and the planner tells you where each contribution goes.
          </p>
        </Panel>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Portfolio"
        caption={caption}
        monthless
        actions={
          <>
            <Button asChild variant="ghost">
              <Link to="/settings#investments">Edit targets</Link>
            </Button>
            <Button asChild variant="primary">
              <Link to="/contribute">
                <PiggyBank className="size-4" aria-hidden /> Where to contribute
              </Link>
            </Button>
          </>
        }
      />
      {new Date().getMonth() === 0 && (
        <Notice tone="warning" title="January: review the targets." action={<Button asChild variant="ghost" size="sm"><Link to="/settings#investments">Review</Link></Button>}>
          Once a year, before the first contribution, check that the allocation still fits your plans.
        </Notice>
      )}
      <Hero v={v} />
      <Allocation v={v} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Growth v={v} />
        <Returns v={v} />
      </div>
      <Dividends />
      <Positions v={v} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-6">
        <Picks v={v} />
        <Scoreboard />
      </div>
    </>
  );
}

function Hero({ v }: { v: PortfolioOverview }) {
  const nw = v.netWorth;
  const rows: { label: string; caption: string; cents: number }[] = [
    { label: "Invested", caption: "The allocation below", cents: nw.investedCents },
    { label: "Emergency fund", caption: "Apart from the allocation", cents: nw.emergencyCents },
    { label: "Cash in accounts", caption: "Checking, savings and the wallet", cents: nw.cashCents },
    { label: "Card bills", caption: "Owed on the cards", cents: nw.cardsCents },
    ...(nw.otherCents ? [{ label: "Outside Open Finance", caption: "Balances you typed in", cents: nw.otherCents }] : []),
  ];
  const from = since(v.since);
  return (
    <Panel className="lg:p-6" footer="Net worth counts everything you own. The allocation below counts only invested money.">
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,420px)] lg:gap-10">
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-1">
            <span className="type-label text-ink-muted">Invested</span>
            <Amount cents={v.investedCents} size="hero" sign="none" />
            {v.monthDeltaCents !== null && (
              <span className={cn("type-caption", v.monthDeltaCents >= 0 ? "text-inflow" : "text-warning")}>
                {formatBRL(v.monthDeltaCents, { sign: "always" })} in returns since {v.monthFrom ? formatDay(v.monthFrom) : "a month ago"}
              </span>
            )}
          </div>
          {v.putIn12mCents !== null && (
            <div className="grid grid-cols-2 gap-4 border-t border-rule pt-4 sm:grid-cols-3">
              <Mini label={`Put in since ${from}`}>
                <Amount cents={v.putIn12mCents} sign="none" />
              </Mini>
              <Mini label="Returns">
                <Amount cents={v.returns12mCents ?? 0} sign="always" />
              </Mini>
              <Mini label="Return">
                <span className={cn("type-figure", (v.return12mBp ?? 0) >= 0 ? "text-inflow" : "text-ink")}>
                  {v.return12mBp !== null ? formatPercent(v.return12mBp, { sign: "always" }) : "—"}
                </span>
                {v.benchmarks.cdiBp !== null && <span className="type-caption text-ink-muted">CDI {formatPercent(v.benchmarks.cdiBp)}</span>}
              </Mini>
            </div>
          )}
        </div>
        <div className="flex flex-col gap-2">
          <span className="type-label text-ink-muted">Net worth</span>
          <div role="table" aria-label="Net worth" className="flex flex-col">
            {rows.map((r) => (
              <div key={r.label} role="row" className="grid min-h-11 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t border-rule py-1.5">
                <span role="cell" className="flex flex-wrap items-baseline gap-x-2">
                  <span className="text-[14px] leading-5">{r.label}</span>
                  <span className="type-caption text-ink-muted">{r.caption}</span>
                </span>
                <span role="cell">
                  <Amount cents={r.cents} />
                </span>
              </div>
            ))}
            <div role="row" className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-t-2 border-ink pt-2.5">
              <span role="cell" className="text-[14px] leading-5 font-semibold">
                Net worth
              </span>
              <span role="cell">
                <Amount cents={nw.totalCents} size="lg" sign="none" />
              </span>
            </div>
          </div>
        </div>
      </div>
    </Panel>
  );
}

function Mini({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <span className="type-caption text-ink-muted">{label}</span>
      {children}
    </div>
  );
}

function sentence(parts: string[]): string {
  if (parts.length <= 1) return parts.join("");
  return parts.slice(0, -1).join(", ") + " and " + parts.at(-1);
}

function Allocation({ v }: { v: PortfolioOverview }) {
  const above = v.classes.filter((c) => c.status === "above").map((c) => classOf(c.class).name);
  const below = v.classes.filter((c) => c.status === "below").map((c) => classOf(c.class).name);
  const title =
    above.length || below.length
      ? [above.length ? `${sentence(above)} ${above.length === 1 ? "is" : "are"} above ${above.length === 1 ? "its" : "their"} band` : "", below.length ? `${sentence(below)} ${below.length === 1 ? "is" : "are"} below ${below.length === 1 ? "its" : "their"} band` : ""]
          .filter(Boolean)
          .join(" and ")
      : "Every class is inside its band";
  const scale = Math.max(50, ...v.classes.map((c) => Math.ceil((Math.max(c.nowBp, c.targetBp + v.bandBp) / 100 + 5) / 10) * 10));
  const pct = (bp: number) => `${Math.min(bp / 100 / scale, 1) * 100}%`;
  const band = v.bandBp / 100;
  return (
    <Panel
      title={title}
      bodyClassName="-mx-5"
      footer={`Bands are ${band.toString().replace(".", ",")} points either side of each target. Rebalancing happens only through new contributions; lastro never suggests selling.`}
    >
      <div role="table" aria-label="Allocation against target">
        <div role="row" className="grid h-8 grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_56px] items-center gap-x-3 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted sm:grid-cols-[150px_120px_minmax(0,1fr)_64px_64px_minmax(0,180px)]">
          <span role="columnheader">Class</span>
          <span role="columnheader" className="hidden text-right sm:block">
            Value
          </span>
          <span role="columnheader" className="flex justify-between font-figure normal-case">
            <span>0%</span>
            <span>{scale / 2}%</span>
            <span>{scale}%</span>
          </span>
          <span role="columnheader" className="text-right">
            Now
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            Target
          </span>
          <span role="columnheader" className="hidden sm:block">
            Status
          </span>
        </div>
        {v.classes.map((c) => {
          const k = classOf(c.class);
          const diff = (c.nowBp - c.targetBp) / 100;
          return (
            <div key={c.class} role="row" className="grid min-h-14 grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_56px] items-center gap-x-3 border-b border-rule px-5 py-2 sm:grid-cols-[150px_120px_minmax(0,1fr)_64px_64px_minmax(0,180px)]">
              <span role="cell" className="flex flex-col gap-0.5">
                <CategoryTag name={k.name} color={k.color} />
                <span className="type-caption text-ink-muted sm:hidden">{formatBRL(c.valueCents)}</span>
              </span>
              <span role="cell" className="hidden text-right sm:block">
                <Amount cents={c.valueCents} sign="none" />
              </span>
              <span
                role="cell"
                aria-label={`${k.name}: ${formatPercent(c.nowBp)} now, target ${formatPercent(c.targetBp, { decimals: 0 })}, band ${formatPercent(c.targetBp - v.bandBp, { decimals: 0 })} to ${formatPercent(c.targetBp + v.bandBp, { decimals: 0 })}`}
                className="relative h-4"
              >
                <span className="absolute inset-y-1 left-0 right-0 rounded-xs bg-paper-sunken" />
                <span
                  className="absolute inset-y-0 rounded-xs border border-dashed border-ink-muted/60 bg-ink/5"
                  style={{ left: pct(Math.max(c.targetBp - v.bandBp, 0)), width: `calc(${pct(c.targetBp + v.bandBp)} - ${pct(Math.max(c.targetBp - v.bandBp, 0))})` }}
                />
                <span className="absolute inset-y-1 left-0 rounded-xs" style={{ width: pct(c.nowBp), background: `var(--${k.color})` }} />
                <span className="absolute -inset-y-0.5 w-0.5 bg-ink" style={{ left: pct(c.targetBp) }} />
              </span>
              <span role="cell" className="text-right type-figure">
                {formatPercent(c.nowBp)}
              </span>
              <span role="cell" className="hidden text-right type-figure text-ink-muted sm:block">
                {formatPercent(c.targetBp, { decimals: 0 })}
              </span>
              <span role="cell" className="col-span-3 flex flex-wrap items-center gap-x-2 gap-y-0.5 sm:col-span-1">
                {c.status === "in_band" ? (
                  <span className="type-caption text-ink-muted">In band</span>
                ) : (
                  <Badge tone="warning" icon={TriangleAlert}>
                    {c.status === "above" ? "Above band" : "Below band"}
                  </Badge>
                )}
                <span className="type-caption text-ink-muted">
                  {Math.abs(diff) < 0.05
                    ? "on target"
                    : `${Math.abs(diff).toFixed(1).replace(".", ",")} ${Math.abs(diff).toFixed(1) === "1.0" ? "point" : "points"} ${diff > 0 ? "over" : "under"}`}
                  {c.status === "below" ? ": gets new money first" : c.status === "above" ? ": no new money until it is back" : ""}
                </span>
              </span>
            </div>
          );
        })}
        <div className="flex flex-wrap gap-x-5 gap-y-1 px-5 pt-3 type-caption text-ink-muted">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-2.5 w-4 rounded-xs bg-note-100" aria-hidden />
            Now, in the class colour
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-3 w-0.5 bg-ink" aria-hidden />
            Target
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-3 w-4 rounded-xs border border-dashed border-ink-muted/60 bg-ink/5" aria-hidden />
            Band
          </span>
        </div>
      </div>
    </Panel>
  );
}

function Growth({ v }: { v: PortfolioOverview }) {
  const wide = useWide();
  const first = v.growth[0];
  const last = v.growth.at(-1);
  const growth = first && last ? last.valueCents - first.valueCents : 0;
  const putIn = first && last ? last.basisCents - first.basisCents : 0;
  const returns = growth - putIn;
  const share = growth > 0 ? Math.round((returns / growth) * 100) : null;
  const title =
    share === null
      ? "Growth of the invested total"
      : returns >= 0
        ? `Returns made ${share}% of the growth since ${first ? formatMonth(first.date.slice(0, 7), true) : ""}`
        : `Contributions carried the growth; returns took ${formatBRL(-returns)}`;
  return (
    <Panel title={title} footer="The invested total at the end of each month, split into what you put in (net of what you took out) and what it earned.">
      <div className="flex flex-col gap-3">
        <GrowthChart points={v.growth} width={wide ? 640 : 340} />
        <div className="flex flex-wrap gap-x-4 gap-y-1 type-caption text-ink-muted">
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-xs bg-rule-strong/60" aria-hidden />
            Put in since {first ? formatMonth(first.date.slice(0, 7), true) : ""}, {formatBRL(putIn)}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2.5 rounded-xs bg-inflow/40" aria-hidden />
            Returns, {formatBRL(returns, { sign: "always" })}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-0.5 w-4 bg-ink" aria-hidden />
            Invested total
          </span>
        </div>
      </div>
    </Panel>
  );
}

/** Real return: (1 + r) / (1 + inflation) − 1, in basis points. */
function real(bp: number, ipcaBp: number): number {
  return Math.round(((1 + bp / 10000) / (1 + ipcaBp / 10000) - 1) * 10000);
}

function Returns({ v }: { v: PortfolioOverview }) {
  const { cdiBp, ipcaBp } = v.benchmarks;
  const rows = v.classes.filter((c) => c.return12mBp !== null);
  const beatIPCA = ipcaBp !== null && rows.every((c) => c.return12mBp! > ipcaBp);
  const beatCDI = cdiBp !== null ? rows.filter((c) => c.return12mBp! > cdiBp).map((c) => classOf(c.class).name) : [];
  const title =
    rows.length === 0 || cdiBp === null
      ? "Return per class"
      : `${beatIPCA ? "Every class beat inflation" : "Not every class beat inflation"}; ${beatCDI.length === 0 ? "none beat CDI" : beatCDI.length === rows.length ? "all beat CDI" : `only ${sentence(beatCDI)} beat CDI`}`;
  const pp = (bp: number) => `${bp >= 0 ? "+" : "−"}${(Math.abs(bp) / 100).toFixed(1).replace(".", ",")} pp`;
  return (
    <Panel
      title={title}
      bodyClassName="-mx-5"
      footer={`Since ${since(v.benchmarks.from)}, with money put in or taken out counted for half the time. CDI and IPCA from Banco Central (SGS)${v.benchmarks.ipcaUntil ? `, IPCA through ${formatMonth(v.benchmarks.ipcaUntil)}` : ""}.`}
    >
      <div role="table" aria-label="Return per class">
        <div role="row" className="grid h-8 grid-cols-[minmax(0,1fr)_72px_80px] sm:grid-cols-[minmax(0,1fr)_72px_72px_80px] items-center gap-x-3 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted">
          <span role="columnheader">Class</span>
          <span role="columnheader" className="text-right">
            Return
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            vs CDI
          </span>
          <span role="columnheader" className="text-right">
            Over IPCA
          </span>
        </div>
        {rows.map((c) => {
          const k = classOf(c.class);
          const r = c.return12mBp!;
          return (
            <div key={c.class} role="row" className="grid min-h-11 grid-cols-[minmax(0,1fr)_72px_80px] sm:grid-cols-[minmax(0,1fr)_72px_72px_80px] items-center gap-x-3 border-b border-rule px-5">
              <span role="cell">
                <CategoryTag name={k.name} color={k.color} />
              </span>
              <span role="cell" className={cn("text-right type-figure", r < 0 && "text-ink")}>
                {formatPercent(r, { sign: "always" })}
              </span>
              <span role="cell" className={cn("hidden text-right type-figure-sm sm:block", cdiBp !== null && r >= cdiBp ? "text-inflow" : "text-ink-muted")}>
                {cdiBp !== null ? pp(r - cdiBp) : "—"}
              </span>
              <span role="cell" className={cn("text-right type-figure-sm", ipcaBp !== null && real(r, ipcaBp) < 0 ? "text-ink" : "text-ink-muted")}>
                {ipcaBp !== null ? formatPercent(real(r, ipcaBp), { sign: "always" }) : "—"}
              </span>
            </div>
          );
        })}
        {[
          { name: "CDI", bp: cdiBp },
          { name: "IPCA", bp: ipcaBp },
        ].map((b) => (
          <div key={b.name} role="row" className="grid min-h-10 grid-cols-[minmax(0,1fr)_72px_80px] sm:grid-cols-[minmax(0,1fr)_72px_72px_80px] items-center gap-x-3 border-b border-rule bg-paper px-5">
            <span role="cell" className="text-[14px] text-ink-muted">
              {b.name}
            </span>
            <span role="cell" className="text-right type-figure text-ink-muted">
              {b.bp !== null ? formatPercent(b.bp) : "—"}
            </span>
            <span role="cell" className="hidden sm:block" />
            <span role="cell" />
          </div>
        ))}
      </div>
    </Panel>
  );
}

/** "191.621611" → "191,62"; whole numbers stay whole. */
function formatQuantity(q: string): string {
  const n = Number(q);
  return Number.isInteger(n) ? String(n) : n.toFixed(2).replace(".", ",");
}

/** "2028-05-26" → "26 May 2028". */
function fullDay(d: string): string {
  return `${formatDay(d)} ${d.slice(0, 4)}`;
}

/** "2028-05-26" → "May 2028". */
function monthYear(d: string): string {
  return `${formatDay(d).split(" ")[1]} ${d.slice(0, 4)}`;
}

function groupDetail(g: PositionGroup): string {
  const parts: string[] = [];
  if (g.ticker && g.ticker !== g.name) parts.push(g.ticker);
  if (g.indexer === "CDI" || g.indexer === "SELIC") parts.push(`${formatPercent(g.rateBp, { decimals: 0 })} of ${g.indexer === "CDI" ? "CDI" : "Selic"}`);
  if (g.indexer === "IPCA") parts.push(`IPCA + ${formatPercent(g.rateBp)}`);
  if (g.indexer === "PRE") parts.push(`${formatPercent(g.rateBp)} a year`);
  if (g.lots.length > 1) parts.push(`${g.lots.length} ${g.ticker ? "lots" : "contributions"}`);
  if (g.firstMaturity && g.lastMaturity) {
    parts.push(g.firstMaturity === g.lastMaturity ? `until ${fullDay(g.firstMaturity)}` : `until ${monthYear(g.firstMaturity)} to ${monthYear(g.lastMaturity)}`);
  }
  if (g.priced) parts.push(`priced ${formatDay(g.asOf)}`);
  return parts.join(" · ");
}

/** A group's part in a section: what of it counts there, and that part's cost. */
type Part = { g: PositionGroup; cents: number; basis: number };

function partOf(g: PositionGroup, cents: number): Part {
  return { g, cents, basis: g.valueCents > 0 ? Math.round((g.basisCents * cents) / g.valueCents) : 0 };
}

const positionCols = "grid-cols-[minmax(0,1fr)_auto] sm:grid-cols-[minmax(0,1fr)_140px_110px_128px_72px]";

function Positions({ v }: { v: PortfolioOverview }) {
  // The opened row: a group can show in its class and in the emergency fund.
  const [open, setOpen] = useState<string | null>(null);
  const sections = [
    ...assetClasses.map((k) => ({
      key: k.id as string,
      name: k.name,
      color: k.color,
      parts: v.groups.filter((g) => g.class === k.id && g.valueCents > g.emergencyCents).map((g) => partOf(g, g.valueCents - g.emergencyCents)),
    })),
    { key: "emergency", name: "Emergency fund", color: "note-200" as const, parts: v.groups.filter((g) => g.emergencyCents > 0 || g.shortCents > 0).map((g) => partOf(g, g.emergencyCents)) },
  ].filter((s) => s.parts.length > 0);
  return (
    <Panel
      title="Positions"
      bodyClassName="-mx-5"
      footer="Quantities and values from the banks through Meu Pluggy; stocks and ETFs at the latest close when it is newer. Lots from the same bank, issuer, indexer and rate are one row; open it to see each contribution. “Since bought” is the value over what it cost. The emergency fund is the amount you set, taken from its group; the rest of the group counts in its class."
    >
      <div role="table" aria-label="Positions by class">
        <div role="row" className={cn("grid h-8 items-center gap-x-4 border-y border-rule bg-paper-sunken px-5 type-label text-ink-muted", positionCols)}>
          <span role="columnheader">Asset</span>
          <span role="columnheader" className="hidden sm:block">
            Where
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            Quantity
          </span>
          <span role="columnheader" className="text-right">
            Value
          </span>
          <span role="columnheader" className="hidden text-right sm:block">
            Since bought
          </span>
        </div>
        {sections.map((s) => {
          const total = s.parts.reduce((t, p) => t + p.cents, 0);
          const basis = s.parts.reduce((t, p) => t + p.basis, 0);
          return (
            <div key={s.key} role="rowgroup">
              <div role="row" className={cn("grid min-h-10 items-center gap-x-4 border-b border-rule bg-paper px-5", positionCols)}>
                <span role="cell" className="flex items-center gap-2">
                  {s.key === "emergency" ? (
                    <span className="inline-flex items-center gap-1.5 text-[14px] font-semibold">
                      <ShieldCheck className="size-4 text-ink-muted" aria-hidden /> Emergency fund
                    </span>
                  ) : (
                    <CategoryTag name={s.name} color={s.color} />
                  )}
                  <span className="type-caption text-ink-muted">
                    {s.key === "emergency" ? "not in the allocation" : `${v.investedCents ? Math.round((total / v.investedCents) * 100) : 0}% of invested`}
                  </span>
                </span>
                <span role="cell" className="hidden sm:block" />
                <span role="cell" className="hidden sm:block" />
                <span role="cell" className="text-right">
                  <Amount cents={total} sign="none" tone="muted" />
                </span>
                <span role="cell" className="hidden text-right type-figure-sm text-ink-muted sm:block">
                  {basis > 0 ? formatPercent(((total - basis) * 10000) / basis, { sign: "always" }) : ""}
                </span>
              </div>
              {s.parts.map((p) => {
                const key = `${s.key}:${p.g.id}`;
                return (
                  <GroupRow key={key} p={p} emergency={s.key === "emergency"} open={open === key} onToggle={() => setOpen(open === key ? null : key)} onDone={() => setOpen(null)} />
                );
              })}
            </div>
          );
        })}
      </div>
    </Panel>
  );
}

function GroupRow({ p, emergency, open, onToggle, onDone }: { p: Part; emergency: boolean; open: boolean; onToggle: () => void; onDone: () => void }) {
  const g = p.g;
  const many = g.lots.length > 1;
  const split = g.emergencyCents > 0 && g.emergencyCents < g.valueCents;
  let caption = groupDetail(g) || g.kind;
  if (emergency && g.reserve === "amount") caption = `${formatBRL(g.emergencyCents)} of ${formatBRL(g.valueCents)}${caption ? ` · ${caption}` : ""}`;
  else if (!emergency && split) caption = `${caption} · ${formatBRL(g.emergencyCents)} of it is the emergency fund`;
  const quantity = g.ticker ? formatQuantity(String(g.lots.reduce((t, l) => t + Number(l.quantity), 0))) : "";
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <div>
      <button
        type="button"
        role="row"
        aria-expanded={open}
        onClick={onToggle}
        className={cn(
          "grid min-h-14 w-full cursor-pointer items-center gap-x-4 border-0 border-b border-rule px-5 py-2 text-left focus-visible:focus-ring",
          positionCols,
          open ? "bg-tinta-soft" : "bg-paper-raised hover:bg-paper-sunken",
        )}
      >
        <span role="cell" className="flex min-w-0 items-center gap-2 pl-4">
          {many ? <Chevron className="size-4 shrink-0 text-ink-muted" aria-hidden /> : <span className="w-4 shrink-0" aria-hidden />}
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="truncate text-[14px] leading-5 font-semibold">{g.name}</span>
            <span className="type-caption text-ink-muted sm:truncate">
              {caption}
              <span className="sm:hidden"> · {g.institution}</span>
            </span>
            {emergency && g.shortCents > 0 && (
              <span className="inline-flex items-center gap-1 type-caption text-warning">
                <TriangleAlert className="size-3.5" aria-hidden /> {formatBRL(g.shortCents)} short of {formatBRL(g.reserveCents)}
              </span>
            )}
          </span>
        </span>
        <span role="cell" className="hidden truncate text-[14px] text-ink-muted sm:block">
          {g.institution}
        </span>
        <span role="cell" className="hidden text-right type-figure-sm text-ink-muted sm:block">
          {quantity}
        </span>
        <span role="cell" className="flex flex-col items-end">
          <Amount cents={p.cents} sign="none" />
          {!emergency && split && <span className="type-figure-sm text-ink-muted">of {formatBRL(g.valueCents)}</span>}
          {g.returnBp !== null && <span className="type-figure-sm text-ink-muted sm:hidden">{formatPercent(g.returnBp, { sign: "always" })}</span>}
        </span>
        <span role="cell" className={cn("hidden text-right type-figure-sm sm:block", g.returnBp !== null && g.returnBp < 0 ? "text-ink" : "text-ink-muted")}>
          {g.returnBp !== null ? formatPercent(g.returnBp, { sign: "always" }) : "—"}
        </span>
      </button>
      {open && <GroupEditor g={g} onDone={onDone} />}
    </div>
  );
}

function GroupEditor({ g, onDone }: { g: PositionGroup; onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState(g.reserve);
  const [amount, setAmount] = useState(g.reserveCents ? inputBRL(g.reserveCents) : "");
  const cents = parseBRL(amount);
  const rest = mode === "amount" && cents ? Math.max(g.valueCents - cents, 0) : null;
  async function save(body: { class?: string; reserve?: Reserve; reserveCents?: number }) {
    setBusy(true);
    setError(null);
    try {
      await send("PATCH", `/api/v1/position-groups/${encodeURIComponent(g.id)}`, body);
      onDone();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const many = g.lots.length > 1;
  return (
    <div className="flex flex-col gap-5 border-b border-rule bg-paper py-4 pr-5 pl-5 shadow-[inset_3px_0_0_var(--tinta)] sm:pl-14">
      <div className="flex flex-col gap-2">
        <span className="type-label text-ink-muted">Counts in</span>
        <div role="radiogroup" aria-label="Class" className="flex flex-wrap gap-1.5">
          {assetClasses.map((k) => {
            const on = g.reserve !== "all" && g.class === k.id;
            return (
              <button
                key={k.id}
                type="button"
                role="radio"
                aria-checked={on}
                disabled={busy}
                onClick={() => void save({ class: k.id === g.autoClass ? "" : k.id })}
                className={cn(
                  "flex h-9 cursor-pointer items-center gap-2 rounded-sm border px-2.5 focus-visible:focus-ring",
                  on ? "border-tinta bg-tinta-soft" : "border-rule bg-paper-raised hover:bg-paper-sunken",
                )}
              >
                <CategoryTag name={k.name} color={k.color} />
                {k.id === g.autoClass && <span className="type-caption text-ink-muted">automatic</span>}
              </button>
            );
          })}
        </div>
        <span className="type-caption text-ink-muted">
          {many ? `Applies to all ${g.lots.length} ${g.ticker ? "lots" : "contributions"}, and to the next ones that join this group.` : "Applies to the next lots of this product too."}
        </span>
      </div>

      <form
        className="flex flex-col gap-2 border-t border-rule pt-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (cents) void save({ reserve: "amount", reserveCents: cents });
        }}
      >
        <span className="inline-flex items-center gap-1.5 type-label text-ink-muted">
          <ShieldCheck className="size-3.5" aria-hidden /> Emergency fund
        </span>
        <SegmentedControl<Reserve>
          label="Emergency fund"
          className="self-start"
          options={[
            { value: "none", label: "None" },
            { value: "amount", label: "An amount" },
            { value: "all", label: "All of it" },
          ]}
          value={mode}
          onChange={(m) => {
            setMode(m);
            if (m !== "amount") void save({ reserve: m });
          }}
        />
        {mode === "amount" && (
          <div className="flex flex-wrap items-end gap-x-3 gap-y-2">
            <TextField
              label="Set apart from this group"
              numeric
              prefix="R$"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              error={amount && !cents ? "An amount, like 12.000,00" : undefined}
              className="w-[200px]"
            />
            <Button type="submit" variant="primary" size="sm" disabled={busy || !cents}>
              Save
            </Button>
          </div>
        )}
        {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
        <span className="max-w-[70ch] type-caption text-ink-muted">
          The emergency fund is shown apart and never gets a share of a contribution.
          {rest !== null && ` The other ${formatBRL(rest)} counts in ${classOf(g.class).name}.`}
          {mode === "amount" && " When the group holds less than this, all of it is the emergency fund and the panel says how much is missing."}
        </span>
      </form>

      {many && <Lots g={g} />}
      {g.ticker && (g.kind === "stock" || g.kind === "bdr" || g.kind === "fii") && <Valuation ticker={g.ticker} />}
      {g.ticker && <PriceAlert ticker={g.ticker} />}
      {g.ticker && <Prices ticker={g.ticker} />}
    </div>
  );
}

/** Each lot of a group, five at first. */
function Lots({ g }: { g: PositionGroup }) {
  const [all, setAll] = useState(false);
  const shown = all ? g.lots : g.lots.slice(0, 5);
  return (
    <div className="flex flex-col border-t border-rule pt-4">
      <span className="pb-2 type-label text-ink-muted">{g.ticker ? "Lots" : "Contributions"}</span>
      <div role="table" aria-label={g.ticker ? "Lots" : "Contributions"}>
        {shown.map((l) => (
          <div key={l.id} role="row" className="grid min-h-9 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-x-3 border-b border-rule py-1 sm:grid-cols-[minmax(0,1fr)_128px_72px] sm:gap-x-4">
            <span role="cell" className="type-caption text-ink-muted">
              {l.maturity ? `Until ${fullDay(l.maturity)}` : g.ticker ? `${formatQuantity(l.quantity)} units` : `As of ${fullDay(l.asOf)}`}
            </span>
            <span role="cell" className="text-right">
              <Amount cents={l.valueCents} sign="none" size="sm" />
            </span>
            <span role="cell" className="text-right type-figure-sm text-ink-muted">
              {l.returnBp !== null ? formatPercent(l.returnBp, { sign: "always" }) : "—"}
            </span>
          </div>
        ))}
      </div>
      {g.lots.length > 5 && (
        <Button variant="ghost" size="sm" className="mt-2 self-start" onClick={() => setAll(!all)}>
          {all ? "Show fewer" : `Show ${g.lots.length - 5} more`}
          {all ? <ChevronUp aria-hidden /> : <ChevronDown aria-hidden />}
        </Button>
      )}
    </div>
  );
}

/** A year of a ticker's closes, from the market data (F8). */
function Prices({ ticker }: { ticker: string }) {
  const wide = useWide();
  const res = useResource<{ closes: { date: string; closeCents: number }[] }>(`/api/v1/quotes/${encodeURIComponent(ticker)}?months=12`);
  if (!res.data) return null;
  if (res.data.closes.length < 2)
    return (
      <p className="m-0 border-t border-rule pt-3 type-caption text-ink-muted">
        No price history for {ticker} yet. The next sync reads it; brapi answers most tickers only with LASTRO_BRAPI_TOKEN set.
      </p>
    );
  return (
    <div className="border-t border-rule pt-3">
      <PriceChart ticker={ticker} closes={res.data.closes} width={wide ? 900 : 360} />
    </div>
  );
}

function Picks({ v }: { v: PortfolioOverview }) {
  const [editing, setEditing] = useState<string | null>(null);
  const stocks = v.groups.filter((h) => h.ticker && (h.kind === "stock" || h.kind === "bdr"));
  const withThesis = new Set(v.theses.map((t) => t.ticker));
  const missing = stocks.filter((h) => !withThesis.has(h.ticker!));
  const soon = v.theses.filter((t) => t.held && t.reportInDays !== null && t.reportInDays <= 7).sort((a, b) => a.reportInDays! - b.reportInDays!);
  const title = soon.length ? `Stock picks: ${soon[0].ticker} reports ${soon[0].reportInDays === 0 ? "today" : soon[0].reportInDays === 1 ? "tomorrow" : `in ${soon[0].reportInDays} days`}` : "Stock picks";
  const names = new Map(stocks.map((h) => [h.ticker!, h.name]));
  if (stocks.length === 0 && v.theses.length === 0) return null;
  return (
    <Panel title={title} footer="Each pick keeps its thesis and what would break it. Read them before each earnings report, not after.">
      <div className="grid gap-4 md:grid-cols-2">
        {v.theses.map((t) =>
          editing === t.ticker ? (
            <ThesisForm key={t.ticker} ticker={t.ticker} thesis={t} onDone={() => setEditing(null)} />
          ) : (
            <ThesisCard key={t.ticker} t={t} name={names.get(t.ticker)} onEdit={() => setEditing(t.ticker)} />
          ),
        )}
        {missing.map((h) =>
          editing === h.ticker ? (
            <ThesisForm key={h.ticker} ticker={h.ticker!} onDone={() => setEditing(null)} />
          ) : (
            <article key={h.ticker} className="flex flex-col items-start gap-2 rounded-md border border-dashed border-rule-strong p-4">
              <span className="flex items-baseline gap-2">
                <span className="font-figure text-[16px] font-medium">{h.ticker}</span>
                <span className="type-caption text-ink-muted">{h.name}</span>
              </span>
              <p className="m-0 type-caption text-ink-muted">No thesis yet. A pick without one is a guess.</p>
              <Button size="sm" onClick={() => setEditing(h.ticker!)}>
                Write the thesis
              </Button>
            </article>
          ),
        )}
      </div>
    </Panel>
  );
}

function ThesisCard({ t, name, onEdit }: { t: Thesis; name?: string; onEdit: () => void }) {
  const soon = t.reportInDays !== null && t.reportInDays <= 7;
  return (
    <article className={cn("flex flex-col gap-3 rounded-md border p-4", soon ? "border-warning bg-warning-soft/40" : "border-rule")}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-figure text-[16px] font-medium">{t.ticker}</span>
        {name && <span className="type-caption text-ink-muted">{name}</span>}
        {t.nextReport && (
          <Badge tone={soon ? "warning" : "neutral"} icon={Clock}>
            {t.reportInDays === null ? `reported ${formatDay(t.nextReport)}` : t.reportInDays === 0 ? "reports today" : `reports ${formatDay(t.nextReport)}`}
          </Badge>
        )}
        <span className="ml-auto">{t.held ? <Amount cents={t.valueCents} sign="none" size="sm" /> : <Badge>Not held</Badge>}</span>
      </div>
      {t.held && <ValuationLine ticker={t.ticker} />}
      <dl className="m-0 grid grid-cols-[64px_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-[14px] leading-5">
        <dt className="type-label text-ink-muted">Thesis</dt>
        <dd className="m-0">{t.thesis}</dd>
        <dt className="type-label text-ink-muted">Breaks</dt>
        <dd className="m-0">{t.breaks}</dd>
      </dl>
      <Button variant="ghost" size="sm" className="self-start" onClick={onEdit}>
        Edit
      </Button>
    </article>
  );
}

function ThesisForm({ ticker, thesis, onDone }: { ticker: string; thesis?: Thesis; onDone: () => void }) {
  const [text, setText] = useState(thesis?.thesis ?? "");
  const [breaks, setBreaks] = useState(thesis?.breaks ?? "");
  const [next, setNext] = useState(thesis?.nextReport ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function run(f: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await f();
      onDone();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form
      className="flex flex-col gap-3 rounded-md border border-tinta p-4"
      onSubmit={(e) => {
        e.preventDefault();
        void run(() => send("PUT", `/api/v1/theses/${ticker}`, { thesis: text, breaks, nextReport: next || null }));
      }}
    >
      <span className="font-figure text-[16px] font-medium">{ticker}</span>
      <TextField label="Thesis" value={text} onChange={(e) => setText(e.target.value)} placeholder="Why you hold it, in a line" maxLength={280} />
      <TextField label="What would break it" value={breaks} onChange={(e) => setBreaks(e.target.value)} placeholder="The fact that would make you sell" maxLength={280} />
      <TextField label="Next earnings report" type="date" value={next} onChange={(e) => setNext(e.target.value)} hint="A reminder shows a week before, with the thesis." />
      {error && <p className="m-0 type-caption text-danger">Could not save: {error}.</p>}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" size="sm" disabled={busy || !text.trim() || !breaks.trim()}>
          <Check className="size-4" aria-hidden /> Save
        </Button>
        <Button type="button" variant="ghost" size="sm" onClick={onDone}>
          Cancel
        </Button>
        {thesis && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-auto"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                const back = { thesis: thesis.thesis, breaks: thesis.breaks, nextReport: thesis.nextReport || null };
                await send("DELETE", `/api/v1/theses/${ticker}`);
                toast({ id: "thesis", text: `The ${ticker} thesis is gone.`, undo: () => send("PUT", `/api/v1/theses/${ticker}`, back) });
              })
            }
          >
            Delete
          </Button>
        )}
      </div>
    </form>
  );
}

type Score = { year: number; from: string; to: string; partial: boolean; satelliteBp: number | null; ibovespaBp: number | null; cdiBp: number | null };

/** The stock picks against the Ibovespa and CDI, year by year (F8). */
function Scoreboard() {
  const res = useResource<{ years: Score[]; ibovespa: boolean }>("/api/v1/scoreboard");
  const d = res.data;
  if (!d || d.years.length === 0) return null;
  const latest = d.years.at(-1)!;
  const verdict = (y: Score) => {
    if (y.satelliteBp === null) return "";
    const vs = [
      y.ibovespaBp !== null ? (y.satelliteBp >= y.ibovespaBp ? "ahead of the Ibovespa" : "behind the Ibovespa") : null,
      y.cdiBp !== null ? (y.satelliteBp >= y.cdiBp ? "ahead of CDI" : "behind CDI") : null,
    ].filter(Boolean);
    return vs.join(" and ");
  };
  const v = verdict(latest);
  const title = v ? `${v.charAt(0).toUpperCase() + v.slice(1)} in ${latest.year}${latest.partial ? " so far" : ""}` : "Stock picks against the index";
  const all = d.years.flatMap((y) => [y.satelliteBp, y.ibovespaBp, y.cdiBp]).filter((x): x is number => x !== null);
  const max = Math.max(500, ...all.map(Math.abs));
  const bars: { key: "satelliteBp" | "ibovespaBp" | "cdiBp"; label: string; color: string }[] = [
    { key: "satelliteBp", label: "Your picks", color: "var(--note-1)" },
    { key: "ibovespaBp", label: "Ibovespa", color: "var(--ink-muted)" },
    { key: "cdiBp", label: "CDI", color: "var(--rule-strong)" },
  ];
  return (
    <Panel
      title={title}
      footer={`The Brazilian stocks class, with money put in counted half the year, against the Ibovespa's price and CDI.${d.ibovespa ? "" : " Ibovespa quotes need LASTRO_BRAPI_TOKEN."} In January, decide whether the satellite stays.`}
    >
      <div className="flex flex-col gap-4">
        {d.years.map((y) => (
          <div key={y.year} className="flex flex-col gap-1.5">
            <span className="flex items-baseline gap-2">
              <span className="font-figure text-[14px] font-medium">{y.year}</span>
              {y.partial && (
                <span className="type-caption text-ink-muted">
                  {formatDay(dayAfter(y.from))} to {formatDay(y.to)}
                </span>
              )}
            </span>
            {bars.map((b) => {
              const bp = y[b.key];
              return (
                <div key={b.key} className="grid grid-cols-[80px_minmax(0,1fr)_64px] items-center gap-2">
                  <span className="type-caption text-ink-muted">{b.label}</span>
                  <span className="relative h-3">
                    <span className="absolute inset-y-0 left-1/2 w-px bg-rule-strong" aria-hidden />
                    {bp !== null && (
                      <span
                        className="absolute inset-y-0.5 rounded-xs"
                        style={{
                          background: b.color,
                          left: bp >= 0 ? "50%" : `${50 - (Math.abs(bp) / max) * 50}%`,
                          width: `${(Math.abs(bp) / max) * 50}%`,
                        }}
                      />
                    )}
                  </span>
                  <span className="text-right type-figure-sm text-ink">
                    {bp === null ? "—" : formatPercent(bp, { sign: "always" })}
                  </span>
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </Panel>
  );
}

/** "2025-12-31" → "2026-01-01": a period measured from a valuation starts the day after it. */
function dayAfter(d: string): string {
  const t = new Date(d + "T12:00:00Z");
  t.setUTCDate(t.getUTCDate() + 1);
  return t.toISOString().slice(0, 10);
}
