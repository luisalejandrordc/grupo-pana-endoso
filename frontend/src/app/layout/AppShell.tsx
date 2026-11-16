import { useEffect, useState, type ReactNode } from "react";
import { useLocation } from "react-router-dom";
import type { EmployeeProfile } from "../../shared/types";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";

export function AppShell({
  children,
  user,
  query,
  setQuery,
  onSignOut,
}: {
  children: ReactNode;
  user: EmployeeProfile;
  query: string;
  setQuery: (value: string) => void;
  onSignOut: () => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const { pathname } = useLocation();

  // A new page starts with an empty search and a closed menu.
  useEffect(() => {
    setQuery("");
    setMenuOpen(false);
  }, [pathname, setQuery]);

  return (
    <div className="shell">
      <a className="skip-link" href="#content">
        Saltar al contenido
      </a>
      <Sidebar
        user={user}
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        onSignOut={onSignOut}
      />
      <div className="shell-main">
        <Topbar
          query={query}
          onQueryChange={setQuery}
          onMenu={() => setMenuOpen(true)}
        />
        <main id="content" className="page">
          {children}
        </main>
      </div>
    </div>
  );
}
