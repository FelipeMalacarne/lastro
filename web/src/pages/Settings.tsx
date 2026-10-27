import { useEffect, useState, type ReactNode } from "react";
import { Check, Clock, ExternalLink, TriangleAlert } from "lucide-react";

import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { SegmentedControl } from "@/components/SegmentedControl";
import { SyncStatus } from "@/components/SyncStatus";
import { TextField } from "@/components/TextField";
import { Button } from "@/components/ui/button";
import { assetClasses, send, typeName, useResource, type AssetClass, type BudgetMonth, type Category, type PortfolioOverview, type Rule } from "@/lib/api";
import { formatDay, formatMonth, formatPercent, inputBRL, monthOf, parseBRL } from "@/lib/format";
import { Link } from "@/lib/router";
import { setTheme, useTheme, type ThemePreference } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/shell/AppShell";
import { useBanks } from "@/shell/Accounts";

type ServerSettings = {
  version: string;
  env: string;
  signIn: string;
  model: { state: "off" | "online" | "missing" | "offline"; name: string };
  alerts: string;
};

const sections = [
  { id: "connections", label: "Connections" },
  { id: "budget", label: "Budget" },
  { id: "investments", label: "Investments" },
  { id: "categorization", label: "Categorization" },
  { id: "appearance", label: "Appearance" },
  { id: "server", label: "Server" },
];

const themes: { value: ThemePreference; label: string }[] = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
];

// The sync CronJob: minute 17 of every sixth hour, in Brasília time.
const syncHours = [0, 6, 12, 18];

