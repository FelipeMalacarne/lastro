import { useEffect } from "react";

import { navigate, useLocation } from "@/lib/router";
import { SidebarAccounts } from "@/shell/Accounts";
import { AppShell } from "@/shell/AppShell";
import { AccountPage } from "@/pages/Account";
import { Budget } from "@/pages/Budget";
import { Categories } from "@/pages/Categories";
import { Gallery } from "@/pages/Gallery";
import { Review } from "@/pages/Review";
import { Settings } from "@/pages/Settings";
import { Soon } from "@/pages/Soon";
import { Transactions } from "@/pages/Transactions";

export function App() {
  const { path } = useLocation();

  // Until the dashboard exists (M3), the app opens on Transactions.
  useEffect(() => {
    if (path === "/") navigate("/transactions" + location.search, { replace: true });
  }, [path]);

  return <AppShell accounts={<SidebarAccounts />}>{page(path)}</AppShell>;
}

function page(path: string) {
  const account = /^\/accounts\/([0-9a-f-]{36})$/.exec(path);
  if (account) return <AccountPage key={account[1]} id={account[1]} />;
  switch (path) {
    case "/dashboard":
      return <Soon title="Dashboard" milestone="M3" what="The month at a glance: spending so far, by category against your 6-month average, and what is left in the budget." />;
    case "/review":
      return <Review />;
    case "/categories":
      return <Categories />;
    case "/settings":
      return <Settings />;
    case "/budget":
      return <Budget />;
    case "/portfolio":
      return <Soon title="Portfolio" milestone="M4" what="Positions, net worth and where this month's contribution goes." />;
    case "/_components":
      if (import.meta.env.DEV) return <Gallery />;
      return <Transactions />;
    case "/planning":
      return <Soon title="Planning" milestone="M5" what="Projections in today's money, and how long until each goal." />;
    default:
      return <Transactions />;
  }
}
