import { useState } from "react";
import { useDebounced } from "../../shared/lib/useDebounced";
import type { Role } from "../../shared/types";
import { Icon } from "../../shared/ui/Icon";
import type { Toast } from "../../shared/ui/toast";
import type { ApiModelSummary } from "./api";
import { CreateModelModal } from "./CreateModelModal";
import { categories, categoryLabels, formatPrice } from "./labels";
import { ModelDetailModal } from "./ModelDetailModal";
import { useModelList } from "./queries";

const PAGE_SIZE = 12;

export function CatalogPage({
  query,
  role,
  toast,
}: {
  query: string;
  role: Role;
  toast: (toast: Toast) => void;
}) {
  const canManage = role === "system_admin" || role === "sales_manager";
  const [category, setCategory] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "archived">("active");
  const [pageState, setPageState] = useState({ key: "", page: 1 });
  const [creating, setCreating] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);
  const search = useDebounced(query.trim());

  // A new search or filter starts again from the first page, without a wasted request for the old page.
  const filterKey = `${search}|${category}|${status}`;
  const page = pageState.key === filterKey ? pageState.page : 1;
  const setPage = (update: (current: number) => number) =>
    setPageState({ key: filterKey, page: update(page) });

  const list = useModelList({
    q: search,
    category,
    status: canManage ? status : "active",
    page,
    pageSize: PAGE_SIZE,
    sort: "name",
  });
  const total = list.data?.total ?? 0;
  const lastPage = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const filtering = Boolean(
    search || category || (canManage && status !== "active"),
  );

  return (
    <div className="stack">
      <header className="page-head">
        <div>
          <h1>Catálogo de vehículos</h1>
          <p>
            Explora los modelos, versiones y colores disponibles para nuevas
            operaciones.
          </p>
        </div>
        {canManage && (
          <button
            className="button button-primary"
            onClick={() => setCreating(true)}
          >
            <Icon name="plus" size={17} /> Nuevo modelo
          </button>
        )}
      </header>

      <div className="catalog-toolbar">
        <div className="catalog-filter">
          <Icon name="filter" size={16} />
          <select
            aria-label="Categoría"
            value={category}
            onChange={(event) => setCategory(event.target.value)}
          >
            <option value="">Todas las categorías</option>
            {categories.map((item) => (
              <option key={item} value={item}>
                {categoryLabels[item]}
              </option>
            ))}
          </select>
        </div>
        {canManage && (
          <div className="catalog-filter">
            <Icon name="layers" size={16} />
            <select
              aria-label="Estado"
              value={status}
              onChange={(event) =>
                setStatus(event.target.value as typeof status)
              }
            >
              <option value="active">Activos</option>
              <option value="archived">Archivados</option>
              <option value="all">Todos</option>
            </select>
          </div>
        )}
        <span className="catalog-count">
          {list.data ? `${total} modelos` : "Cargando…"}
        </span>
      </div>

      {list.isError ? (
        <div className="list-error" role="alert">
          <h3>No se pudo cargar el catálogo</h3>
          <p>
            {list.error instanceof Error
              ? list.error.message
              : "Inténtalo nuevamente."}
          </p>
          <button
            className="button button-secondary"
            onClick={() => list.refetch()}
          >
            <Icon name="refresh" size={15} /> Reintentar
          </button>
        </div>
      ) : (
        <div className="vehicle-catalog-grid" aria-busy={list.isFetching}>
          {list.isPending &&
            Array.from({ length: 6 }, (_, i) => (
              <article
                key={i}
                className="catalog-vehicle-card"
                aria-hidden="true"
              >
                <div className="catalog-vehicle-image">
                  <div className="catalog-image-placeholder" />
                </div>
                <div className="catalog-vehicle-body">
                  <span className="skeleton-line" />
                  <span
                    className="skeleton-line"
                    style={{ marginTop: 10, width: "60%" }}
                  />
                </div>
              </article>
            ))}
          {list.data?.items.map((model) => (
            <ModelCard
              key={model.id}
              model={model}
              onOpen={() => setOpenId(model.id)}
            />
          ))}
        </div>
      )}

      {list.data && total === 0 && (
        <div className="empty-state">
          <h3>{filtering ? "Sin resultados" : "El catálogo está vacío"}</h3>
          <p>
            {filtering
              ? "No hay modelos que coincidan con la búsqueda o los filtros."
              : canManage
                ? "Crea el primer modelo con el botón «Nuevo modelo»."
                : "Aún no hay modelos disponibles. Consulta con un gerente."}
          </p>
        </div>
      )}

      {total > PAGE_SIZE && (
        <div className="table-footer">
          <span>
            Página {page} de {lastPage}
          </span>
          <div className="pagination">
            <button
              aria-label="Página anterior"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              <Icon name="chevron-right" size={13} className="flip-icon" />
            </button>
            <button
              aria-label="Página siguiente"
              disabled={page >= lastPage}
              onClick={() => setPage((p) => p + 1)}
            >
              <Icon name="chevron-right" size={13} />
            </button>
          </div>
        </div>
      )}

      {creating && (
        <CreateModelModal
          toast={toast}
          onClose={() => setCreating(false)}
          onCreated={(model) => {
            setCreating(false);
            setOpenId(model.id);
          }}
        />
      )}
      {openId && (
        <ModelDetailModal
          modelId={openId}
          role={role}
          toast={toast}
          onClose={() => setOpenId(null)}
        />
      )}
    </div>
  );
}

function ModelCard({
  model,
  onOpen,
}: {
  model: ApiModelSummary;
  onOpen: () => void;
}) {
  const [first, ...rest] = model.categories;
  return (
    <article className="catalog-vehicle-card">
      <div className="catalog-vehicle-image">
        {model.mainImageUrl ? (
          <img src={model.mainImageUrl} alt={model.name} loading="lazy" />
        ) : (
          <div className="catalog-image-placeholder">
            <Icon name="car" size={30} />
          </div>
        )}
        <div className="catalog-card-categories">
          {first && (
            <span className="category-badge">{categoryLabels[first]}</span>
          )}
          {rest.length > 0 && (
            <span className="category-badge">+{rest.length}</span>
          )}
        </div>
        <span className={`catalog-status ${model.isActive ? "" : "archived"}`}>
          {model.isActive ? "Activo" : "Archivado"}
        </span>
      </div>
      <div className="catalog-vehicle-body">
        <div className="catalog-vehicle-heading">
          <h3>{model.name}</h3>
        </div>
        <p>{model.description || "Sin descripción."}</p>
        <div className="catalog-vehicle-meta">
          <span>
            <Icon name="layers" size={14} />
            {model.activeVersionCount}{" "}
            {model.activeVersionCount === 1 ? "versión" : "versiones"}
          </span>
          <span>
            <Icon name="tag" size={14} />
            {model.activeColorCount}{" "}
            {model.activeColorCount === 1 ? "color" : "colores"}
          </span>
        </div>
        {model.minPrice && (
          <div className="catalog-version-list">
            <div>
              <span>Desde</span>
              <strong>{formatPrice(model.minPrice)}</strong>
            </div>
          </div>
        )}
        <button
          className="catalog-footer-link"
          onClick={onOpen}
          aria-label={`Ver detalle de ${model.name}`}
        >
          Ver detalle de modelo <Icon name="arrow-right" size={15} />
        </button>
      </div>
    </article>
  );
}
