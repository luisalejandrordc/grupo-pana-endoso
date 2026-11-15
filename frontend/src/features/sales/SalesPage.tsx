import { useEffect } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { formatDate } from "../../shared/lib/format";
import { useDebounced } from "../../shared/lib/useDebounced";
import type { Role } from "../../shared/types";
import { Avatar } from "../../shared/ui/Avatar";
import { Icon } from "../../shared/ui/Icon";
import type { SaleListItem, SaleListParams } from "./api";
import { errorText, stateLabels, temperatureLabels } from "./labels";
import { Progress, StateBadge, TaskBadge, TempBadge } from "./pills";
import { useAdvisors, useSaleList } from "./queries";

const PAGE_SIZE = 12;

const states = [
  { value: "active", label: "En curso" },
  { value: "completed", label: "Completadas" },
  { value: "cancelled", label: "Canceladas" },
  { value: "all", label: "Todas" },
] as const;

const sorts = [
  { value: "-createdAt", label: "Más recientes" },
  { value: "-updatedAt", label: "Con actividad reciente" },
  { value: "dueDate", label: "Vence primero" },
  { value: "customerName", label: "Cliente (A–Z)" },
] as const;

/** How many of the 13 tasks are done, which the list only knows through the current task. */
export function completedTasks(sale: SaleListItem): number | null {
  if (sale.state === "completed") return 13;
  if (sale.state === "cancelled") return null;
  return sale.currentTask ? sale.currentTask.number - 1 : 0;
}

function dueText(sale: SaleListItem) {
  const task = sale.currentTask;
  if (!task?.dueDate || task.status !== "in_progress") return null;
  return `${task.isOverdue ? "Venció" : "Vence"} ${formatDate(task.dueDate)}`;
}

