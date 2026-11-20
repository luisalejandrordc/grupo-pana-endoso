import { Link, useLocation } from "react-router-dom";
import { useSearchQuery, Icon, SearchField, logoSquare } from "@endosoflow/ui";
import { searchPlaceholderFor } from "../navigation";

export function Topbar({
  onMenu,
  menuOpen,
}: {
  onMenu: () => void;
  menuOpen: boolean;
}) {
  const { pathname } = useLocation();
  const [query, setQuery] = useSearchQuery();
  const placeholder = searchPlaceholderFor(pathname);
  return (
    <header className={`topbar ${placeholder ? "" : "is-empty"}`}>
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
      {placeholder && (
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder={placeholder}
          shortcut
        />
      )}
    </header>
  );
}
