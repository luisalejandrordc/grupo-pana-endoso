import { useLocation } from "react-router-dom";
import { Icon } from "../../shared/ui/Icon";
import { searchPlaceholders } from "../navigation";

export function Topbar({
  query,
  onQueryChange,
  onMenu,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  onMenu: () => void;
}) {
  const { pathname } = useLocation();
  const section = "/" + pathname.split("/")[1];
  // The search belongs to lists, not to a sale or a form.
  const searchable =
    searchPlaceholders[section] !== undefined &&
    !pathname.startsWith("/sales/");
  return (
    <header className={`topbar ${searchable ? "" : "is-empty"}`}>
      <button
        className="icon-button topbar-menu"
        onClick={onMenu}
        aria-label="Abrir menú"
      >
        <Icon name="menu" size={20} />
      </button>
      {searchable ? (
        <label className="search">
          <Icon name="search" size={17} />
          <input
            type="search"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={searchPlaceholders[section]}
            aria-label="Buscar"
          />
        </label>
      ) : (
        <span />
      )}
    </header>
  );
}
