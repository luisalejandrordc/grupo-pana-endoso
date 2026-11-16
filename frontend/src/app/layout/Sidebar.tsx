import { NavLink, useNavigate } from "react-router-dom";
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
  const navigate = useNavigate();
  const name = `${user.firstName} ${user.lastName}`;
  return (
    <>
      {open && <div className="scrim" onClick={onClose} aria-hidden />}
      <aside className={`sidebar ${open ? "is-open" : ""}`}>
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
        <nav className="sidebar-nav" aria-label="Navegación principal">
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
          <button
            className="user-chip"
            onClick={() => navigate("/profile")}
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
          </button>
          <button className="nav-link nav-link-quiet" onClick={onSignOut}>
            <Icon name="logout" size={18} />
            Cerrar sesión
          </button>
        </div>
      </aside>
    </>
  );
}
