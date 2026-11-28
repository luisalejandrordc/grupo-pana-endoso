import { useState } from "react";
import {
  useDebounced,
  Badge,
  EmptyState,
  ErrorState,
  Icon,
  InitialsTile,
  PageHeader,
  Pagination,
  SearchField,
  useSearchQuery,
  type Toast,
} from "@endosoflow/ui";
import type { Role } from "../../shared/types";
import type { ApiCustomer } from "./api";
import { CustomerFormModal, type FormMode } from "./CustomerFormModal";
import { customerTypeLabels, customerTypes } from "./labels";
import { useCustomerList } from "./queries";

const PAGE_SIZE = 20;

export function CustomersPage({
  role,
  toast,
}: {
  role: Role;
  toast: (toast: Toast) => void;
}) {
  const [query, setQuery] = useSearchQuery();
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
  const filtering = Boolean(search || typeFilter || status !== "all");

  return (
    <div className="stack">
      <PageHeader
        title="Directorio de clientes"
        description="Consulta y mantén la información de cada cliente y las operaciones vinculadas a él."
        actions={
          <button
            className="button button-primary"
            onClick={() => setModal({ kind: "create" })}
          >
            <Icon name="plus" size={17} /> Nuevo cliente
          </button>
        }
      />

      <section className="panel">
        <div className="toolbar">
          <SearchField
            value={query}
            onChange={setQuery}
            label="Buscar clientes"
            placeholder="Nombre o documento"
            shortcut
          />
          <div className="filter-bar">
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
          <span className="toolbar-count">
            {list.data ? `${total} registros` : "Cargando…"}
          </span>
        </div>

        {list.isError ? (
          <ErrorState
            title="No se pudo cargar la lista"
            message={
              list.error instanceof Error
                ? list.error.message
                : "Inténtalo nuevamente."
            }
            onRetry={() => list.refetch()}
          />
        ) : (
          <div className="table-scroll" aria-busy={list.isFetching}>
            <table className="table table-cards">
              <thead>
                <tr>
                  <th>Cliente</th>
                  <th>Documento</th>
                  <th>Contacto</th>
                  <th>Estado</th>
                  <th>Activación</th>
                </tr>
              </thead>
              <tbody>
                {list.isPending &&
                  Array.from({ length: 6 }, (_, i) => (
                    <tr key={i} aria-hidden="true">
                      {Array.from({ length: 5 }, (_, j) => (
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
          <EmptyState
            icon="users"
            title={filtering ? "Sin resultados" : "Aún no hay clientes"}
            description={
              filtering
                ? "No hay clientes que coincidan con la búsqueda o los filtros."
                : role === "sales_advisor"
                  ? "Los clientes aparecen aquí cuando los creas o cuando se te asigna una venta."
                  : "Crea el primer cliente con el botón «Nuevo cliente»."
            }
          />
        )}

        <Pagination
          page={page}
          pageSize={PAGE_SIZE}
          total={total}
          showSinglePage
          onPageChange={(next) => setPage(() => next)}
        />
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
    <tr className="is-link" onClick={onOpen}>
      <td className="cell-primary">
        <div className="entity-cell">
          <InitialsTile name={customer.displayName} />
          <div>
            <button
              className="cell-title"
              title="Ver y editar la ficha"
              onClick={(event) => {
                event.stopPropagation();
                onOpen();
              }}
            >
              {customer.displayName}
            </button>
            <span className="cell-sub">
              {customerTypeLabels[customer.customerType]}
            </span>
          </div>
        </div>
      </td>
      <td>
        <span className="cell-title">
          {customer.documentType} {customer.documentNumber}
        </span>
      </td>
      <td>
        <div className="contact-cell">
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
      <td className="cell-aside">
        <Badge tone={customer.isActive ? "done" : "neutral"}>
          {customer.isActive ? "Activo" : "Inactivo"}
        </Badge>
      </td>
      <td>
        <Badge plain tone={customer.activatedAt ? "done" : "warn"}>
          {customer.activatedAt ? "Activada" : "Pendiente"}
        </Badge>
      </td>
    </tr>
  );
}
