import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./index.css";
import { App } from "./App";
import { ErrorBoundary } from "./shell/ErrorBoundary";
import "./lib/theme";
import "./lib/update";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/* The last resort, should the shell itself fail: each screen has its own. */}
    <ErrorBoundary className="min-h-dvh bg-paper p-4 font-text text-ink lg:p-8">
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
