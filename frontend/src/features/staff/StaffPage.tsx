import { useEffect, useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import { formatDate } from "../../shared/lib/format";
import { Avatar } from "../../shared/ui/Avatar";
import { roleLabel, roleOptions, roleShortLabel } from "../../shared/lib/roles";
import {
  type DocumentType,
  type Employee,
  type EmployeeProfile,
  type Role,
} from "../../shared/types";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { Modal } from "../../shared/ui/Modal";
import { type Toast } from "../../shared/ui/toast";

export function StaffPage({
  user,
  toast,
}: {
  user: EmployeeProfile;
  toast: (toast: Toast) => void;
}) {
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<"all" | "active" | "inactive">("all");
  const [roleFilter, setRoleFilter] = useState("all");
  const [loading, setLoading] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({
    documentType: "DNI" as DocumentType,
    documentNumber: "",
    firstName: "",
    lastName: "",
    nationality: "Peruana",
    gender: "F" as "F" | "M",
    birthdate: "",
    hireDate: "",
    businessPhone: "",
    businessEmail: "",
    personalPhone: "",
    personalEmail: "",
    role: "sales_advisor" as Role,
  });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (user.role !== "system_admin") return;
    let alive = true;
    setLoading(true);
    api
      .listEmployees({
        page: 1,
        pageSize: 100,
        status: status === "all" ? "all" : status,
        ...(roleFilter === "all" ? {} : { role: roleFilter as Role }),
        ...(query ? { q: query } : {}),
      })
      .then((result) => {
        if (alive) setEmployees(result.items);
      })
      .catch((error) =>
        toast({
          title: "No se pudo cargar la lista",
          message:
            error instanceof Error ? error.message : "Inténtalo de nuevo.",
          tone: "error",
        }),
      )
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [user.role, status, roleFilter, query]);

  const filtered = employees.filter(
    (employee) =>
      `${employee.firstName} ${employee.lastName} ${employee.documentNumber} ${employee.businessEmail}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (status === "all" ||
        (status === "active" ? employee.isActive : !employee.isActive)) &&
      (roleFilter === "all" || employee.role === roleFilter),
  );

  async function createEmployee(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const result = await api.createEmployee(form);
      setEmployees((old) => [result.employee, ...old]);
      toast({
        title: "Empleado creado",
        message: result.activationLinkIssued
          ? "Se emitió el enlace de activación."
          : "No se pudo emitir el enlace; puedes reenviarlo más adelante.",
        tone: "success",
      });
      setShowCreate(false);
      setForm({
        documentType: "DNI",
        documentNumber: "",
        firstName: "",
        lastName: "",
        nationality: "Peruana",
        gender: "F",
        birthdate: "",
        hireDate: "",
        businessPhone: "",
        businessEmail: "",
        personalPhone: "",
        personalEmail: "",
        role: "sales_advisor",
      });
    } catch (error) {
      toast({
        title: "No se pudo crear el empleado",
        message: error instanceof Error ? error.message : "Revisa los datos.",
        tone: "error",
      });
    } finally {
      setSaving(false);
    }
  }

  if (user.role !== "system_admin")
    return (
      <div className="stack">
        <div className="permission-state">
          <div className="permission-icon">
            <Icon name="lock" size={24} />
          </div>
          <h2>Acceso restringido</h2>
          <p>
            Solo el Administrador del sistema puede consultar y administrar
            todos los empleados.
          </p>
        </div>
      </div>
    );

  return (
    <div className="stack">
      <header className="page-head">
        <div>
          <h1>Equipo de trabajo</h1>
          <p>Gestiona las cuentas de colaboradores y sus permisos de acceso.</p>
        </div>
        <button
          className="button button-primary"
          onClick={() => setShowCreate(true)}
        >
          <Icon name="plus" size={17} /> Nuevo empleado
        </button>
      </header>
      <section className="workspace-card">
        <div className="card-heading">
          <div>
            <h3>Colaboradores</h3>
            <p>Los cambios de rol y estado se validan en el backend.</p>
          </div>
          {loading && (
            <span className="loading-inline">
              <span className="tiny-spinner" /> Actualizando…
            </span>
          )}
        </div>
        <div className="staff-filter-row">
          <div className="input-with-icon staff-search">
            <Icon name="search" size={16} />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Nombre, documento o correo"
            />
          </div>
          <label className="filter-control">
            <span>Rol</span>
            <select
              value={roleFilter}
              onChange={(event) => setRoleFilter(event.target.value)}
            >
              <option value="all">Todos los roles</option>
              {roleOptions.map((role) => (
                <option key={role} value={role}>
                  {roleLabel[role]}
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
              <option value="active">Activos</option>
              <option value="inactive">Inactivos</option>
            </select>
          </label>
        </div>
        <div className="table-scroll">
          <table className="data-table">
            <thead>
              <tr>
                <th>COLABORADOR</th>
                <th>DOCUMENTO</th>
                <th>ROL</th>
                <th>CORREO CORPORATIVO</th>
                <th>ESTADO</th>
                <th>ACTIVACIÓN</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((employee) => (
                <tr key={employee.id}>
                  <td>
                    <div className="sale-customer-cell">
                      <div className="table-avatar">
                        <Avatar
                          id={employee.id}
                          name={`${employee.firstName} ${employee.lastName}`}
                          hasPhoto={employee.hasPhoto}
                          size={36}
                        />
                      </div>
                      <div>
                        <strong>
                          {employee.firstName} {employee.lastName}
                        </strong>
                        <span>Desde {formatDate(employee.hireDate)}</span>
                      </div>
                    </div>
                  </td>
                  <td>
                    <strong className="regular-strong">
                      {employee.documentType} {employee.documentNumber}
                    </strong>
                  </td>
                  <td>
                    <span className={`role-badge role-${employee.role}`}>
                      {roleShortLabel[employee.role]}
                    </span>
                  </td>
                  <td>
                    <span className="email-cell">
                      <Icon name="mail" size={14} />
                      {employee.businessEmail}
                    </span>
                  </td>
                  <td>
                    <span
                      className={`status-pill ${employee.isActive ? "status-active" : "status-inactive"}`}
                    >
                      <i />
                      {employee.isActive ? "Activo" : "Inactivo"}
                    </span>
                  </td>
                  <td>
                    {employee.activatedAt ? (
                      <span className="activation-status success">
                        <Icon name="check-circle" size={14} /> Activado
                      </span>
                    ) : (
                      <span className="activation-status pending">
                        <Icon name="clock" size={14} /> Pendiente
                      </span>
                    )}
                  </td>
                  <td>
                    <button
                      className="icon-button subtle"
                      title="Opciones"
                      onClick={() =>
                        toast({
                          title: `${employee.firstName} ${employee.lastName}`,
                          message:
                            "Edición, desactivación y reenvío de invitación se pueden conectar a las rutas de personal disponibles.",
                          tone: "info",
                        })
                      }
                    >
                      <Icon name="more" size={17} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="table-footer">
          <span>Mostrando {filtered.length} colaboradores</span>
        </div>
      </section>
      {showCreate && (
        <Modal
          title="Registrar empleado"
          description="Se enviará un enlace de activación al correo corporativo cuando el backend lo permita."
          onClose={() => setShowCreate(false)}
          footer={
            <>
              <button
                className="button button-secondary"
                onClick={() => setShowCreate(false)}
              >
                Cancelar
              </button>
              <button
                className="button button-primary"
                form="employee-form"
                type="submit"
                disabled={saving}
              >
                {saving ? "Guardando…" : "Crear empleado"}{" "}
                <Icon name="arrow-right" size={15} />
              </button>
            </>
          }
        >
          <form
            id="employee-form"
            onSubmit={createEmployee}
            className="create-employee-form"
          >
            <div className="form-grid-two">
              <Field label="Tipo de documento">
                <select
                  value={form.documentType}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      documentType: event.target.value as DocumentType,
                    })
                  }
                >
                  <option>DNI</option>
                  <option>CE</option>
                  <option>PAS</option>
                </select>
              </Field>
              <Field label="Número de documento">
                <input
                  required
                  value={form.documentNumber}
                  onChange={(event) =>
                    setForm({ ...form, documentNumber: event.target.value })
                  }
                />
              </Field>
              <Field label="Nombres">
                <input
                  required
                  value={form.firstName}
                  onChange={(event) =>
                    setForm({ ...form, firstName: event.target.value })
                  }
                />
              </Field>
              <Field label="Apellidos">
                <input
                  required
                  value={form.lastName}
                  onChange={(event) =>
                    setForm({ ...form, lastName: event.target.value })
                  }
                />
              </Field>
              <Field label="Nacionalidad">
                <input
                  required
                  value={form.nationality}
                  onChange={(event) =>
                    setForm({ ...form, nationality: event.target.value })
                  }
                />
              </Field>
              <Field label="Género">
                <select
                  value={form.gender}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      gender: event.target.value as "F" | "M",
                    })
                  }
                >
                  <option value="F">Femenino</option>
                  <option value="M">Masculino</option>
                </select>
              </Field>
              <Field label="Fecha de nacimiento">
                <input
                  required
                  type="date"
                  value={form.birthdate}
                  onChange={(event) =>
                    setForm({ ...form, birthdate: event.target.value })
                  }
                />
              </Field>
              <Field label="Fecha de contratación">
                <input
                  required
                  type="date"
                  value={form.hireDate}
                  onChange={(event) =>
                    setForm({ ...form, hireDate: event.target.value })
                  }
                />
              </Field>
              <Field label="Teléfono corporativo">
                <input
                  required
                  value={form.businessPhone}
                  onChange={(event) =>
                    setForm({ ...form, businessPhone: event.target.value })
                  }
                />
              </Field>
              <Field label="Correo corporativo">
                <input
                  required
                  type="email"
                  value={form.businessEmail}
                  onChange={(event) =>
                    setForm({ ...form, businessEmail: event.target.value })
                  }
                />
              </Field>
              <Field label="Teléfono personal (opcional)">
                <input
                  value={form.personalPhone}
                  onChange={(event) =>
                    setForm({ ...form, personalPhone: event.target.value })
                  }
                />
              </Field>
              <Field label="Correo personal (opcional)">
                <input
                  type="email"
                  value={form.personalEmail}
                  onChange={(event) =>
                    setForm({ ...form, personalEmail: event.target.value })
                  }
                />
              </Field>
              <Field label="Rol asignado" className="full-span">
                <select
                  value={form.role}
                  onChange={(event) =>
                    setForm({ ...form, role: event.target.value as Role })
                  }
                >
                  {roleOptions.map((role) => (
                    <option key={role} value={role}>
                      {roleLabel[role]}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
