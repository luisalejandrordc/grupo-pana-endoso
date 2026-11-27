import { Icon, Menu, logoLandscape, type MenuItem } from "@endosoflow/ui";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import type { CustomerProfile } from "../../shared/types";

/**
 * The frame around every signed-in screen. A customer has one main place, their purchases, so the bar has no
 * menu of pages: the logo leads home, and everything about the person (their account, signing out) is one
 * dropdown on the right.
 */
export function PortalShell({
  user,
  onSignOut,
  children,
}: {
  user: CustomerProfile;
  onSignOut: () => void;
  children: ReactNode;
}) {
  const navigate = useNavigate();
  const items: MenuItem[] = [
    { label: "Mi cuenta", icon: "user", onSelect: () => navigate("/account") },
    {
      label: "Cerrar sesión",
      icon: "logout",
      danger: true,
      onSelect: onSignOut,
    },
  ];

  return (
    <div className="shell">
      <a className="skip-link" href="#contenido">
        Saltar al contenido
      </a>
      <header className="shell-bar">
        <div className="shell-bar-inner">
          <Link
            to="/"
            className="shell-brand"
            aria-label="EndosoFlow, mis compras"
          >
            <img src={logoLandscape} alt="" />
          </Link>
          <Menu
            label={`Cuenta de ${user.displayName}`}
            items={items}
            triggerClassName="button button-ghost account-trigger"
          >
            <span className="account-avatar" aria-hidden="true">
              <Icon name="user" size={16} />
            </span>
            <span className="account-name">{user.displayName}</span>
          </Menu>
        </div>
      </header>
      <main id="contenido" className="shell-main" tabIndex={-1}>
        {children}
      </main>
      <footer className="shell-footer">© 2026 Grupo Pana</footer>
    </div>
  );
}
