import { errorText } from "@endosoflow/api";
import {
  Badge,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  formatDate,
  Icon,
  Menu,
  PageHeader,
  Pagination,
  SearchField,
  useDebounced,
  type MenuItem,
  type Toast,
} from "@endosoflow/ui";
import { useState } from "react";
import { roleLabel, roleOptions, roleShortLabel } from "../../shared/lib/roles";
import type { EmployeeProfile, Role } from "../../shared/types";
import { Avatar } from "../../shared/ui/Avatar";
import type { Employee } from "./api";
import { EmployeeFormModal, type EmployeeFormMode } from "./EmployeeFormModal";
import { roleTone } from "./labels";
import {
  useDeactivateEmployee,
  useEmployeeList,
  useReactivateEmployee,
  useResendActivation,
} from "./queries";

const PAGE_SIZE = 20;

type Status = "all" | "active" | "inactive";

const fullName = (employee: Employee) =>
  `${employee.firstName} ${employee.lastName}`;

export function StaffPage({
  user,
  toast,
  onSelfUpdated,
}: {
  user: EmployeeProfile;
  toast: (toast: Toast) => void;
  /** Called after the signed-in administrator edits their own record, so the session shows the new data. */
  onSelfUpdated?: () => void;
}) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<Status>("all");
  const [role, setRole] = useState<Role | "all">("all");
  const [page, setPage] = useState(1);
  const [form, setForm] = useState<EmployeeFormMode | null>(null);
  const [deactivating, setDeactivating] = useState<Employee | null>(null);
  const [deactivateError, setDeactivateError] = useState<string | null>(null);
  const search = useDebounced(query.trim());
  const isAdmin = user.role === "system_admin";

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
    isAdmin,
  );
  const deactivate = useDeactivateEmployee();
  const reactivate = useReactivateEmployee();
  const resend = useResendActivation();

  const total = list.data?.total ?? 0;
  const filtering = search !== "" || status !== "all" || role !== "all";

  async function confirmDeactivate() {
    if (!deactivating) return;
    setDeactivateError(null);
    try {
      await deactivate.mutateAsync(deactivating.id);
      toast({
        title: "Colaborador desactivado",
        message: `${fullName(deactivating)} ya no puede iniciar sesión.`,
        tone: "success",
      });
      setDeactivating(null);
    } catch (error) {
      setDeactivateError(errorText(error));
    }
  }

  async function reactivateEmployee(employee: Employee) {
    try {
      await reactivate.mutateAsync(employee.id);
      toast({
        title: "Colaborador reactivado",
        message: `${fullName(employee)} puede volver a iniciar sesión.`,
        tone: "success",
      });
    } catch (error) {
      toast({
        title: "No se pudo reactivar",
        message: errorText(error),
        tone: "error",
      });
    }
  }

  async function resendActivation(employee: Employee) {
    try {
      await resend.mutateAsync(employee.id);
      toast({
        title: "Enlace de activación enviado",
        message: `Se envió a ${employee.businessEmail}.`,
        tone: "success",
      });
    } catch (error) {
      toast({
        title: "No se pudo reenviar la activación",
        message: errorText(error),
        tone: "error",
      });
    }
  }

  /** What can be done to this person depends on their state: and nobody can lock themselves out. */
  function actionsFor(employee: Employee): MenuItem[] {
    const items: MenuItem[] = [
      {
        label: "Editar",
        icon: "edit",
        onSelect: () => setForm({ kind: "edit", employee }),
      },
    ];
    if (employee.isActive && !employee.activatedAt)
      items.push({
        label: "Reenviar activación",
        icon: "mail",
        onSelect: () => resendActivation(employee),
      });
    if (!employee.isActive)
      items.push({
        label: "Reactivar",
        icon: "refresh",
        onSelect: () => reactivateEmployee(employee),
      });
    else if (employee.id !== user.id)
      items.push({
        label: "Desactivar",
        icon: "circle-x",
        danger: true,
        onSelect: () => {
          setDeactivateError(null);
          setDeactivating(employee);
        },
      });
    return items;
  }

  if (!isAdmin)
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
            onClick={() => setForm({ kind: "create" })}
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
                  <th>
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {list.isPending &&
                  Array.from({ length: 5 }, (_, i) => (
                    <tr key={i} aria-hidden="true">
                      {Array.from({ length: 7 }, (_, j) => (
                        <td key={j}>
                          <span className="skeleton-line" />
                        </td>
                      ))}
                    </tr>
                  ))}
                {list.data?.items.map((employee) => (
                  <EmployeeRow
                    key={employee.id}
                    employee={employee}
                    isSelf={employee.id === user.id}
                    actions={actionsFor(employee)}
                    onEdit={() => setForm({ kind: "edit", employee })}
                  />
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

      {form && (
        <EmployeeFormModal
          // Another person (or another version of the same one) is a fresh form, not an edited one.
          key={form.kind === "edit" ? form.employee.id : "create"}
          mode={form}
          currentUserId={user.id}
          toast={toast}
          onClose={() => setForm(null)}
          onSaved={(saved) => {
            if (saved.id === user.id) onSelfUpdated?.();
          }}
        />
      )}

      {deactivating && (
        <ConfirmDialog
          title={`Desactivar a ${fullName(deactivating)}`}
          confirmLabel="Desactivar"
          tone="danger"
          busy={deactivate.isPending}
          error={deactivateError}
          onConfirm={confirmDeactivate}
          onClose={() => setDeactivating(null)}
        >
          <p>
            Se cerrarán sus sesiones activas y no podrá iniciar sesión mientras
            esté inactivo. Su historial se conserva y puedes reactivarlo cuando
            quieras.
          </p>
        </ConfirmDialog>
      )}
    </div>
  );
}

function EmployeeRow({
  employee,
  isSelf,
  actions,
  onEdit,
}: {
  employee: Employee;
  isSelf: boolean;
  actions: MenuItem[];
  onEdit: () => void;
}) {
  const name = fullName(employee);
  return (
    <tr
      className={`is-link${employee.isActive ? "" : " is-inactive"}`}
      onClick={onEdit}
    >
      <td className="cell-primary">
        <div className="entity-cell">
          <Avatar
            id={employee.id}
            name={name}
            hasPhoto={employee.hasPhoto}
            size={38}
          />
          <div>
            <button
              className="cell-title"
              title="Editar colaborador"
              onClick={(event) => {
                event.stopPropagation();
                onEdit();
              }}
            >
              {name}
            </button>
            <span className="cell-sub">
              Desde {formatDate(employee.hireDate)}
              {isSelf && <span className="chip">Tú</span>}
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
      <td>
        <Badge tone={employee.isActive ? "done" : "neutral"}>
          {employee.isActive ? "Activo" : "Inactivo"}
        </Badge>
      </td>
      <td>
        <Badge plain tone={employee.activatedAt ? "done" : "warn"}>
          {employee.activatedAt ? "Activada" : "Pendiente"}
        </Badge>
      </td>
      {/* A click inside the menu (which React sees as inside this cell) must not also open the row. */}
      <td className="cell-actions" onClick={(event) => event.stopPropagation()}>
        <Menu variant="icon" label={`Acciones de ${name}`} items={actions} />
      </td>
    </tr>
  );
}
