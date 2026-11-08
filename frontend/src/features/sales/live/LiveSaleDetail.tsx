import { useEffect, useState } from "react";
import { customerTypeLabels } from "../../customers/labels";
import {
  formatDate,
  formatMoneyText,
  initials,
} from "../../../shared/lib/format";
import type { Role } from "../../../shared/types";
import { Field } from "../../../shared/ui/Field";
import { Icon } from "../../../shared/ui/Icon";
import type { Toast } from "../../../shared/ui/toast";
import { StatusPill, TaskPill, TempPill } from "../components/pills";
import { salesApi, type ApiSaleDetail, type ApiTask } from "./api";
import {
  CancelSaleDialog,
  DeleteSaleDialog,
  ReasonDialog,
  ReassignDialog,
} from "./dialogs";
import { errorText, shortId, stateLabels } from "./labels";
import { canCancel, canReopenSale, isManagement } from "./permissions";
import {
  useAdvisors,
  useSale,
  useSaleMutation,
  useSelectableCatalog,
  useTasks,
} from "./queries";
import { LiveDocumentsTab, LiveHistoryTab, LiveTasksTab } from "./LiveTabs";

type Tab = "Resumen" | "Tareas" | "Documentos" | "Notas" | "Historial";
const TABS: readonly Tab[] = [
  "Resumen",
  "Tareas",
  "Documentos",
  "Notas",
  "Historial",
];

type SaleDialog = "reassign" | "cancel" | "reopen" | "delete" | null;

export function LiveSaleDetail({
  saleId,
  role,
  userId,
  onBack,
  toast,
}: {
  saleId: string;
  role: Role;
  userId: string;
  onBack: () => void;
  toast: (toast: Toast) => void;
}) {
  const saleQuery = useSale(saleId);
  const tasksQuery = useTasks(saleId);
  const [tab, setTab] = useState<Tab>("Resumen");
  const [selected, setSelected] = useState<number | null>(null);

  const sale = saleQuery.data;
  const tasks = tasksQuery.data;
  const current = sale?.currentTaskNumber ?? null;

  // The task list follows the sale's current task, so finishing a task moves the screen to the next one.
  useEffect(() => {
    if (current !== null) setSelected(current);
    else if (sale) setSelected(13);
  }, [current, sale?.id, sale?.state]);

  if (saleQuery.isPending || tasksQuery.isPending)
    return (
      <div className="page-stack sale-detail-page">
        <div className="live-loading">Cargando la venta…</div>
      </div>
    );
  if (saleQuery.isError || tasksQuery.isError || !sale || !tasks)
    return (
      <div className="page-stack sale-detail-page">
        <div className="detail-back-row">
          <button className="back-link" onClick={onBack}>
            <Icon name="arrow-left" size={16} /> Volver a ventas
          </button>
        </div>
        <div className="list-error" role="alert">
          <h3>No se pudo abrir la venta</h3>
          <p>{errorText(saleQuery.error ?? tasksQuery.error)}</p>
          <button
            className="button button-secondary"
            onClick={() => {
              void saleQuery.refetch();
              void tasksQuery.refetch();
            }}
          >
            <Icon name="refresh" size={15} /> Reintentar
          </button>
        </div>
      </div>
    );

  const openTask = (number?: number) => {
    if (number !== undefined) setSelected(number);
    setTab("Tareas");
  };

  return (
    <div className="page-stack sale-detail-page">
      <div className="detail-back-row">
        <button className="back-link" onClick={onBack}>
          <Icon name="arrow-left" size={16} /> Volver a ventas
        </button>
        <span className="detail-separator">/</span>
        <span>Venta {shortId(sale.id)}</span>
      </div>
      <SaleBanner sale={sale} role={role} toast={toast} />
      <Hero
        sale={sale}
        role={role}
        userId={userId}
        onDeleted={onBack}
        toast={toast}
      />
      <Progress sale={sale} tasks={tasks} />
      <section className="workspace-card sale-detail-content">
        <div className="detail-tabs" role="tablist">
          {TABS.map((item) => (
            <button
              key={item}
              role="tab"
              aria-selected={tab === item}
              className={tab === item ? "active" : ""}
              onClick={() => setTab(item)}
            >
              {item === "Tareas" && <Icon name="list" size={16} />}
              {item === "Documentos" && <Icon name="file-text" size={16} />}
              {item === "Notas" && <Icon name="message" size={16} />}
              {item === "Historial" && <Icon name="history" size={16} />}
              {item}
              {item === "Tareas" && <span className="tab-count">13</span>}
            </button>
          ))}
        </div>
        {tab === "Resumen" && (
          <Summary sale={sale} tasks={tasks} onOpenTask={openTask} />
        )}
        {tab === "Tareas" && (
          <LiveTasksTab
            sale={sale}
            tasks={tasks}
            selected={selected ?? 1}
            onSelect={setSelected}
            role={role}
            userId={userId}
            toast={toast}
          />
        )}
        {tab === "Documentos" && (
          <LiveDocumentsTab
            saleId={sale.id}
            tasks={tasks}
            onOpenTask={openTask}
            toast={toast}
          />
        )}
        {tab === "Notas" && (
          <Notes
            sale={sale}
            editable={
              sale.state !== "cancelled" &&
              (isManagement(role) || sale.advisorId === userId)
            }
            toast={toast}
          />
        )}
        {tab === "Historial" && (
          <LiveHistoryTab saleId={sale.id} tasks={tasks} />
        )}
      </section>
    </div>
  );
}

