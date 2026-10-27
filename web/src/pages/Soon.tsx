import { PageHeader } from "@/shell/AppShell";
import { Panel } from "@/components/Panel";
import { Link } from "@/lib/router";

/** A destination whose screen arrives in a later milestone. */
export function Soon({ title, milestone, what }: { title: string; milestone: string; what: string }) {
  return (
    <>
      <PageHeader title={title} caption={`Arrives in ${milestone}`} />
      <Panel>
        <p className="m-0 max-w-[70ch] type-body">
          {what} Until then, <Link to="/transactions">Transactions</Link> has every account and transaction lastro has synced.
        </p>
      </Panel>
    </>
  );
}
