import { useState } from "react";
import { errorText } from "../../shared/api/errors";
import { formatDate } from "../../shared/lib/format";
import { roleLabel, roleOptions, roleShortLabel } from "../../shared/lib/roles";
import { useDebounced } from "../../shared/lib/useDebounced";
import type { Employee, EmployeeProfile, Role } from "../../shared/types";
import { Avatar } from "../../shared/ui/Avatar";
import { Badge } from "../../shared/ui/Badge";
import { EmptyState, ErrorState } from "../../shared/ui/EmptyState";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pagination } from "../../shared/ui/Pagination";
import { SearchField } from "../../shared/ui/SearchField";
import type { Toast } from "../../shared/ui/toast";
import { CreateEmployeeModal } from "./CreateEmployeeModal";
import { roleTone } from "./labels";
import { useEmployeeList } from "./queries";

const PAGE_SIZE = 20;

type Status = "all" | "active" | "inactive";

export function StaffPage({
  user,
  toast,
}: {
  user: EmployeeProfile;
  toast: (toast: Toast) => void;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<Status>("all");
  const [role, setRole] = useState<Role | "all">("all");
  const [page, setPage] = useState(1);
  const [creating, setCreating] = useState(false);
  const search = useDebounced(query.trim());

  // The server filters and pages: the browser only asks, and starts again from page one when the ask changes.
  const filterKey = `${search}|${status}|${role}`;
  const [pageFor, setPageFor] = useState(filterKey);
  if (pageFor !== filterKey) {
    setPageFor(filterKey);
    setPage(1);
  }

  const list = useEmployeeList(
    {
      q: search || undefined,
      status,
      role: role === "all" ? undefined : role,
      page,
      pageSize: PAGE_SIZE,
    },
    user.role === "system_admin",
  );
  const total = list.data?.total ?? 0;
  const filtering = search !== "" || status !== "all" || role !== "all";

  if (user.role !== "system_admin")
    return (
      <EmptyState
        page
        icon="lock"
        title="Acceso restringido"
        description="Solo el Administrador del sistema puede consultar y administrar a los colaboradores."
      />
    );

  return (
    <div className="stack">
      <PageHeader
        title="Equipo de trabajo"
        description="Gestiona las cuentas de los colaboradores y sus permisos de acceso."
        actions={
          <button
            className="button button-primary"
            onClick={() => setCreating(true)}
          >
            <Icon name="plus" size={17} /> Nuevo colaborador
          </button>
        }
      />

      <section className="panel">
        <div className="toolbar">
          <SearchField
            value={query}
            onChange={setQuery}
            label="Buscar colaboradores"
            placeholder="Nombre, documento o correo"
          />
          <div className="filter-bar">
            <label className="filter-control">
              <span>Rol</span>
              <select
                value={role}
                onChange={(event) =>
                  setRole(event.target.value as Role | "all")
                }
              >
                <option value="all">Todos los roles</option>
                {roleOptions.map((item) => (
                  <option key={item} value={item}>
                    {roleLabel[item]}
                  </option>
                ))}
              </select>
            </label>
            <label className="filter-control">
              <span>Estado</span>
              <select
                value={status}
                onChange={(event) => setStatus(event.target.value as Status)}
              >
                <option value="all">Todos</option>
                <option value="active">Activo</option>
                <option value="inactive">Inactivo</option>
              </select>
            </label>
          </div>
        </div>

        {list.isError ? (
          <ErrorState
            title="No se pudo cargar la lista"
            message={errorText(list.error)}
            onRetry={() => list.refetch()}
          />
        ) : (
          <div
            className={`table-scroll ${list.isFetching && !list.isPending ? "is-refreshing" : ""}`}
            aria-busy={list.isFetching}
          >
            <table className="table table-cards">
              <thead>
                <tr>
                  <th>Colaborador</th>
                  <th>Documento</th>
                  <th>Rol</th>
                  <th>Correo corporativo</th>
                  <th>Estado</th>
                  <th>Activación</th>
                </tr>
              </thead>
              <tbody>
                {list.isPending &&
                  Array.from({ length: 5 }, (_, i) => (
                    <tr key={i} aria-hidden="true">
                      {Array.from({ length: 6 }, (_, j) => (
                        <td key={j}>
                          <span className="skeleton-line" />
                        </td>
                      ))}
                    </tr>
                  ))}
                {list.data?.items.map((employee) => (
                  <EmployeeRow key={employee.id} employee={employee} />
                ))}
              </tbody>
            </table>
          </div>
        )}

        {list.data && total === 0 && (
          <EmptyState
            icon="users"
            title={filtering ? "Sin resultados" : "Aún no hay colaboradores"}
            description={
              filtering
                ? "No hay colaboradores que coincidan con la búsqueda o los filtros."
                : "Crea la primera cuenta con el botón «Nuevo colaborador»."
            }
          />
        )}

        <Pagination
          page={page}
          pageSize={PAGE_SIZE}
          total={total}
          noun="colaboradores"
          showSinglePage
          onPageChange={setPage}
        />
      </section>

      {creating && (
        <CreateEmployeeModal toast={toast} onClose={() => setCreating(false)} />
      )}
    </div>
  );
}

function EmployeeRow({ employee }: { employee: Employee }) {
  const name = `${employee.firstName} ${employee.lastName}`;
  return (
    <tr>
      <td className="cell-primary">
        <div className="entity-cell">
          <Avatar
            id={employee.id}
            name={name}
            hasPhoto={employee.hasPhoto}
            size={38}
          />
          <div>
            <span className="cell-title">{name}</span>
            <span className="cell-sub">
              Desde {formatDate(employee.hireDate)}
            </span>
          </div>
        </div>
      </td>
      <td>
        <span className="cell-title">
          {employee.documentType} {employee.documentNumber}
        </span>
      </td>
      <td>
        <Badge plain tone={roleTone[employee.role]}>
          {roleShortLabel[employee.role]}
        </Badge>
      </td>
      <td>
        <div className="contact-cell">
          <span>
            <Icon name="mail" size={13} />
            {employee.businessEmail}
          </span>
        </div>
      </td>
      <td className="cell-aside">
        <Badge tone={employee.isActive ? "done" : "neutral"}>
          {employee.isActive ? "Activo" : "Inactivo"}
        </Badge>
      </td>
      <td>
        <Badge plain tone={employee.activatedAt ? "done" : "warn"}>
          {employee.activatedAt ? "Activada" : "Pendiente"}
        </Badge>
      </td>
    </tr>
  );
}
