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
import { Portfolio } from "@/pages/Portfolio";
import { Review } from "@/pages/Review";
import { Settings } from "@/pages/Settings";
import { Soon } from "@/pages/Soon";
import { Transactions } from "@/pages/Transactions";

export function App() {
  const { path } = useLocation();

  // The app opens on the dashboard.
  useEffect(() => {
    if (path === "/") navigate("/dashboard" + location.search, { replace: true });
  }, [path]);

  return <AppShell accounts={<SidebarAccounts />}>{page(path)}</AppShell>;
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
    case "/_components":
      if (import.meta.env.DEV) return <Gallery />;
      return <Transactions />;
    case "/planning":
      return <Soon title="Planning" milestone="M5" what="Projections in today's money, and how long until each goal." />;
    default:
      return <Transactions />;
  }
}
