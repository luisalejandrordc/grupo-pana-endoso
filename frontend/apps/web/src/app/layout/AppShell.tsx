import { useCallback, useState, type ReactNode } from "react";
import { useLocation } from "react-router-dom";
import { useDocumentTitle } from "@endosoflow/ui";
import type { EmployeeProfile } from "../../shared/types";
import { titleFor } from "../navigation";
import { ErrorBoundary } from "../ErrorBoundary";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";

export function AppShell({
  children,
  user,
  onSignOut,
}: {
  children: ReactNode;
  user: EmployeeProfile;
  onSignOut: () => void;
}) {
  const { pathname } = useLocation();
  // The drawer is open for one address only: going anywhere else closes it, with no effect to do so.
  const [openAt, setOpenAt] = useState<string | null>(null);
  const menuOpen = openAt === pathname;
  const closeMenu = useCallback(() => setOpenAt(null), []);
  useDocumentTitle(titleFor(pathname));

  return (
    <div className="shell">
      <a className="skip-link" href="#content">
        Saltar al contenido
      </a>
      <Sidebar
        user={user}
        open={menuOpen}
        onClose={closeMenu}
        onSignOut={onSignOut}
      />
      <div className="shell-main">
        <Topbar onMenu={() => setOpenAt(pathname)} menuOpen={menuOpen} />
        <main id="content" className="page">
          <ErrorBoundary resetKey={pathname}>{children}</ErrorBoundary>
        </main>
      </div>
    </div>
  );
}
