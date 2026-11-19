import { useState } from "react";
import { useDebounced } from "../../shared/lib/useDebounced";
import type { Role } from "../../shared/types";
import { Badge } from "../../shared/ui/Badge";
import { EmptyState, ErrorState } from "../../shared/ui/EmptyState";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pagination } from "../../shared/ui/Pagination";
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
  const filtering = Boolean(
    search || category || (canManage && status !== "active"),
  );

  return (
    <div className="stack">
      <PageHeader
        title="Catálogo de vehículos"
        description="Explora los modelos, versiones y colores disponibles para nuevas operaciones."
        actions={
          canManage && (
            <button
              className="button button-primary"
              onClick={() => setCreating(true)}
            >
              <Icon name="plus" size={17} /> Nuevo modelo
            </button>
          )
        }
      />

      <div className="panel toolbar">
        <div className="filter-bar">
          <select
            aria-label="Categoría"
            className="filter-select"
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
          {canManage && (
            <select
              aria-label="Estado"
              className="filter-select"
              value={status}
              onChange={(event) =>
                setStatus(event.target.value as typeof status)
              }
            >
              <option value="active">Activos</option>
              <option value="archived">Archivados</option>
              <option value="all">Todos</option>
            </select>
          )}
        </div>
        <span className="toolbar-count">
          {list.data ? `${total} modelos` : "Cargando…"}
        </span>
      </div>

      {list.isError ? (
        <div className="panel">
          <ErrorState
            title="No se pudo cargar el catálogo"
            message={
              list.error instanceof Error
                ? list.error.message
                : "Inténtalo nuevamente."
            }
            onRetry={() => list.refetch()}
          />
        </div>
      ) : (
        <div className="catalog-grid" aria-busy={list.isFetching}>
          {list.isPending &&
            Array.from({ length: 6 }, (_, i) => (
              <article key={i} className="model-card" aria-hidden="true">
                <div className="model-card-image" />
                <div className="model-card-body">
                  <span className="skeleton-line" />
                  <span className="skeleton-line" style={{ width: "60%" }} />
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
        <div className="panel">
          <EmptyState
            icon="car"
            title={filtering ? "Sin resultados" : "El catálogo está vacío"}
            description={
              filtering
                ? "No hay modelos que coincidan con la búsqueda o los filtros."
                : canManage
                  ? "Crea el primer modelo con el botón «Nuevo modelo»."
                  : "Aún no hay modelos disponibles. Consulta con un gerente."
            }
          />
        </div>
      )}

      {total > PAGE_SIZE && (
        <div className="panel">
          <Pagination
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            onPageChange={(next) => setPage(() => next)}
          />
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
    <article className={`model-card${model.isActive ? "" : " is-archived"}`}>
      <div className="model-card-image">
        {model.mainImageUrl ? (
          <img src={model.mainImageUrl} alt={model.name} loading="lazy" />
        ) : (
          <div className="model-card-placeholder">
            <Icon name="car" size={30} />
          </div>
        )}
        <div className="model-card-tags">
          {first && <span className="chip">{categoryLabels[first]}</span>}
          {rest.length > 0 && <span className="chip">+{rest.length}</span>}
        </div>
        {!model.isActive && (
          <span className="model-card-status">
            <Badge tone="neutral" plain>
              Archivado
            </Badge>
          </span>
        )}
      </div>
      <div className="model-card-body">
        <h3>{model.name}</h3>
        <p>{model.description || "Sin descripción."}</p>
        <div className="model-card-meta">
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
        <div className="model-card-foot">
          <div className="model-card-price">
            {model.minPrice && (
              <>
                <small>Desde</small>
                <strong>{formatPrice(model.minPrice)}</strong>
              </>
            )}
          </div>
          <button
            className="model-card-open"
            onClick={onOpen}
            aria-label={`Ver detalle de ${model.name}`}
          >
            Ver detalle <Icon name="arrow-right" size={15} />
          </button>
        </div>
      </div>
    </article>
  );
}
