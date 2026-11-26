import { Icon, logoLandscape } from "@endosoflow/ui";
import type { ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import type { CustomerProfile } from "../../shared/types";

/**
 * The frame around every signed-in screen. A customer has few areas, so it is a top bar: the brand, the two
 * places they can go, who is signed in, and the way out.
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
  return (
    <div className="shell">
      <a className="skip-link" href="#contenido">
        Saltar al contenido
      </a>
      <header className="shell-bar">
        <div className="shell-bar-inner">
          <Link to="/" className="shell-brand" aria-label="EndosoFlow, inicio">
            <img src={logoLandscape} alt="" />
          </Link>
          <nav className="shell-nav" aria-label="Principal">
            <NavLink to="/" end className="shell-link">
              <Icon name="car" size={17} /> <span>Mis compras</span>
            </NavLink>
            <NavLink to="/account" className="shell-link">
              <Icon name="user" size={17} /> <span>Mi cuenta</span>
            </NavLink>
          </nav>
          <div className="shell-user">
            <span className="shell-name" title={user.displayName}>
              {user.displayName}
            </span>
            <button
              type="button"
              className="button button-ghost button-sm"
              onClick={onSignOut}
            >
              <Icon name="logout" size={16} /> <span>Cerrar sesión</span>
            </button>
          </div>
        </div>
      </header>
      <main id="contenido" className="shell-main" tabIndex={-1}>
        {children}
      </main>
      <footer className="shell-footer">© 2026 Grupo Pana</footer>
    </div>
  );
}
