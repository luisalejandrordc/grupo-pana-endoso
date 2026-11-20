import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Icon } from "./Icon";

/** The title block every page opens with: a heading, one line of context and the page's main actions. */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <header className="page-head">
      <div>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="page-head-actions">{actions}</div>}
    </header>
  );
}

/** A way back to the list a page was opened from. */
export function BackLink({
  to,
  children,
}: {
  to: string;
  children: ReactNode;
}) {
  return (
    <nav className="crumbs" aria-label="Ruta de navegación">
      <Link to={to}>
        <Icon name="arrow-left" size={15} /> {children}
      </Link>
    </nav>
  );
}
