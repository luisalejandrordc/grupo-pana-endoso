import { Link } from "react-router-dom";
import { Icon, logoSquare } from "@endosoflow/ui";

/**
 * The bar of narrow screens: the button that opens the menu and the logo. On a wide screen the sidebar already
 * does both, so no bar is drawn. (Search is not here: each list has its own, beside the filters it works with.)
 */
export function Topbar({
  onMenu,
  menuOpen,
}: {
  onMenu: () => void;
  menuOpen: boolean;
}) {
  return (
    <header className="topbar">
      <button
        className="icon-button topbar-menu"
        onClick={onMenu}
        aria-label="Abrir menú"
        aria-controls="app-sidebar"
        aria-expanded={menuOpen}
      >
        <Icon name="menu" size={20} />
      </button>
      <Link className="topbar-brand" to="/sales" aria-label="EndosoFlow">
        <img src={logoSquare} alt="" />
      </Link>
    </header>
  );
}
