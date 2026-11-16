import { useEffect } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { formatDate } from "../../shared/lib/format";
import { useDebounced } from "../../shared/lib/useDebounced";
import type { Role } from "../../shared/types";
import { Avatar } from "../../shared/ui/Avatar";
import { EmptyState, ErrorState } from "../../shared/ui/EmptyState";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pagination } from "../../shared/ui/Pagination";
import { Tabs, type TabItem } from "../../shared/ui/Tabs";
import type { SaleListItem, SaleListParams, Temperature } from "./api";
import { errorText, stateLabels, temperatureLabels } from "./labels";
import { Progress, StateBadge, TaskBadge, TempBadge } from "./pills";
import { completedTasks } from "./progress";
import { useAdvisors, useSaleList, useSaleTotal } from "./queries";

const PAGE_SIZE = 12;

type StateTab = "active" | "completed" | "cancelled" | "all";
type Sort = NonNullable<SaleListParams["sort"]>;

const sorts: { value: Sort; label: string }[] = [
  { value: "-createdAt", label: "Más recientes" },
  { value: "-updatedAt", label: "Con actividad reciente" },
  { value: "dueDate", label: "Vence primero" },
  { value: "customerName", label: "Cliente (A–Z)" },
];

const tabLabels: Record<StateTab, string> = {
  active: "En curso",
  completed: "Completadas",
  cancelled: "Canceladas",
  all: "Todas",
};

const isState = (value: string | null): value is StateTab =>
  value === "active" ||
  value === "completed" ||
  value === "cancelled" ||
  value === "all";
const isTemperature = (value: string | null): value is Temperature =>
  value === "hot" || value === "warm" || value === "cold";
const isSort = (value: string | null): value is Sort =>
  sorts.some((item) => item.value === value);

function dueText(sale: SaleListItem) {
  const task = sale.currentTask;
  if (!task?.dueDate || task.status !== "in_progress") return null;
  return `${task.isOverdue ? "Venció" : "Vence"} ${formatDate(task.dueDate)}`;
}