/** The next "HH:17" the sync CronJob runs at, in Brasília time. */
export function nextSync(now = new Date()): string {
  const [h, m] = new Intl.DateTimeFormat("en-GB", { timeZone: "America/Sao_Paulo", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
    .format(now)
    .split(":")
    .map(Number);
  const next = syncHours.find((x) => x > h || (x === h && m < 17)) ?? syncHours[0];
  return `${String(next).padStart(2, "0")}:17`;
}

/** Settings (canvas: "Settings"). The Budget and Investments sections arrive with M3 and M4. */
export function Settings() {
  const [at, setAt] = useState("connections");
  const { banks } = useBanks();
  const warn = banks.some((b) => !b.connection || b.connection.health !== "ok" || b.connection.expiresSoon);

  // A link to a section (/settings#budget) opens on it.
  useEffect(() => {
    const id = location.hash.slice(1);
    if (id) requestAnimationFrame(() => document.getElementById(id)?.scrollIntoView({ block: "start" }));
  }, []);

  // The section nav marks the section at the top of the screen.
  useEffect(() => {
    const els = sections.map((s) => document.getElementById(s.id)).filter((e): e is HTMLElement => !!e);
    const io = new IntersectionObserver(
      (entries) => {
        const top = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0];
        if (top) setAt(top.target.id);
      },
      { rootMargin: "0px 0px -60% 0px" },
    );
    els.forEach((e) => io.observe(e));
    return () => io.disconnect();
  }, []);

  return (
    <>
      <PageHeader title="Settings" caption="Theme saves as you pick it; the other sections save with their button · server settings come from the LASTRO_* variables and are read-only here" monthless />
      <div className="flex items-start gap-8">
        <nav aria-label="Settings sections" className="sticky top-8 hidden w-[184px] flex-none flex-col gap-0.5 lg:flex">
          {sections.map((s) => {
            const on = s.id === at;
            return (
              <a
                key={s.id}
                href={`#${s.id}`}
                aria-current={on ? "location" : undefined}
                onClick={(e) => {
                  e.preventDefault();
                  document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth", block: "start" });
                  setAt(s.id);
                }}
                className={cn(
                  "flex h-9 items-center justify-between gap-2 rounded-sm px-2.5 text-[14px] leading-5 font-semibold no-underline",
                  on ? "bg-tinta-soft text-tinta shadow-[inset_2px_0_0_var(--tinta)] hover:text-tinta" : "text-ink hover:bg-paper-sunken hover:text-ink",
                )}
              >
                {s.label}
                {s.id === "connections" && warn && <TriangleAlert className="size-3.5 text-warning" aria-label="needs attention" />}
              </a>
            );
          })}
        </nav>
        <div className="flex min-w-0 max-w-[820px] flex-1 flex-col gap-4 lg:gap-6">
          <Connections />
          <BudgetSettings />
          <Investments />
          <Categorization />
          <Appearance />
          <Server />
        </div>
      </div>
    </>
  );
}

function Section({ id, title, intro, action, children }: { id: string; title: ReactNode; intro?: ReactNode; action?: ReactNode; children: ReactNode }) {
  return (
    <section id={id} aria-labelledby={`s-${id}`} className="flex scroll-mt-6 flex-col gap-4 rounded-md border border-rule bg-paper-raised p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h2 id={`s-${id}`} className="m-0 flex items-center gap-2 type-heading">
            {title}
          </h2>
          {intro && <p className="m-0 max-w-[60ch] type-caption text-ink-muted">{intro}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

/** A setting: what it is and why on the left, its value or control on the right. */
function Row({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="grid gap-x-6 gap-y-2 border-t border-rule pt-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="text-[14px] leading-5 font-semibold">{label}</span>
        {hint && <span className="type-caption text-ink-muted">{hint}</span>}
      </span>
      <div className="flex flex-wrap items-center gap-2 sm:justify-end">{children}</div>
    </div>
  );
}

function Connections() {
  const { banks, loading, error } = useBanks();
  return (
    <Section id="connections" title="Connections" intro="Your banks and brokers, read through Meu Pluggy. lastro only reads; it never moves money.">
      {error && <p className="m-0 type-body text-ink-muted">Could not load the connections: {error}.</p>}
      {loading && banks.length === 0 && <p className="m-0 type-body text-ink-muted">Loading…</p>}
      {!loading && banks.length === 0 && !error && <p className="m-0 type-body text-ink-muted">No connections yet. The first sync brings them in.</p>}
      <ul role="list" className="m-0 flex list-none flex-col p-0">
        {banks.map((b) => {
          const c = b.connection;
          const state = c?.health ?? "stale";
          const attention = state === "stale" || state === "reauth";
          return (
            <li
              key={c?.itemId ?? b.institution}
              className="grid gap-x-4 gap-y-2 border-t border-rule py-3.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,200px)_auto] sm:items-center"
            >
              <SyncStatus institution={b.institution} state={state} detail={c?.detail ?? "Not synced by Pluggy: seeded or imported by hand"} />
              <span className="pl-10 type-caption text-ink-muted sm:pl-0">
                {b.accounts.map((a) => (a.type === "credit_card" ? "credit card" : typeName[a.type].toLowerCase())).join(", ") || "No accounts"}
                {c?.consentExpiresAt && (
                  <span className={cn("block", c.expiresSoon && "font-semibold text-warning")}>Consent until {formatDay(c.consentExpiresAt.slice(0, 10))}</span>
                )}
              </span>
              <span className="pl-10 sm:pl-0">
                {attention || c?.expiresSoon ? (
                  <Button asChild size="sm">
                    <a href="https://meu.pluggy.ai" target="_blank" rel="noreferrer">
                      Open Meu Pluggy <ExternalLink className="size-3.5" aria-hidden />
                    </a>
                  </Button>
                ) : null}
              </span>
            </li>
          );
        })}
      </ul>
      <div className="flex flex-col gap-1 border-t border-rule pt-3.5 type-caption text-ink-muted">
        <span>
          <Clock className="mr-1.5 inline size-3.5 align-[-2px]" aria-hidden />
          Automatic sync every 6 hours, at <span className="font-figure">00:17, 06:17, 12:17, 18:17</span> Brasília time · next at{" "}
          <span className="font-figure">{nextSync()}</span>
        </span>
        <span>
          To add a bank, connect it in Meu Pluggy and add its item id to <code className="font-figure">LASTRO_PLUGGY_ITEM_IDS</code>.
        </span>
      </div>
    </Section>
  );
}

function BudgetSettings() {
  const month = monthOf(new Date());
  const res = useResource<BudgetMonth>(`/api/v1/budget?month=${month}`);
  const v = res.data;
  return (
    <Section
      id="budget"
      title="Budget"
      intro={
        <>
          How each month starts. Category limits are set on the <Link to="/budget">Budget</Link> screen.
        </>
      }
    >
      {v ? <PayYourselfFirst key={`${v.payYourselfFirst.plannedCents}-${v.payday}`} v={v} /> : <p className="m-0 type-body text-ink-muted">Loading…</p>}
    </Section>
  );
}

function PayYourselfFirst({ v }: { v: BudgetMonth }) {
  const [amount, setAmount] = useState(inputBRL(v.payYourselfFirst.plannedCents));
  const [payday, setPayday] = useState(String(v.payday));
  const [state, setState] = useState<"idle" | "busy" | "saved" | string>("idle");
  const cents = parseBRL(amount);
  const day = Number(payday);
  const validDay = Number.isInteger(day) && day >= 1 && day <= 31;
  const changed = cents !== v.payYourselfFirst.plannedCents || day !== v.payday;

  async function save() {
    if (cents === null || !validDay) return;
    setState("busy");
    try {
      await send("PUT", `/api/v1/budget/${v.month}/pay-yourself-first`, { amountCents: cents, payday: day });
      setState("saved");
    } catch (e) {
      setState((e as Error).message);
    }
  }

  return (
    <form
      className="flex flex-col gap-0"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <Row label="Pay yourself first" hint="A fixed line every month, set aside before any limit and never counted as spending. Done once transfers to your broker reach it.">
        <TextField aria-label="Pay yourself first, in reais" prefix="R$" numeric inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} className="w-[160px]" error={amount && cents === null ? "Type an amount" : undefined} />
      </Row>
      <Row label="Payday" hint="When the salary lands and pay yourself first is due. A day past the month's end means its last day.">
        <TextField aria-label="Payday, day of the month" numeric inputMode="numeric" value={payday} onChange={(e) => setPayday(e.target.value)} suffix="of each month" className="w-[200px]" error={payday && !validDay ? "1 to 31" : undefined} />
      </Row>
      <div className="flex flex-wrap items-center gap-3 border-t border-rule pt-4">
        <Button type="submit" size="sm" variant="primary" disabled={state === "busy" || !changed || cents === null || !validDay}>
          Save
        </Button>
        <span role="status" className={cn("type-caption", state !== "idle" && state !== "busy" && state !== "saved" ? "text-danger" : "text-ink-muted")}>
          {state === "saved"
            ? `Saved from ${formatMonth(v.month)} on.`
            : state !== "idle" && state !== "busy"
              ? `Could not save: ${state}.`
              : `From ${formatMonth(v.month)} on; earlier months keep theirs.`}
        </span>
      </div>
    </form>
  );
}

function Investments() {
  const res = useResource<PortfolioOverview>("/api/v1/portfolio");
  const v = res.data;
  return (
    <Section id="investments" title="Investments" intro="The targets the contribution planner aims for. It never suggests selling.">
      {v ? <TargetsForm key={JSON.stringify(v.targets) + v.bandBp} v={v} /> : <p className="m-0 type-body text-ink-muted">Loading…</p>}
    </Section>
  );
}

function TargetsForm({ v }: { v: PortfolioOverview }) {
  const [values, setValues] = useState<Record<AssetClass, string>>(
    Object.fromEntries(assetClasses.map((k) => [k.id, String((v.targets[k.id] ?? 0) / 100).replace(".", ",")])) as Record<AssetClass, string>,
  );
  const [band, setBand] = useState(String(v.bandBp / 100).replace(".", ","));
  const [state, setState] = useState<"idle" | "busy" | "saved" | string>("idle");
  const bps = (t: string) => {
    const n = Number(t.replace(",", "."));
    return t.trim() !== "" && Number.isFinite(n) && n >= 0 ? Math.round(n * 100) : null;
  };
  const parts = assetClasses.map((k) => bps(values[k.id]));
  const sum = parts.reduce<number>((s, p) => s + (p ?? 0), 0);
  const bandBps = bps(band);
  const valid = parts.every((p) => p !== null) && sum === 10000 && bandBps !== null && bandBps <= 1000;
  const emergency = v.holdings.filter((h) => h.emergency);

  async function save() {
    setState("busy");
    try {
      await send("PUT", "/api/v1/portfolio/targets", {
        targets: Object.fromEntries(assetClasses.map((k, i) => [k.id, parts[i]])),
        bandBp: bandBps,
      });
      setState("saved");
    } catch (e) {
      setState((e as Error).message);
    }
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <Row
        label="Target allocation"
        hint={
          <span className={cn("inline-flex items-center gap-1", sum === 10000 ? "text-inflow" : "text-warning")}>
            {sum === 10000 ? <Check className="size-3.5" aria-hidden /> : <TriangleAlert className="size-3.5" aria-hidden />}
            {sum === 10000 ? "Adds up to 100%" : `Adds up to ${formatPercent(sum)}: ${formatPercent(Math.abs(10000 - sum))} ${sum < 10000 ? "missing" : "too much"}`}
            <span className="text-ink-muted">· the emergency fund stays out</span>
          </span>
        }
      >
        <div className="grid w-full gap-2 sm:w-[460px] sm:grid-cols-2">
          {assetClasses.map((k) => (
            <label key={k.id} className="flex items-center justify-between gap-2">
              <CategoryTag name={k.name} color={k.color} />
              <TextField aria-label={`${k.name} target, percent`} numeric inputMode="decimal" value={values[k.id]} suffix="%" className="w-[84px]" onChange={(e) => setValues({ ...values, [k.id]: e.target.value })} />
            </label>
          ))}
        </div>
      </Row>
      <Row label="Tolerance band" hint="A class further than this from its target gets new money first, or none.">
        <TextField aria-label="Band, points either side" numeric inputMode="decimal" value={band} onChange={(e) => setBand(e.target.value)} suffix="points either side" className="w-[200px]" error={bandBps === null || bandBps > 1000 ? "0 to 10" : undefined} />
      </Row>
      <Row
        label="Emergency fund"
        hint={
          <>
            Shown apart and never part of a contribution split. Choose its positions on the <Link to="/portfolio">Portfolio</Link>, by clicking one.
          </>
        }
      >
        <span className="text-right text-[14px] leading-5">{emergency.length ? emergency.map((h) => h.name).join(", ") : <span className="text-ink-muted">None set</span>}</span>
      </Row>
      <Row label="Review the targets" hint="Each January the Portfolio asks you to look at the allocation again, before the year's first contribution.">
        <span className="text-[14px] text-ink-muted">Every January</span>
      </Row>
      <div className="flex flex-wrap items-center gap-3 border-t border-rule pt-4">
        <Button type="submit" size="sm" variant="primary" disabled={!valid || state === "busy"}>
          Save targets
        </Button>
        <span role="status" className={cn("type-caption", !["idle", "busy", "saved"].includes(state) ? "text-danger" : "text-ink-muted")}>
          {state === "saved" ? "Saved. The next contribution aims for them." : !["idle", "busy"].includes(state) ? `Could not save: ${state}.` : ""}
        </span>
      </div>
    </form>
  );
}

function Categorization() {
  const cats = useResource<{ categories: Category[] }>("/api/v1/categories");
  const rules = useResource<{ rules: Rule[] }>("/api/v1/rules");
  const server = useResource<ServerSettings>("/api/v1/settings");
  const live = cats.data?.categories.filter((c) => !c.archived).length;
  const n = rules.data?.rules.length;
  const model = server.data?.model;

  return (
    <Section id="categorization" title="Categorization" intro="Your rules run first. The local model only sees what no rule matches.">
      <Row
        label="Categories and rules"
        hint={live !== undefined && n !== undefined ? `${live} categories and ${n} ${n === 1 ? "rule" : "rules"}. Every correction becomes a rule.` : "Every correction becomes a rule."}
      >
        <Button asChild size="sm">
          <Link to="/categories">Manage categories and rules</Link>
        </Button>
      </Row>
      <Row
        label="Local model"
        hint={
          <>
            Ollama, run at home: no financial data leaves the network. When it is off, new transactions wait in{" "}
            <Link to="/review">To review</Link>.
          </>
        }
      >
        <span className="flex flex-col items-start gap-1 sm:items-end">
          {!model ? (
            <Badge>Checking…</Badge>
          ) : model.state === "online" ? (
            <Badge tone="inflow" icon={Check}>
              Online · {model.name}
            </Badge>
          ) : model.state === "off" ? (
            <Badge>Off</Badge>
          ) : model.state === "missing" ? (
            <Badge tone="warning" icon={TriangleAlert}>
              {model.name} not pulled
            </Badge>
          ) : (
            <Badge tone="warning" icon={TriangleAlert}>
              Offline
            </Badge>
          )}
          <span className="type-caption text-ink-muted">
            Set by <span className="font-figure">LASTRO_OLLAMA_URL</span> and <span className="font-figure">LASTRO_OLLAMA_MODEL</span>
          </span>
        </span>
      </Row>
    </Section>
  );
}

function Appearance() {
  const { preference } = useTheme();
  return (
    <Section id="appearance" title="Appearance">
      <Row label="Theme" hint="System follows your device. The moon in the sidebar switches it too. Saved in this browser.">
        <SegmentedControl label="Theme" options={themes} value={preference} onChange={setTheme} />
      </Row>
    </Section>
  );
}

function Server() {
  const s = useResource<ServerSettings>("/api/v1/settings");
  const rows: { label: string; value: ReactNode; figure?: boolean }[] = [
    { label: "Running", value: s.data ? (s.data.version.length > 12 ? s.data.version.slice(0, 7) : s.data.version) : "…", figure: true },
    { label: "Environment", value: s.data?.env ?? "…", figure: true },
    { label: "Sign-in", value: s.data?.signIn ?? "…" },
    { label: "Backups", value: "postgres-main to R2, through Barman Cloud" },
    {
      label: "Alerts",
      value: s.data ? (s.data.alerts ? `Shown here and pushed to ${s.data.alerts} after each sync` : "Shown in the app only; set LASTRO_TELEGRAM_BOT_TOKEN and LASTRO_TELEGRAM_CHAT_ID to push them") : "…",
    },
  ];
  return (
    <Section
      id="server"
      title={
        <>
          Server <Badge>Read-only</Badge>
        </>
      }
      intro="Changed in the saradomin overlay, not here."
    >
      <div role="table" aria-label="Server" className="flex flex-col">
        {rows.map((r) => (
          <div key={r.label} role="row" className="grid grid-cols-[140px_minmax(0,1fr)] gap-4 border-t border-rule py-2.5">
            <span role="cell" className="type-caption text-ink-muted">
              {r.label}
            </span>
            <span role="cell" className={cn("text-[14px] leading-5 [overflow-wrap:anywhere]", r.figure && "font-figure")}>
              {r.value}
            </span>
          </div>
        ))}
      </div>
    </Section>
  );
}