export function SalesPage({ role, query }: { role: Role; query: string }) {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const management = role !== "sales_advisor";
  const state = params.get("state") ?? "active";
  const temperature = params.get("temperature") ?? "all";
  const advisor = params.get("advisor") ?? "all";
  const sort = params.get("sort") ?? "-createdAt";
  const page = Number(params.get("page") ?? "1") || 1;
  const search = useDebounced(query.trim());
  const advisors = useAdvisors(management);

  function change(next: Record<string, string | null>) {
    const copy = new URLSearchParams(params);
    for (const [key, value] of Object.entries(next)) {
      if (value === null || value === "" || value === "all") copy.delete(key);
      else copy.set(key, value);
    }
    if (!("page" in next)) copy.delete("page");
    setParams(copy, { replace: true });
  }

  // Typing a search starts again from the first page.
  useEffect(() => {
    if (search && params.has("page")) change({ page: null });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  const request: SaleListParams = {
    q: search || undefined,
    state: state === "all" ? undefined : state,
    temperature: temperature === "all" ? undefined : temperature,
    advisorId: advisor === "all" ? undefined : advisor,
    sort: sort as SaleListParams["sort"],
    page,
    pageSize: PAGE_SIZE,
  };
  const list = useSaleList(request);
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const filtered =
    state !== "active" ||
    temperature !== "all" ||
    advisor !== "all" ||
    search !== "";

  return (
    <div className="stack">
      <header className="page-head">
        <div>
          <h1>Ventas</h1>
          <p>
            {role === "sales_advisor"
              ? "Tus operaciones, con la tarea que toca atender en cada una."
              : "Todas las operaciones del equipo y la tarea que toca atender en cada una."}
          </p>
        </div>
        <button
          className="button button-primary"
          onClick={() => navigate("/sales/new")}
        >
          <Icon name="plus" size={17} /> Nueva venta
        </button>
      </header>

      <section className="panel">
        <div className="toolbar">
          <div
            className="segmented"
            role="tablist"
            aria-label="Estado de la venta"
          >
            {states.map((item) => (
              <button
                key={item.value}
                role="tab"
                aria-selected={state === item.value}
                className={state === item.value ? "is-active" : ""}
                onClick={() =>
                  change({ state: item.value === "active" ? null : item.value })
                }
              >
                {item.label}
              </button>
            ))}
          </div>
          <div className="toolbar-filters">
            <select
              aria-label="Prioridad"
              value={temperature}
              onChange={(event) => change({ temperature: event.target.value })}
            >
              <option value="all">Toda prioridad</option>
              {Object.entries(temperatureLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  Prioridad {label.toLowerCase()}
                </option>
              ))}
            </select>
            {management && (
              <select
                aria-label="Asesor"
                value={advisor}
                onChange={(event) => change({ advisor: event.target.value })}
              >
                <option value="all">Todos los asesores</option>
                {advisors.data?.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.firstName} {item.lastName}
                  </option>
                ))}
              </select>
            )}
            <select
              aria-label="Orden"
              value={sort}
              onChange={(event) => change({ sort: event.target.value })}
            >
              {sorts.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {list.isError ? (
          <div className="empty" role="alert">
            <Icon name="alert" size={22} />
            <strong>No se pudieron cargar las ventas</strong>
            <p>{errorText(list.error)}</p>
            <button
              className="button button-secondary"
              onClick={() => list.refetch()}
            >
              Reintentar
            </button>
          </div>
        ) : list.isPending ? (
          <div
            className="skeleton-rows"
            aria-busy="true"
            aria-label="Cargando ventas"
          >
            {Array.from({ length: 6 }, (_, index) => (
              <span key={index} className="skeleton" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <div className="empty">
            <Icon name="sales" size={22} />
            <strong>
              {filtered ? "Ninguna venta coincide" : "Aún no hay ventas"}
            </strong>
            <p>
              {filtered
                ? "Prueba con otros filtros o borra la búsqueda."
                : "Cuando registres la primera venta aparecerá aquí."}
            </p>
            {filtered ? (
              <button
                className="button button-secondary"
                onClick={() =>
                  setParams(new URLSearchParams(), { replace: true })
                }
              >
                Quitar filtros
              </button>
            ) : (
              <button
                className="button button-primary"
                onClick={() => navigate("/sales/new")}
              >
                <Icon name="plus" size={17} /> Registrar una venta
              </button>
            )}
          </div>
        ) : (
          <div
            className={`table-scroll ${list.isFetching ? "is-refreshing" : ""}`}
          >
            <table className="table sales-table">
              <thead>
                <tr>
                  <th>Venta</th>
                  <th>Vehículo</th>
                  {management && <th>Asesor</th>}
                  <th>Avance</th>
                  <th>Tarea actual</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {items.map((sale) => {
                  const done = completedTasks(sale);
                  const due = dueText(sale);
                  return (
                    <tr
                      key={sale.id}
                      className="is-link"
                      tabIndex={0}
                      onClick={() => navigate(`/sales/${sale.id}`)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter")
                          navigate(`/sales/${sale.id}`);
                      }}
                    >
                      <td className="cell-sale">
                        <strong className="cell-title">
                          {sale.customerName}
                        </strong>
                        <span className="cell-sub">
                          <span className="code">{sale.code}</span>
                          {sale.temperature === "hot" && (
                            <TempBadge temperature="hot" />
                          )}
                        </span>
                      </td>
                      <td className="cell-vehicle">
                        <span className="vehicle-cell">
                          {sale.imageUrl ? (
                            <img src={sale.imageUrl} alt="" loading="lazy" />
                          ) : (
                            <span className="vehicle-fallback">
                              <Icon name="car" size={18} />
                            </span>
                          )}
                          <span>
                            <strong className="cell-title">
                              {sale.vehicleModel}
                            </strong>
                            <span className="cell-sub">
                              {sale.vehicleVersion} · {sale.vehicleColor}
                            </span>
                          </span>
                        </span>
                      </td>
                      {management && (
                        <td className="cell-advisor">
                          <span className="person-cell">
                            <Avatar
                              id={sale.advisorId}
                              name={sale.advisorName}
                              hasPhoto={sale.advisorHasPhoto}
                              size={28}
                            />
                            {sale.advisorName}
                          </span>
                        </td>
                      )}
                      <td className="cell-progress">
                        {done === null ? (
                          <span className="muted">—</span>
                        ) : (
                          <Progress done={done} />
                        )}
                      </td>
                      <td className="cell-task">
                        {sale.currentTask ? (
                          <span className="task-cell">
                            <strong className="cell-title">
                              {sale.currentTask.number}. {sale.currentTask.name}
                            </strong>
                            <span className="cell-sub">
                              <TaskBadge
                                status={sale.currentTask.status}
                                overdue={sale.currentTask.isOverdue}
                              />
                              {due && <span>{due}</span>}
                            </span>
                          </span>
                        ) : (
                          <span className="muted">
                            {sale.state === "completed" ? "Entregada" : "—"}
                          </span>
                        )}
                      </td>
                      <td className="cell-state">
                        <StateBadge state={sale.state} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {total > PAGE_SIZE && (
          <footer className="pager">
            <span>
              {(page - 1) * PAGE_SIZE + 1}–{Math.min(page * PAGE_SIZE, total)}{" "}
              de {total} ventas
              {state !== "all" &&
                ` ${stateLabels[state as keyof typeof stateLabels]?.toLowerCase() ?? ""}`}
            </span>
            <span className="pager-buttons">
              <button
                className="button button-secondary button-sm"
                disabled={page <= 1}
                onClick={() => change({ page: String(page - 1) })}
              >
                Anterior
              </button>
              <span>
                Página {page} de {pages}
              </span>
              <button
                className="button button-secondary button-sm"
                disabled={page >= pages}
                onClick={() => change({ page: String(page + 1) })}
              >
                Siguiente
              </button>
            </span>
          </footer>
        )}
      </section>
    </div>
  );
}