function SaleBanner({
  sale,
  role,
  toast,
}: {
  sale: ApiSaleDetail;
  role: Role;
  toast: (toast: Toast) => void;
}) {
  const [dialog, setDialog] = useState(false);
  const reopen = useSaleMutation((reason: string) =>
    salesApi.reopen(sale.id, reason),
  );
  if (sale.state === "completed")
    return (
      <div className="sale-alert sale-alert-green">
        <Icon name="check-circle" size={18} />
        <div>
          <strong>¡Operación completada!</strong>
          <span>
            La venta cerró las 13 tareas del proceso
            {sale.completedAt ? ` el ${formatDate(sale.completedAt)}` : ""}.
          </span>
        </div>
      </div>
    );
  if (sale.state !== "cancelled") return null;
  return (
    <div className="sale-alert sale-alert-red">
      <Icon name="alert" size={18} />
      <div>
        <strong>Esta operación está cancelada</strong>
        <span>
          Se canceló en la tarea {sale.cancelledAtTaskNumber ?? "—"}
          {sale.cancellationText ? `: ${sale.cancellationText}` : ""}. Las
          tareas están en modo lectura.
        </span>
      </div>
      {canReopenSale(role, sale) && (
        <button
          className="button button-secondary button-sm"
          onClick={() => setDialog(true)}
        >
          Reabrir venta
        </button>
      )}
      {dialog && (
        <ReasonDialog
          title="Reabrir venta"
          description="La venta vuelve a estar activa y la tarea en curso conserva el tiempo que le quedaba."
          label="Motivo de la reapertura"
          confirmLabel="Reabrir venta"
          onClose={() => setDialog(false)}
          onConfirm={async (reason) => {
            await reopen.mutateAsync(reason);
            toast({ title: "Venta reabierta", tone: "success" });
          }}
        />
      )}
    </div>
  );
}