export function SalesPage({ role, query }: { role: Role; query: string }) {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const management = role !== "sales_advisor";
  const rawState = params.get("state");
  const state: StateTab = isState(rawState) ? rawState : "active";
  const rawTemperature = params.get("temperature");
  const temperature = isTemperature(rawTemperature)
    ? rawTemperature
    : undefined;
  const advisor = params.get("advisor") ?? "all";
  const rawSort = params.get("sort");
  const sort: Sort = isSort(rawSort) ? rawSort : "-createdAt";
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

  const advisorId = advisor === "all" ? undefined : advisor;
  const request: SaleListParams = {
    q: search || undefined,
    state: state === "all" ? undefined : state,
    temperature,
    advisorId,
    sort,
    page,
    pageSize: PAGE_SIZE,
  };
  const list = useSaleList(request);

  // The tab counts follow the other filters, so each number is what that tab would show.
  const shared = { q: search || undefined, temperature, advisorId };
  const active = useSaleTotal({ ...shared, state: "active" });
  const completed = useSaleTotal({ ...shared, state: "completed" });
  const cancelled = useSaleTotal({ ...shared, state: "cancelled" });
  const counts: Record<StateTab, number | undefined> = {
    active: active.data,
    completed: completed.data,
    cancelled: cancelled.data,
    all:
      active.data !== undefined &&
      completed.data !== undefined &&
      cancelled.data !== undefined
        ? active.data + completed.data + cancelled.data
        : undefined,
  };
  const tabs: TabItem<StateTab>[] = (
    ["active", "completed", "cancelled", "all"] as const
  ).map((id) => ({
    id,
    label: tabLabels[id],
    count: counts[id],
  }));

  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const filtered =
    state !== "active" ||
    temperature !== undefined ||
    advisorId !== undefined ||
    search !== "";
  const showState = state === "all";

  return (
    <div className="stack">
      <PageHeader
        title="Ventas"
        description={
          role === "sales_advisor"
            ? "Tus operaciones, con la tarea que toca atender en cada una."
            : "Todas las operaciones del equipo y la tarea que toca atender en cada una."
        }
        actions={
          <button
            className="button button-primary"
            onClick={() => navigate("/sales/new")}
          >
            <Icon name="plus" size={17} /> Nueva venta
          </button>
        }
      />

      <section className="panel">
        <div className="toolbar sales-toolbar">
          <Tabs
            variant="segmented"
            label="Estado de la venta"
            items={tabs}
            value={state}
            onChange={(id) => change({ state: id === "active" ? null : id })}
          />
          <div className="filter-bar">
            <select
              aria-label="Prioridad"
              className="filter-select"
              value={temperature ?? "all"}
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
                className="filter-select"
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
              className="filter-select"
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
          <ErrorState
            title="No se pudieron cargar las ventas"
            message={errorText(list.error)}
            onRetry={() => list.refetch()}
          />
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
          <EmptyState
            icon="sales"
            title={filtered ? "Ninguna venta coincide" : "Aún no hay ventas"}
            description={
              filtered
                ? "Prueba con otros filtros o borra la búsqueda."
                : "Cuando registres la primera venta aparecerá aquí."
            }
            actions={
              filtered ? (
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
              )
            }
          />
        ) : (
          <div
            className={`table-scroll ${list.isFetching ? "is-refreshing" : ""}`}
          >
            <table className="table table-cards">
              <thead>
                <tr>
                  <th>Venta</th>
                  <th>Vehículo</th>
                  {management && <th>Asesor</th>}
                  <th>Avance</th>
                  <th>Tarea actual</th>
                  {showState && <th>Estado</th>}
                </tr>
              </thead>
              <tbody>
                {items.map((sale) => (
                  <SaleRow
                    key={sale.id}
                    sale={sale}
                    management={management}
                    showState={showState}
                    onOpen={() => navigate(`/sales/${sale.id}`)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}

        <Pagination
          page={page}
          pageSize={PAGE_SIZE}
          total={total}
          noun={
            state === "all"
              ? "ventas"
              : `ventas ${stateLabels[state].toLowerCase()}`
          }
          onPageChange={(next) => change({ page: String(next) })}
        />
      </section>
    </div>
  );
}

function SaleRow({
  sale,
  management,
  showState,
  onOpen,
}: {
  sale: SaleListItem;
  management: boolean;
  showState: boolean;
  onOpen: () => void;
}) {
  const done = completedTasks(sale);
  const due = dueText(sale);
  const task = sale.currentTask;
  const overdue = Boolean(task?.isOverdue && task.status === "in_progress");
  return (
    // The customer's name is the real link (keyboard, middle click); the rest of the row is a bigger target.
    <tr className={`is-link${overdue ? " is-overdue" : ""}`} onClick={onOpen}>
      <td className="cell-primary">
        <Link
          className="cell-title"
          to={`/sales/${sale.id}`}
          onClick={(event) => event.stopPropagation()}
        >
          {sale.customerName}
        </Link>
        <span className="cell-sub">
          <span className="code">{sale.code}</span>
          {sale.temperature === "hot" && <TempBadge temperature="hot" />}
        </span>
      </td>
      <td>
        <span className="vehicle-cell">
          {sale.imageUrl ? (
            <img src={sale.imageUrl} alt="" loading="lazy" />
          ) : (
            <span className="vehicle-fallback">
              <Icon name="car" size={18} />
            </span>
          )}
          <span>
            <strong className="cell-title">{sale.vehicleModel}</strong>
            <span className="cell-sub">
              {sale.vehicleVersion} · {sale.vehicleColor}
            </span>
          </span>
        </span>
      </td>
      {management && (
        <td>
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
      <td>
        {done === null ? (
          <span className="muted">—</span>
        ) : (
          <Progress done={done} />
        )}
      </td>
      <td>
        {task ? (
          <>
            <strong className="cell-title">
              {task.number}. {task.name}
            </strong>
            <span className="cell-sub">
              {(overdue || task.status !== "in_progress") && (
                <TaskBadge status={task.status} overdue={overdue} />
              )}
              {due && <span className={overdue ? "cell-late" : ""}>{due}</span>}
            </span>
          </>
        ) : (
          <span className="muted">
            {sale.state === "completed" ? "Entregada" : "—"}
          </span>
        )}
      </td>
      {showState && (
        <td className="cell-aside">
          <StateBadge state={sale.state} />
        </td>
      )}
    </tr>
  );
}
