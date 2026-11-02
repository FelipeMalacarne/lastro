import { Component, type ReactNode } from "react";

import { Notice } from "@/components/Notice";
import { Button } from "@/components/ui/button";

type Props = { className?: string; children: ReactNode };

/**
 * A screen that fails to draw says so and offers Reload, instead of leaving
 * the page empty. What was saved is on the server either way.
 */
export class ErrorBoundary extends Component<Props, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className={this.props.className}>
        <Notice
          tone="danger"
          title="This screen could not be shown."
          action={
            <Button variant="ghost" size="sm" onClick={() => location.reload()}>
              Reload
            </Button>
          }
        >
          {error.message.replace(/\.$/, "")}.
        </Notice>
      </div>
    );
  }
}
