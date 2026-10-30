import { useEffect } from "react";

import { navigate, useLocation } from "@/lib/router";
import { SidebarAccounts } from "@/shell/Accounts";
import { AppShell } from "@/shell/AppShell";
import { AccountPage } from "@/pages/Account";
import { Budget } from "@/pages/Budget";
import { Categories } from "@/pages/Categories";
import { Contribute } from "@/pages/Contribute";
import { Dashboard } from "@/pages/Dashboard";
import { Gallery } from "@/pages/Gallery";
import { Planning } from "@/pages/Planning";
import { Portfolio } from "@/pages/Portfolio";
import { Report } from "@/pages/Report";
import { Review } from "@/pages/Review";
import { Settings } from "@/pages/Settings";
import { Transactions } from "@/pages/Transactions";

export function App() {
  const { path } = useLocation();
  const shown = page(path);

  // The app opens on the dashboard, and so does a path it does not know.
  useEffect(() => {
    if (!shown) navigate("/dashboard" + location.search, { replace: true });
  }, [path]);

  return <AppShell accounts={<SidebarAccounts />}>{shown}</AppShell>;
}

function page(path: string) {
  const account = /^\/accounts\/([0-9a-f-]{36})$/.exec(path);
  if (account) return <AccountPage key={account[1]} id={account[1]} />;
  switch (path) {
    case "/dashboard":
      return <Dashboard />;
    case "/review":
      return <Review />;
    case "/categories":
      return <Categories />;
    case "/settings":
      return <Settings />;
    case "/budget":
      return <Budget />;
    case "/portfolio":
      return <Portfolio />;
    case "/contribute":
      return <Contribute />;
    case "/transactions":
      return <Transactions />;
    case "/_components":
      return import.meta.env.DEV ? <Gallery /> : null;
    case "/planning":
      return <Planning />;
    case "/report":
      return <Report />;
    default:
      return null;
  }
}