function Hero({
  sale,
  role,
  userId,
  onDeleted,
  toast,
}: {
  sale: ApiSaleDetail;
  role: Role;
  userId: string;
  onDeleted: () => void;
  toast: (toast: Toast) => void;
}) {
  const [menu, setMenu] = useState(false);
  const [dialog, setDialog] = useState<SaleDialog>(null);
  const catalog = useSelectableCatalog();
  const advisors = useAdvisors(dialog === "reassign");
  const image = catalog.data
    ?.flatMap((model) => model.colors)
    .find((color) => color.id === sale.colorId)?.mainImageUrl;

  const reassign = useSaleMutation((advisorId: string) =>
    salesApi.reassign(sale.id, advisorId),
  );
  const cancel = useSaleMutation(
    (input: {
      category: Parameters<typeof salesApi.cancel>[1];
      text: string;
    }) => salesApi.cancel(sale.id, input.category, input.text),
  );
  const reopen = useSaleMutation((reason: string) =>
    salesApi.reopen(sale.id, reason),
  );
  const remove = useSaleMutation(
    (input: { reason: string; confirmation: string }) =>
      salesApi.remove(sale.id, input.reason, input.confirmation),
  );

  const management = isManagement(role);
  const mayCancel = canCancel(role, userId, sale);
  const mayReopen = canReopenSale(role, sale);
  const mayDelete = role === "system_admin" && sale.state !== "completed";
  const open = (next: SaleDialog) => {
    setMenu(false);
    setDialog(next);
  };

  return (
    <section className="sale-hero-card">
      <div className="sale-hero-main">
        <div className="sale-hero-avatar">{initials(sale.customerName)}</div>
        <div className="sale-hero-copy">
          <div className="sale-code-row">
            <span className="sale-code">Venta {shortId(sale.id)}</span>
            <StatusPill state={sale.state} />
            <TempPill temperature={sale.temperature} />
          </div>
          <h2>{sale.customerName}</h2>
          <p>
            {sale.customerDocumentType} {sale.customerDocumentNumber}{" "}
            <span className="dot-separator">·</span>{" "}
            {customerTypeLabels[
              sale.customerType as keyof typeof customerTypeLabels
            ] ?? sale.customerType}
          </p>
          <div className="sale-hero-meta">
            <span>
              <Icon name="user" size={14} /> {sale.advisorName}
            </span>
            <span>
              <Icon name="calendar" size={14} /> Creada el{" "}
              {formatDate(sale.createdAt)}
            </span>
          </div>
        </div>
      </div>
      <div className="sale-hero-vehicle">
        {image ? (
          <img src={image} alt={sale.modelName} />
        ) : (
          <div className="live-vehicle-placeholder">
            <Icon name="car" size={26} />
          </div>
        )}
        <div>
          <span>UNIDAD ASOCIADA</span>
          <strong>{sale.modelName}</strong>
          <small>
            {sale.versionName} <b>·</b> {sale.colorName}
          </small>
        </div>
      </div>
      <div className="sale-hero-actions">
        <div className="price-block">
          <span>PRECIO DE REFERENCIA</span>
          <strong>{formatMoneyText(sale.referencePrice)}</strong>
        </div>
        <div className="action-menu-wrap">
          <button
            className="button button-secondary"
            onClick={() => setMenu(!menu)}
          >
            <Icon name="dots" size={18} /> Acciones{" "}
            <Icon name="chevron-down" size={14} />
          </button>
          {menu && (
            <div className="action-menu">
              {management && sale.state === "active" && (
                <button onClick={() => open("reassign")}>
                  <Icon name="users" size={16} /> Reasignar asesor
                </button>
              )}
              {mayReopen && (
                <button onClick={() => open("reopen")}>
                  <Icon name="refresh" size={16} /> Reabrir venta
                </button>
              )}
              {mayCancel && (
                <button
                  className="danger-menu-item"
                  onClick={() => open("cancel")}
                >
                  <Icon name="circle-x" size={16} /> Cancelar venta
                </button>
              )}
              {sale.state === "active" && !mayCancel && (
                <div className="action-menu-note">
                  {role === "sales_advisor"
                    ? "Solicita a Gerencia la cancelación a partir de la tarea 4."
                    : "Solo un administrador puede cancelar después de la tarea 6."}
                </div>
              )}
              {mayDelete && (
                <button
                  className="danger-menu-item"
                  onClick={() => open("delete")}
                >
                  <Icon name="circle-x" size={16} /> Eliminar definitivamente
                </button>
              )}
              {!management && sale.state === "active" && (
                <div className="action-menu-note">
                  Solo Gerencia puede reasignar la venta.
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {dialog === "reassign" && (
        <ReassignDialog
          sale={sale}
          advisors={advisors.data ?? []}
          loading={advisors.isPending}
          onClose={() => setDialog(null)}
          onConfirm={async (advisorId) => {
            await reassign.mutateAsync(advisorId);
            toast({ title: "Venta reasignada", tone: "success" });
          }}
        />
      )}
      {dialog === "cancel" && (
        <CancelSaleDialog
          onClose={() => setDialog(null)}
          onConfirm={async (category, text) => {
            await cancel.mutateAsync({ category, text });
            toast({
              title: "Venta cancelada",
              message: "Las tareas quedaron en modo lectura.",
              tone: "info",
            });
          }}
        />
      )}
      {dialog === "reopen" && (
        <ReasonDialog
          title="Reabrir venta"
          description="La venta vuelve a estar activa y la tarea en curso conserva el tiempo que le quedaba."
          label="Motivo de la reapertura"
          confirmLabel="Reabrir venta"
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            await reopen.mutateAsync(reason);
            toast({ title: "Venta reabierta", tone: "success" });
          }}
        />
      )}
      {dialog === "delete" && (
        <DeleteSaleDialog
          sale={sale}
          onClose={() => setDialog(null)}
          onConfirm={async (reason, confirmation) => {
            await remove.mutateAsync({ reason, confirmation });
            toast({ title: "Venta eliminada", tone: "success" });
            onDeleted();
          }}
        />
      )}
    </section>
  );
}

function Progress({ sale, tasks }: { sale: ApiSaleDetail; tasks: ApiTask[] }) {
  const progress = Math.round((sale.completedTasks / 13) * 100);
  const doneByMilestone = (number: number) =>
    tasks.filter(
      (task) => task.milestoneNumber === number && task.status === "completed",
    ).length;
  const totalByMilestone = (number: number) =>
    tasks.filter((task) => task.milestoneNumber === number).length;
  return (
    <section className="progress-card">
      <div className="progress-head">
        <div>
          <span className="section-kicker">PROGRESO DE LA OPERACIÓN</span>
          <h3>Ruta de endoso</h3>
        </div>
        <div className="progress-percent">
          <strong>{progress}%</strong>
          <span>completado</span>
        </div>
      </div>
      <div className="progress-track">
        <div style={{ width: `${progress}%` }} />
      </div>
      <div className="milestone-grid">
        {sale.milestones.map((milestone) => (
          <div
            className={`milestone ${milestone.state === "completed" ? "done" : milestone.state === "in_progress" ? "current" : ""}`}
            key={milestone.number}
          >
            <div className="milestone-node">
              {milestone.state === "completed" ? (
                <Icon name="check" size={15} />
              ) : (
                <span>{String(milestone.number).padStart(2, "0")}</span>
              )}
            </div>
            <div className="milestone-label">
              <strong>{milestone.name}</strong>
              <span>
                {doneByMilestone(milestone.number)} de{" "}
                {totalByMilestone(milestone.number)} tareas
              </span>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function Summary({
  sale,
  tasks,
  onOpenTask,
}: {
  sale: ApiSaleDetail;
  tasks: ApiTask[];
  onOpenTask: (number?: number) => void;
}) {
  const active =
    sale.state === "active"
      ? tasks.find((task) => task.number === sale.currentTaskNumber)
      : undefined;
  const mine = [...tasks]
    .filter((task) => task.status !== "pending" || task.eligible)
    .slice(-4);
  return (
    <div className="detail-summary-grid">
      <div className="summary-main-column">
        <div className="next-action-card">
          <div className="next-action-icon">
            <Icon
              name={active?.status === "in_progress" ? "clock" : "check-circle"}
              size={22}
            />
          </div>
          <div className="next-action-copy">
            <span className="section-kicker">SIGUIENTE ACCIÓN</span>
            <h3>
              {sale.state !== "active"
                ? sale.state === "completed"
                  ? "Proceso finalizado"
                  : "Venta cancelada"
                : (active?.name ?? "Listo para avanzar")}
            </h3>
            <p>
              {sale.state !== "active"
                ? "No hay tareas editables mientras la venta permanezca en este estado."
                : active?.status === "in_progress"
                  ? `La tarea ${active.number} está en curso. Completa sus datos y documentos para habilitar la siguiente.`
                  : `La tarea ${active?.number ?? 13} está lista para iniciar.`}
            </p>
          </div>
          {active && (
            <button
              className="button button-primary button-sm"
              onClick={() => onOpenTask(active.number)}
            >
              {active.status === "in_progress"
                ? "Continuar tarea"
                : "Ir a la tarea"}{" "}
              <Icon name="arrow-right" size={14} />
            </button>
          )}
        </div>
        <div className="subsection-heading">
          <h3>Resumen de la operación</h3>
          <span>Estado: {stateLabels[sale.state]}</span>
        </div>
        <div className="summary-data-grid">
          <div className="summary-data-item">
            <span>Cliente</span>
            <strong>{sale.customerName}</strong>
            <small>
              {customerTypeLabels[
                sale.customerType as keyof typeof customerTypeLabels
              ] ?? sale.customerType}
            </small>
          </div>
          <div className="summary-data-item">
            <span>Documento</span>
            <strong>
              {sale.customerDocumentType} {sale.customerDocumentNumber}
            </strong>
            <small>Datos al crear la venta</small>
          </div>
          <div className="summary-data-item">
            <span>Asesor asignado</span>
            <strong>{sale.advisorName}</strong>
            <small>Responsable de la operación</small>
          </div>
          <div className="summary-data-item">
            <span>Fecha de creación</span>
            <strong>{formatDate(sale.createdAt)}</strong>
            <small>Hora local de Lima</small>
          </div>
          <div className="summary-data-item">
            <span>Serie y VIN</span>
            <strong>{sale.vehicleSeries ?? "—"}</strong>
            <small>{sale.vin ?? "Aún sin VIN"}</small>
          </div>
          <div className="summary-data-item">
            <span>Placa</span>
            <strong>{sale.licensePlate ?? "—"}</strong>
            <small>Se registra en la tarea 10</small>
          </div>
        </div>
        <div className="subsection-heading progress-subheading">
          <h3>Avance de tareas</h3>
          <button className="text-button" onClick={() => onOpenTask()}>
            Ver las 13 tareas <Icon name="arrow-right" size={14} />
          </button>
        </div>
        <div className="mini-task-list">
          {mine.map((task) => (
            <button
              key={task.number}
              className="mini-task-row"
              onClick={() => onOpenTask(task.number)}
            >
              <span
                className={`mini-task-icon ${task.status === "pending" ? "eligible" : task.status}`}
              >
                {task.status === "completed" ? (
                  <Icon name="check" size={14} />
                ) : (
                  String(task.number).padStart(2, "0")
                )}
              </span>
              <span className="mini-task-name">
                <strong>{task.name}</strong>
                <small>Tarea {task.number}</small>
              </span>
              <TaskPill
                status={task.status === "pending" ? "eligible" : task.status}
              />
              <Icon name="chevron-right" size={15} />
            </button>
          ))}
        </div>
      </div>
      <aside className="summary-side-column">
        <div className="side-info-card">
          <div className="side-card-title">
            <h3>Vehículo</h3>
            <Icon name="car" size={17} />
          </div>
          <strong className="summary-vehicle-name">{sale.modelName}</strong>
          <span className="summary-vehicle-version">{sale.versionName}</span>
          <div className="vehicle-spec-row">
            <span>Color</span>
            <strong>{sale.colorName}</strong>
          </div>
          <div className="vehicle-spec-row">
            <span>Precio referencial</span>
            <strong>{formatMoneyText(sale.referencePrice)}</strong>
          </div>
        </div>
      </aside>
    </div>
  );
}

function Notes({
  sale,
  editable,
  toast,
}: {
  sale: ApiSaleDetail;
  editable: boolean;
  toast: (toast: Toast) => void;
}) {
  const [value, setValue] = useState(sale.notes ?? "");
  const [error, setError] = useState<string | null>(null);
  const save = useSaleMutation((notes: string) =>
    salesApi.updateNotes(
      sale.id,
      sale.version,
      notes.trim() === "" ? null : notes,
    ),
  );
  useEffect(() => setValue(sale.notes ?? ""), [sale.version]);
  const dirty = value.trim() !== (sale.notes ?? "").trim();
  return (
    <div className="notes-tab">
      <div className="subsection-heading">
        <div>
          <h3>Notas internas</h3>
          <p>
            Información para el equipo comercial. No visible para el cliente.
          </p>
        </div>
        <span className="internal-badge">
          <Icon name="lock" size={13} /> Solo interno
        </span>
      </div>
      <Field label="Notas de la operación" error={error ?? undefined}>
        <textarea
          rows={7}
          value={value}
          disabled={!editable}
          maxLength={2000}
          onChange={(event) => setValue(event.target.value)}
          placeholder="Agrega contexto, acuerdos o información para el equipo…"
        />
      </Field>
      <div className="notes-footer">
        <span>
          {editable
            ? "Las notas no modifican los datos originales de la venta."
            : "La venta está cancelada: las notas están en modo lectura."}
        </span>
        {editable && (
          <button
            className="button button-primary button-sm"
            disabled={!dirty || save.isPending}
            onClick={async () => {
              setError(null);
              try {
                await save.mutateAsync(value);
                toast({ title: "Notas guardadas", tone: "success" });
              } catch (caught) {
                setError(errorText(caught));
              }
            }}
          >
            Guardar notas <Icon name="check" size={15} />
          </button>
        )}
      </div>
    </div>
  );
}
