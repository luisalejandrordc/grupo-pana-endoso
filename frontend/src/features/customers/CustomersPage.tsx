import { useState } from "react";
import { useDebounced } from "../../shared/lib/useDebounced";
import { initials } from "../../shared/lib/format";
import type { Role } from "../../shared/types";
import { Icon } from "../../shared/ui/Icon";
import type { Toast } from "../../shared/ui/toast";
import type { ApiCustomer } from "./api";
import { CustomerFormModal, type FormMode } from "./CustomerFormModal";
import {
  customerTypeLabels,
  customerTypes,
  portalStatusLabels,
} from "./labels";
import { useCustomerList } from "./queries";

const PAGE_SIZE = 20;

export function CustomersPage({
  query,
  role,
  toast,
}: {
  query: string;
  role: Role;
  toast: (toast: Toast) => void;
}) {
  const [typeFilter, setTypeFilter] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "inactive">("all");
  const [pageState, setPageState] = useState({ key: "", page: 1 });
  const [modal, setModal] = useState<FormMode | null>(null);
  const search = useDebounced(query.trim());

  // A new search or filter starts again from the first page, without a wasted request for the old page.
  const filterKey = `${search}|${typeFilter}|${status}`;
  const page = pageState.key === filterKey ? pageState.page : 1;
  const setPage = (update: (current: number) => number) =>
    setPageState({ key: filterKey, page: update(page) });

  const list = useCustomerList({
    q: search,
    customerType: typeFilter,
    status,
    page,
    pageSize: PAGE_SIZE,
    sort: "displayName",
  });

  const total = list.data?.total ?? 0;
  const from = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const to = Math.min(page * PAGE_SIZE, total);
  const lastPage = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const filtering = Boolean(search || typeFilter || status !== "all");

  return (
    <div className="stack">
      <header className="page-head">
        <div>
          <h1>Directorio de clientes</h1>
          <p>
            Consulta y mantén la información de cada cliente y las operaciones
            vinculadas a él.
          </p>
        </div>
        <button
          className="button button-primary"
          onClick={() => setModal({ kind: "create" })}
        >
          <Icon name="plus" size={17} /> Nuevo cliente
        </button>
      </header>

      <section className="workspace-card">
        <div className="card-heading">
          <div>
            <h3>Clientes</h3>
            <p>
              Busca por nombre, número de documento o correo desde la barra
              superior.
            </p>
          </div>
          <span className="count-pill">
            {list.data ? `${total} registros` : "Cargando…"}
          </span>
        </div>
        <div className="filter-row">
          <label className="filter-control">
            <span>Tipo de cliente</span>
            <select
              value={typeFilter}
              onChange={(event) => setTypeFilter(event.target.value)}
            >
              <option value="">Todos los tipos</option>
              {customerTypes.map((type) => (
                <option key={type} value={type}>
                  {customerTypeLabels[type]}
                </option>
              ))}
            </select>
          </label>
          <label className="filter-control">
            <span>Estado</span>
            <select
              value={status}
              onChange={(event) =>
                setStatus(event.target.value as typeof status)
              }
            >
              <option value="all">Todos</option>
              <option value="active">Activo</option>
              <option value="inactive">Inactivo</option>
            </select>
          </label>
        </div>

        {list.isError ? (
          <div className="list-error" role="alert">
            <h3>No se pudo cargar la lista</h3>
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
          <div className="table-scroll" aria-busy={list.isFetching}>
            <table className="data-table">
              <thead>
                <tr>
                  <th>CLIENTE</th>
                  <th>DOCUMENTO</th>
                  <th>TIPO</th>
                  <th>CONTACTO</th>
                  <th>PORTAL</th>
                  <th>ESTADO</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {list.isPending &&
                  Array.from({ length: 6 }, (_, i) => (
                    <tr key={i} aria-hidden="true">
                      {Array.from({ length: 7 }, (_, j) => (
                        <td key={j}>
                          <span className="skeleton-line" />
                        </td>
                      ))}
                    </tr>
                  ))}
                {list.data?.items.map((customer) => (
                  <CustomerRow
                    key={customer.id}
                    customer={customer}
                    onOpen={() => setModal({ kind: "edit", customer })}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}

        {list.data && total === 0 && (
          <div className="empty-state">
            <h3>{filtering ? "Sin resultados" : "Aún no hay clientes"}</h3>
            <p>
              {filtering
                ? "No hay clientes que coincidan con la búsqueda o los filtros."
                : role === "sales_advisor"
                  ? "Los clientes aparecen aquí cuando los creas o cuando se te asigna una venta."
                  : "Crea el primer cliente con el botón «Nuevo cliente»."}
            </p>
          </div>
        )}

        <div className="table-footer">
          <span>
            {total > 0 ? (
              <>
                Mostrando{" "}
                <strong>
                  {from}–{to}
                </strong>{" "}
                de <strong>{total}</strong>
              </>
            ) : (
              "Sin registros"
            )}
          </span>
          <div className="pager-info">
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
        </div>
      </section>

      {modal && (
        <CustomerFormModal
          key={modal.kind === "edit" ? modal.customer.id : "create"}
          mode={modal}
          role={role}
          toast={toast}
          onClose={() => setModal(null)}
          onOpenExisting={(customer) => setModal({ kind: "edit", customer })}
          onSaved={(_customer, created) => {
            if (created) setModal(null);
          }}
        />
      )}
    </div>
  );
}

function CustomerRow({
  customer,
  onOpen,
}: {
  customer: ApiCustomer;
  onOpen: () => void;
}) {
  return (
    <tr>
      <td>
        <div className="sale-customer-cell">
          <div className="table-avatar">{initials(customer.displayName)}</div>
          <div>
            <strong>{customer.displayName}</strong>
            <span>ID · {customer.id.slice(0, 8).toUpperCase()}</span>
          </div>
        </div>
      </td>
      <td>
        <strong className="regular-strong">
          {customer.documentType} {customer.documentNumber}
        </strong>
      </td>
      <td>
        <span className="type-tag">
          {customerTypeLabels[customer.customerType]}
        </span>
      </td>
      <td>
        <div className="customer-contact-cell">
          <span>
            <Icon name="mail" size={13} />
            {customer.email}
          </span>
          <span>
            <Icon name="phone" size={13} />
            {customer.phone}
          </span>
        </div>
      </td>
      <td>
        <span
          className={`portal-pill ${customer.portalStatus === "active" ? "portal-active" : customer.portalStatus === "invited" ? "portal-invited" : ""}`}
        >
          {portalStatusLabels[customer.portalStatus]}
        </span>
      </td>
      <td>
        <span
          className={`status-pill ${customer.isActive ? "status-active" : "status-inactive"}`}
        >
          <i />
          {customer.isActive ? "Activo" : "Inactivo"}
        </span>
      </td>
      <td>
        <button
          className="row-open-button"
          aria-label={`Abrir ficha de ${customer.displayName}`}
          title="Ver y editar la ficha"
          onClick={onOpen}
        >
          <Icon name="arrow-up-right" size={16} />
        </button>
      </td>
    </tr>
  );
}
