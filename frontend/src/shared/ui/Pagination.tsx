import { Icon } from "./Icon";

/**
 * The footer of a paged list: which rows are showing, which page this is and the two arrows.
 * It renders nothing for an empty list, and for a single page unless `showSinglePage` is set.
 */
export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  noun,
  showSinglePage = false,
}: {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  /** What the rows are, in the plural, for the summary: "ventas". */
  noun?: string;
  showSinglePage?: boolean;
}) {
  if (total === 0 || (!showSinglePage && total <= pageSize)) return null;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const from = (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);
  return (
    <footer className="pager">
      <span>
        Mostrando{" "}
        <strong>
          {from}–{to}
        </strong>{" "}
        de <strong>{total}</strong>
        {noun ? ` ${noun}` : ""}
      </span>
      <div className="pager-controls">
        <span>
          Página {page} de {pages}
        </span>
        <div className="pager-buttons">
          <button
            aria-label="Página anterior"
            disabled={page <= 1}
            onClick={() => onPageChange(page - 1)}
          >
            <Icon name="chevron-right" size={14} className="flip-icon" />
          </button>
          <button
            aria-label="Página siguiente"
            disabled={page >= pages}
            onClick={() => onPageChange(page + 1)}
          >
            <Icon name="chevron-right" size={14} />
          </button>
        </div>
      </div>
    </footer>
  );
}
