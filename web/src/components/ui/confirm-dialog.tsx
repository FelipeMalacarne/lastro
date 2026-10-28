import { useState, type ReactElement, type ReactNode } from "react";
import { AlertDialog } from "radix-ui";

import { Button } from "@/components/ui/button";

type Props = {
  /** What opens it, usually a ghost Button. */
  trigger: ReactElement;
  /** The question with the thing's name: "Archive Groceries?". */
  title: string;
  /** What happens and what does not. Fine print in a <p className="type-caption text-ink-muted">. */
  children: ReactNode;
  /** The verb and the object: "Archive Groceries", "Delete rule". */
  confirmLabel: string;
  /** Runs the action; if it throws, the dialog stays open and says why. */
  onConfirm: () => Promise<unknown>;
};

/**
 * Cédula's ConfirmDialog on Radix's AlertDialog: asks before the few things
 * that cannot be taken back. Both buttons are secondary, Cancel takes the
 * focus, Esc cancels and the scrim does not close it.
 */
export function ConfirmDialog({ trigger, title, children, confirmLabel, onConfirm }: Props) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function confirm() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
      setOpen(false);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <AlertDialog.Root
      open={open}
      onOpenChange={(o) => {
        if (busy) return;
        setOpen(o);
        setError(null);
      }}
    >
      <AlertDialog.Trigger asChild>{trigger}</AlertDialog.Trigger>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className="fixed inset-0 z-40 bg-(--scrim) data-[state=closed]:animate-[lt-fade-out_120ms_ease-in] data-[state=open]:animate-[lt-fade_160ms_ease-out]" />
        <AlertDialog.Content className="fixed inset-0 z-40 m-auto flex h-fit w-[calc(100%-32px)] max-w-[400px] flex-col gap-2 rounded-lg bg-paper-raised p-5 text-ink shadow-pop outline-none data-[state=closed]:animate-[lt-fade-out_120ms_ease-in] data-[state=open]:animate-[lt-pop_160ms_ease-out] sm:px-6 sm:pt-6">
          <AlertDialog.Title className="m-0 type-heading">{title}</AlertDialog.Title>
          <AlertDialog.Description asChild>
            <div className="flex flex-col gap-2 type-body [&_p]:m-0">{children}</div>
          </AlertDialog.Description>
          {error && (
            <p role="alert" className="m-0 type-caption text-danger">
              Nothing changed: {error}.
            </p>
          )}
          <div className="mt-4 flex flex-wrap justify-end gap-2 max-[480px]:flex-col-reverse max-[480px]:*:h-11 max-[480px]:*:w-full">
            <AlertDialog.Cancel asChild>
              <Button disabled={busy}>Cancel</Button>
            </AlertDialog.Cancel>
            <Button onClick={() => void confirm()} disabled={busy}>
              {confirmLabel}
            </Button>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}
