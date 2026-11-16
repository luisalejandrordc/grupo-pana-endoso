import { useEffect, useRef } from "react";
import { NavLink } from "react-router-dom";
import { useMediaQuery } from "../../shared/hooks/useMediaQuery";
import { roleLabel } from "../../shared/lib/roles";
import type { EmployeeProfile } from "../../shared/types";
import { Avatar } from "../../shared/ui/Avatar";
import { Icon } from "../../shared/ui/Icon";
import { navItems } from "../navigation";

export function Sidebar({
  user,
  open,
  onClose,
  onSignOut,
}: {
  user: EmployeeProfile;
  open: boolean;
  onClose: () => void;
  onSignOut: () => void;
}) {
  const name = `${user.firstName} ${user.lastName}`;
  const drawer = useMediaQuery("(max-width: 900px)");
  const nav = useRef<HTMLElement>(null);
  const modal = drawer && open;

  // As a drawer it behaves like a dialog: focus goes in, Escape closes, the page behind stays put.
  useEffect(() => {
    if (!modal) return;
    nav.current?.querySelector<HTMLElement>("a")?.focus();
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = overflow;
      document.removeEventListener("keydown", onKey);
    };
  }, [modal, onClose]);

  // A closed drawer is off screen: keep it out of the tab order and away from screen readers.
  const hidden = drawer && !open;
  return (
    <>
      {modal && <div className="scrim" onClick={onClose} aria-hidden="true" />}
      <aside
        id="app-sidebar"
        className={`sidebar ${open ? "is-open" : ""}`}
        {...(hidden ? ({ inert: "" } as Record<string, string>) : {})}
      >
        <div className="sidebar-brand">
          <img src="/endosoflow-logo-landscape.png" alt="EndosoFlow" />
          <button
            className="icon-button sidebar-close"
            onClick={onClose}
            aria-label="Cerrar menú"
          >
            <Icon name="x" size={18} />
          </button>
        </div>
        <nav
          ref={nav}
          className="sidebar-nav"
          aria-label="Navegación principal"
        >
          {navItems
            .filter((item) => !item.roles || item.roles.includes(user.role))
            .map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `nav-link ${isActive ? "is-active" : ""}`
                }
              >
                <Icon name={item.icon} size={18} />
                {item.label}
              </NavLink>
            ))}
        </nav>
        <div className="sidebar-foot">
          <NavLink
            to="/profile"
            className={({ isActive }) =>
              `user-chip ${isActive ? "is-active" : ""}`
            }
            title="Mi perfil"
          >
            <Avatar
              id={user.id}
              name={name}
              hasPhoto={user.hasPhoto}
              size={36}
            />
            <span>
              <strong>{name}</strong>
              <small>{roleLabel[user.role]}</small>
            </span>
          </NavLink>
          <button className="nav-link nav-link-quiet" onClick={onSignOut}>
            <Icon name="logout" size={18} />
            Cerrar sesión
          </button>
        </div>
      </aside>
    </>
  );
}
