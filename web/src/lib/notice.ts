import type { Bank } from "@/shell/Accounts";
import { formatDay } from "./format";

export const MEU_PLUGGY = "https://meu.pluggy.ai";
export const PLUGGY_DASHBOARD = "https://dashboard.pluggy.ai";

export type NoticeContent = {
  tone: "warning" | "danger";
  title: string;
  text: string;
  action?: { label: string; href: string };
};

function localDay(iso: string): string {
  const d = new Date(iso);
  return formatDay(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`);
}

/**
 * The one notice to show, worst first: a connection that needs me to log in,
 * one lastro could not read, one whose data is stale, then an authorization
 * that ends soon. The accounts list shows the rest.
 */
export function pickNotice(banks: Bank[]): NoticeContent | null {
  const conns = banks.flatMap((b) => (b.connection ? [b.connection] : []));
  const reauth = conns.find((c) => c.health === "reauth");
  if (reauth) {
    if (reauth.detail.startsWith("Authorization ended")) {
      return {
        tone: "danger",
        title: `${reauth.institution}: the Meu Pluggy authorization ended.`,
        text: "Renew it in the Pluggy Dashboard (Connect account, MeuPluggy) to sync again.",
        action: { label: "Open Pluggy Dashboard", href: PLUGGY_DASHBOARD },
      };
    }
    return {
      tone: "danger",
      title: `${reauth.institution} needs you to log in again.`,
      text: "Its transactions stop updating until you do.",
      action: { label: "Open Meu Pluggy", href: MEU_PLUGGY },
    };
  }
  const failing = conns.find((c) => c.readFailing && c.health === "stale");
  if (failing) {
    return {
      tone: "warning",
      title: `lastro could not read ${failing.institution} on the last sync.`,
      text: "The sync job's log says why; it tries again within six hours.",
    };
  }
  const stale = conns.find((c) => c.health === "stale");
  if (stale) {
    if (!stale.dataUpdatedAt) {
      return { tone: "warning", title: `${stale.institution} has not synced yet.`, text: "The first sync runs within six hours." };
    }
    return {
      tone: "warning",
      title: `${stale.institution} last synced ${stale.short} ago.`,
      text: `Transactions since ${localDay(stale.dataUpdatedAt)} may be missing.`,
      action: { label: "Open Meu Pluggy", href: MEU_PLUGGY },
    };
  }
  const soon = conns.find((c) => c.expiresSoon && c.consentExpiresAt);
  if (soon && soon.consentExpiresAt) {
    return {
      tone: "warning",
      title: `${soon.institution}: the Meu Pluggy authorization ends on ${localDay(soon.consentExpiresAt)}.`,
      text: "Renew it in the Pluggy Dashboard before then.",
      action: { label: "Open Pluggy Dashboard", href: PLUGGY_DASHBOARD },
    };
  }
  return null;
}
