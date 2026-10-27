import { useState } from "react";
import { Search } from "lucide-react";

import { Amount } from "@/components/Amount";
import { Badge } from "@/components/Badge";
import { CategoryTag } from "@/components/CategoryTag";
import { Panel } from "@/components/Panel";
import { SegmentedControl } from "@/components/SegmentedControl";
import { SyncStatus } from "@/components/SyncStatus";
import { TextField } from "@/components/TextField";
import { TransactionRow } from "@/components/TransactionRow";
import { Button } from "@/components/ui/button";

/** Every component in its states, to check against the design system. Dev builds only. */
export function Gallery() {
  const [type, setType] = useState("all");
  return (
    <>
      <h1 className="m-0 type-title">Components</h1>
      <Panel title="Amount">
        <div className="flex flex-wrap items-baseline gap-6">
          <Amount cents={98234500} size="hero" />
          <Amount cents={-148237} size="lg" />
          <Amount cents={1240000} sign="always" />
          <Amount cents={-4890} sign="always" />
          <Amount cents={-2310} sign="always" pending />
          <Amount cents={98000000} estimate size="sm" />
        </div>
      </Panel>
      <Panel title="Badge and CategoryTag">
        <div className="flex flex-wrap items-center gap-3">
          <Badge>Neutral</Badge>
          <Badge tone="tinta">Rule created</Badge>
          <Badge tone="inflow">On track</Badge>
          <Badge tone="warning">4</Badge>
          <Badge tone="danger">Over</Badge>
          <Badge dashed>Pending</Badge>
          <Badge figure>3 of 10</Badge>
          <CategoryTag name="Groceries" color="note-50" />
          <CategoryTag name="Restaurants" color="note-10" onClick={() => {}} />
          <CategoryTag review onClick={() => {}} />
        </div>
      </Panel>
      <Panel title="SyncStatus">
        <div className="grid gap-3 sm:grid-cols-2">
          <SyncStatus institution="Nubank" state="ok" detail="Synced 2 h ago" />
          <SyncStatus institution="Inter" state="syncing" detail="Syncing accounts and transactions" />
          <SyncStatus institution="Itaú" state="stale" detail="Last synced 3 days ago" action={<Button size="sm" variant="ghost">Open Meu Pluggy</Button>} />
          <SyncStatus institution="XP" state="reauth" detail="Needs re-authentication in Meu Pluggy" />
          <SyncStatus compact institution="Nubank" state="ok" detail="2 h" />
          <SyncStatus compact institution="Itaú" state="stale" detail="3 days" />
        </div>
      </Panel>
      <Panel title="Inputs and buttons">
        <div className="flex flex-wrap items-end gap-3">
          <TextField label="Search" placeholder="Description, e.g. iFood" prefix={<Search className="size-4" />} className="w-72" />
          <SegmentedControl label="Type" value={type} onChange={setType} options={[{ value: "all", label: "All" }, { value: "in", label: "Inflows" }, { value: "out", label: "Outflows" }]} />
          <Button>Secondary</Button>
          <Button variant="primary">Primary</Button>
          <Button variant="ghost">Ghost</Button>
          <Button size="sm">Small</Button>
        </div>
      </Panel>
      <Panel title="TransactionRow" bodyClassName="-mx-5 border-t border-rule">
        <TransactionRow date="26 Oct" description="iFood" meta="Nubank · credit card" kind="expense" amountCents={-4890} category={{ name: "Restaurants", color: "note-10" }} />
        <TransactionRow date="26 Oct" description="Uber" meta="Nubank · credit card" kind="expense" amountCents={-2310} pending categorized={false} />
        <TransactionRow date="24 Oct" description="Fast Shop" meta="Nubank · credit card" kind="expense" amountCents={-31990} installment="3 of 10" category={null} onCategoryClick={() => {}} />
        <TransactionRow date="24 Oct" description="Pix from Ana L." meta="Nubank · checking" kind="income" amountCents={48000} category={{ name: "Income", color: "note-2" }} />
        <TransactionRow date="23 Oct" description="Nubank savings" meta="Nubank checking → Nubank savings" kind="transfer" amountCents={-50000} />
        <TransactionRow date="20 Oct" description="Nubank card bill" meta="Paid from Nubank checking" kind="bill_payment" amountCents={-241277} selected />
      </Panel>
    </>
  );
}
