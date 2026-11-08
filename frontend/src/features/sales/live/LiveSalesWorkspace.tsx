import { useEffect, useState } from "react";
import { initials } from "../../../shared/lib/format";
import { useDebounced } from "../../../shared/lib/useDebounced";
import type { Role } from "../../../shared/types";
import { Icon } from "../../../shared/ui/Icon";
import { MetricCard } from "../../../shared/ui/MetricCard";
import { StatusPill, TempPill } from "../components/pills";
import type { SaleListParams } from "./api";
import {
  TASK_NAMES,
  errorText,
  shortId,
  stateLabels,
  temperatureLabels,
} from "./labels";
import { useAdvisors, useSaleList, useSaleTotal } from "./queries";

const PAGE_SIZE = 10;

function today() {
  const text = new Intl.DateTimeFormat("es-PE", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "America/Lima",
  }).format(new Date());
  return text.toLocaleUpperCase("es-PE");
}

export function LiveSalesWorkspace({
  role,
  query,
  onCreate,
  onOpen,
}: {
  role: Role;
  query: string;
  onCreate: () => void;
  onOpen: (saleId: string) => void;
}) {
  const [state, setState] = useState("all");
  const [task, setTask] = useState("all");
  const [temperature, setTemperature] = useState("all");
  const [advisor, setAdvisor] = useState("all");
  const [newest, setNewest] = useState(true);
  const [page, setPage] = useState(1);
  const search = useDebounced(query.trim());
  const advisors = useAdvisors(role !== "sales_advisor");

  // Any change to what is asked for starts again from the first page.
  useEffect(
    () => setPage(1),
    [search, state, task, temperature, advisor, newest],
  );

  const params: SaleListParams = {
    q: search || undefined,
    state: state === "all" ? undefined : state,
    currentTask: task === "all" ? undefined : Number(task),
    temperature: temperature === "all" ? undefined : temperature,
    advisorId: advisor === "all" ? undefined : advisor,
    sort: newest ? "-createdAt" : "customerName",
    page,
    pageSize: PAGE_SIZE,
  };
  const list = useSaleList(params);
  const active = useSaleTotal({ state: "active" });
  const completed = useSaleTotal({ state: "completed" });
  const cancelled = useSaleTotal({ state: "cancelled" });
  const hot = useSaleTotal({ state: "active", temperature: "hot" });

  const total = list.data?.total ?? 0;
  const items = list.data?.items ?? [];
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const first = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const last = Math.min(page * PAGE_SIZE, total);
  const filtered =
    state !== "all" ||
    task !== "all" ||
    temperature !== "all" ||
    advisor !== "all" ||
    search !== "";
  const clear = () => {
    setState("all");
    setTask("all");
    setTemperature("all");
    setAdvisor("all");
  };

  return (
    <div className="page-stack">
      <div className="welcome-row">
        <div>
          <div className="section-kicker">{today()}</div>
          <h2>Hola, vamos a avanzar.</h2>
          <p>
            Este es el estado de las operaciones comerciales
            {role === "sales_advisor" ? " que tienes asignadas" : ""}.
          </p>
        </div>
        <button className="button button-primary" onClick={onCreate}>
          <Icon name="plus" size={18} /> Nueva venta
        </button>
      </div>
      <div className="metrics-grid">
        <MetricCard
          label="Ventas en curso"
          value={active.data ?? "—"}
          detail="operaciones activas"
          icon="sales"
          tone="metric-green"
        />
        <MetricCard
          label="Prioridad alta"
          value={hot.data ?? "—"}
          detail="en curso con interés alto"
          icon="alert"
          tone="metric-amber"
        />
        <MetricCard
          label="Ventas completadas"
          value={completed.data ?? "—"}
          detail="cerradas con éxito"
          icon="check-circle"
          tone="metric-purple"
        />
        <MetricCard
          label="Ventas canceladas"
          value={cancelled.data ?? "—"}
          detail="en modo lectura"
          icon="clock"
          tone="metric-blue"
        />
      </div>
      <section className="workspace-card sales-table-card">
        <div className="card-heading">
          <div>
            <h3>Seguimiento de ventas</h3>
            <p>Controla las operaciones y encuentra la siguiente acción.</p>
          </div>
          <div className="table-heading-actions">
            <span className="count-pill">{total} registros</span>
            <button
              className="button button-secondary button-sm"
              onClick={clear}
              disabled={!filtered}
            >
              <Icon name="filter-x" size={15} /> Limpiar filtros
            </button>
          </div>
        </div>
        <div className="filter-row">
          <label className="filter-control">
            <span>Estado</span>
            <select
              value={state}
              onChange={(event) => setState(event.target.value)}
            >
              <option value="all">Todos los estados</option>
              {(Object.keys(stateLabels) as (keyof typeof stateLabels)[]).map(
                (value) => (
                  <option key={value} value={value}>
                    {stateLabels[value]}
                  </option>
                ),
              )}
            </select>
          </label>
          <label className="filter-control">
            <span>Tarea actual</span>
            <select
              value={task}
              onChange={(event) => setTask(event.target.value)}
            >
              <option value="all">Todas las tareas</option>
              {TASK_NAMES.map((name, index) => (
                <option key={name} value={String(index + 1)}>
                  {index + 1}. {name}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-control">
            <span>Prioridad</span>
            <select
              value={temperature}
              onChange={(event) => setTemperature(event.target.value)}
            >
              <option value="all">Todas</option>
              {(
                Object.keys(
                  temperatureLabels,
                ) as (keyof typeof temperatureLabels)[]
              ).map((value) => (
                <option key={value} value={value}>
                  {temperatureLabels[value]}
                </option>
              ))}
            </select>
          </label>
          {role !== "sales_advisor" && (
            <label className="filter-control">
              <span>Asesor</span>
              <select
                value={advisor}
                onChange={(event) => setAdvisor(event.target.value)}
              >
                <option value="all">Todos los asesores</option>
                {(advisors.data ?? []).map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.firstName} {item.lastName}
                  </option>
                ))}
              </select>
            </label>
          )}
          <button className="sort-button" onClick={() => setNewest(!newest)}>
            <Icon name="calendar" size={16} />
            {newest ? "Más recientes" : "Cliente A–Z"}
            <Icon name="chevron-down" size={14} />
          </button>
        </div>

        {list.isError && (
          <div className="list-error" role="alert">
            <h3>No se pudieron cargar las ventas</h3>
            <p>{errorText(list.error)}</p>
            <button
              className="button button-secondary"
              onClick={() => list.refetch()}
            >
              <Icon name="refresh" size={15} /> Reintentar
            </button>
          </div>
        )}
        {list.isPending && <div className="live-loading">Cargando ventas…</div>}

        {list.data && items.length > 0 && (
          <div className="table-scroll">
            <table className="data-table sales-table">
              <thead>
                <tr>
                  <th>CLIENTE / OPERACIÓN</th>
                  <th>VEHÍCULO</th>
                  <th>ASESOR</th>
                  <th>PRIORIDAD</th>
                  <th>TAREA ACTUAL</th>
                  <th>ESTADO</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {items.map((sale) => {
                  const number = sale.currentTaskNumber;
                  return (
                    <tr
                      key={sale.id}
                      onClick={() => onOpen(sale.id)}
                      tabIndex={0}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") onOpen(sale.id);
                      }}
                    >
                      <td>
                        <div className="sale-customer-cell">
                          <div className="table-avatar">
                            {initials(sale.customerName)}
                          </div>
                          <div>
                            <strong>{sale.customerName}</strong>
                            <span>Venta {shortId(sale.id)}</span>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="vehicle-cell">
                          <div>
                            <strong>{sale.modelName}</strong>
                            <span>
                              {sale.versionName} · {sale.colorName}
                            </span>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="advisor-cell">
                          <span className="mini-avatar">
                            {initials(sale.advisorName)}
                          </span>
                          <span>{sale.advisorName}</span>
                        </div>
                      </td>
                      <td>
                        <TempPill temperature={sale.temperature} />
                      </td>
                      <td>
                        <div className="current-task-cell">
                          <span className="task-number">
                            {number ? String(number).padStart(2, "0") : "13"}
                          </span>
                          <div>
                            <strong>
                              {number
                                ? TASK_NAMES[number - 1]
                                : "Proceso finalizado"}
                            </strong>
                            <span>
                              {sale.state !== "active"
                                ? `${sale.completedTasks} de 13 completadas`
                                : sale.currentTaskStatus === "in_progress"
                                  ? "En progreso"
                                  : "Lista para iniciar"}
                            </span>
                          </div>
                        </div>
                      </td>
                      <td>
                        <StatusPill state={sale.state} />
                      </td>
                      <td>
                        <button
                          className="row-open-button"
                          aria-label={`Abrir la venta de ${sale.customerName}`}
                          onClick={(event) => {
                            event.stopPropagation();
                            onOpen(sale.id);
                          }}
                        >
                          <Icon name="chevron-right" size={17} />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {list.data && items.length === 0 && (
          <div className="empty-state">
            <div className="empty-icon">
              <Icon name="search" size={23} />
            </div>
            <h3>{filtered ? "No encontramos ventas" : "Aún no hay ventas"}</h3>
            <p>
              {filtered
                ? "Prueba cambiando la búsqueda o ajustando los filtros."
                : "Registra la primera venta para empezar."}
            </p>
            {filtered ? (
              <button className="button button-secondary" onClick={clear}>
                Restablecer filtros
              </button>
            ) : (
              <button className="button button-primary" onClick={onCreate}>
                <Icon name="plus" size={16} /> Nueva venta
              </button>
            )}
          </div>
        )}

        <div className="table-footer">
          <span>
            Mostrando{" "}
            <strong>
              {first}–{last}
            </strong>{" "}
            de <strong>{total}</strong> registros
          </span>
          <div className="pagination">
            <button
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
              aria-label="Página anterior"
            >
              <Icon name="chevron-right" size={15} className="flip-icon" />
            </button>
            <span>
              {page} / {pages}
            </span>
            <button
              disabled={page >= pages}
              onClick={() => setPage(page + 1)}
              aria-label="Página siguiente"
            >
              <Icon name="chevron-right" size={15} />
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}
