import { Notice } from "@/components/Notice";
import { Button } from "@/components/ui/button";
import { pickNotice } from "@/lib/notice";
import type { Bank } from "@/shell/Accounts";

/** The one connection worth a word under the page header: stale, failing or about to expire. */
export function SyncNotice({ banks }: { banks: Bank[] }) {
  const notice = pickNotice(banks);
  if (!notice) return null;
  return (
    <Notice
      tone={notice.tone}
      title={notice.title}
      action={
        notice.action && (
          <Button asChild variant="ghost" size="sm" className="max-lg:hidden">
            <a href={notice.action.href} target="_blank" rel="noreferrer">
              {notice.action.label}
            </a>
          </Button>
        )
      }
    >
      {notice.text}{" "}
      {notice.action && (
        <a href={notice.action.href} target="_blank" rel="noreferrer" className="font-semibold lg:hidden">
          {notice.action.label}
        </a>
      )}
    </Notice>
  );
}
