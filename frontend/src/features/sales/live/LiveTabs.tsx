import { useState } from "react";
import { formatDateTime } from "../../../shared/lib/format";
import type { Role } from "../../../shared/types";
import { Icon } from "../../../shared/ui/Icon";
import type { Toast } from "../../../shared/ui/toast";
import { TimelineItem } from "../components/TimelineItem";
import { documentsApi, type ApiSaleDetail, type ApiTask } from "./api";
import { TaskWorkspace } from "./TaskWorkspace";
import { describeChanges, describeEvent, errorText } from "./labels";
import { useDocuments, useHistory } from "./queries";

const statusText = (task: ApiTask) =>
  task.status === "completed"
    ? "Completada"
    : task.status === "in_progress"
      ? "En progreso"
      : task.eligible
        ? "Lista para iniciar"
        : "Pendiente";

export function LiveTasksTab({
  sale,
  tasks,
  selected,
  onSelect,
  role,
  userId,
  toast,
}: {
  sale: ApiSaleDetail;
  tasks: ApiTask[];
  selected: number;
  onSelect: (number: number) => void;
  role: Role;
  userId: string;
  toast: (toast: Toast) => void;
}) {
  const completed = tasks.filter((task) => task.status === "completed").length;
  return (
    <div className="tasks-tab-layout">
      <div className="tasks-list-column">
        <div className="subsection-heading">
          <div>
            <h3>Tareas de la operación</h3>
            <p>{completed} de 13 tareas completadas</p>
          </div>
          <span className="count-pill">{tasks.length} tareas</span>
        </div>
        <div className="task-accordion-list">
          {tasks.map((task) => {
            const display =
              task.status === "pending" && task.eligible
                ? "eligible"
                : task.status;
            return (
              <button
                key={task.number}
                className={`task-accordion-row ${selected === task.number ? "selected" : ""}`}
                onClick={() => onSelect(task.number)}
              >
                <span className={`accordion-task-number ${display}`}>
                  {task.status === "completed" ? (
                    <Icon name="check" size={14} />
                  ) : (
                    String(task.number).padStart(2, "0")
                  )}
                </span>
                <div className="accordion-task-text">
                  <strong>{task.name}</strong>
                  <small>{statusText(task)}</small>
                </div>
                <Icon name="chevron-right" size={16} />
              </button>
            );
          })}
        </div>
      </div>
      <div className="task-detail-column">
        <TaskWorkspace
          sale={sale}
          number={selected}
          tasks={tasks}
          role={role}
          userId={userId}
          toast={toast}
        />
      </div>
    </div>
  );
}

/** Every file of the sale, by task, with the same download as the task screen. */
export function LiveDocumentsTab({
  saleId,
  tasks,
  onOpenTask,
  toast,
}: {
  saleId: string;
  tasks: ApiTask[];
  onOpenTask: (number: number) => void;
  toast: (toast: Toast) => void;
}) {
  const slots = useDocuments(saleId);
  if (slots.isPending)
    return <div className="live-loading">Cargando documentos…</div>;
  if (slots.isError)
    return (
      <div className="list-error" role="alert">
        <h3>No se pudieron cargar los documentos</h3>
        <p>{errorText(slots.error)}</p>
      </div>
    );
  const items = slots.data;
  const uploaded = items.filter((slot) => slot.file).length;
  const required = items.filter((slot) => slot.required);
  const missing = required.filter((slot) => !slot.file).length;

  async function download(fileId: string) {
    try {
      const result = await documentsApi.downloadUrl(fileId);
      window.open(result.url, "_blank", "noopener,noreferrer");
    } catch (error) {
      toast({
        title: "No se pudo abrir el documento",
        message: errorText(error),
        tone: "error",
      });
    }
  }

  return (
    <div className="live-docs-tab">
      <div className="subsection-heading">
        <div>
          <h3>Documentos de la operación</h3>
          <p>
            {uploaded} cargados · {missing} obligatorios pendientes
          </p>
        </div>
      </div>
      {tasks
        .filter((task) => items.some((slot) => slot.taskNumber === task.number))
        .map((task) => (
          <div className="live-docs" key={task.number}>
            <div className="live-section-title">
              <h4>
                Tarea {task.number} · {task.name}
              </h4>
              <button
                className="text-button"
                onClick={() => onOpenTask(task.number)}
              >
                Abrir tarea <Icon name="arrow-right" size={14} />
              </button>
            </div>
            {items
              .filter((slot) => slot.taskNumber === task.number)
              .map((slot) => (
                <div className="live-doc-row" key={slot.slotCode}>
                  <span
                    className={`document-icon ${slot.file ? "uploaded" : ""}`}
                  >
                    <Icon
                      name={slot.file ? "file-check" : "file-text"}
                      size={17}
                    />
                  </span>
                  <div className="live-doc-copy">
                    <strong>
                      {slot.label}
                      {slot.required && (
                        <em className="live-required"> · obligatorio</em>
                      )}
                    </strong>
                    <span>
                      {slot.file
                        ? `${slot.file.fileName} · ${slot.file.uploadedBy} · ${formatDateTime(slot.file.uploadedAt)}`
                        : "Sin archivo"}
                    </span>
                  </div>
                  <div className="live-doc-actions">
                    {slot.file && (
                      <button
                        className="icon-button"
                        title="Descargar"
                        aria-label={`Descargar ${slot.label}`}
                        onClick={() => download(slot.file!.id)}
                      >
                        <Icon name="download" size={16} />
                      </button>
                    )}
                  </div>
                </div>
              ))}
          </div>
        ))}
    </div>
  );
}

const PAGE_SIZE = 20;

export function LiveHistoryTab({
  saleId,
  tasks,
}: {
  saleId: string;
  tasks: ApiTask[];
}) {
  const [page, setPage] = useState(1);
  const history = useHistory(saleId, page, PAGE_SIZE);
  const slots = useDocuments(saleId);

  if (history.isPending)
    return <div className="live-loading">Cargando historial…</div>;
  if (history.isError)
    return (
      <div className="list-error" role="alert">
        <h3>No se pudo cargar el historial</h3>
        <p>{errorText(history.error)}</p>
      </div>
    );

  const taskByInstance = new Map<string, ApiTask>(
    tasks.map((task): [string, ApiTask] => [task.id, task]),
  );
  const slotLabels = new Map<string, string>(
    (slots.data ?? []).map((slot): [string, string] => [
      slot.slotCode,
      slot.label,
    ]),
  );
  const { items, total } = history.data;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="history-tab">
      <div className="subsection-heading">
        <div>
          <h3>Historial de actividad</h3>
          <p>
            Todas las acciones registradas en la operación, de la más reciente a
            la más antigua.
          </p>
        </div>
        <button
          className="button button-secondary button-sm"
          onClick={() => history.refetch()}
        >
          <Icon name="refresh" size={15} /> Actualizar
        </button>
      </div>
      <div className="timeline">
        {items.map((entry) => {
          const task =
            entry.entityType === "task_instance"
              ? taskByInstance.get(entry.entityId)
              : undefined;
          const rawSlot = entry.changes?.slot;
          const slotCode = typeof rawSlot === "string" ? rawSlot : undefined;
          const meta = describeEvent(
            entry,
            task ? `Tarea ${task.number} · ${task.name}` : undefined,
            slotCode ? slotLabels.get(slotCode) : undefined,
          );
          const lines = describeChanges(entry.changes);
          return (
            <TimelineItem
              key={entry.id}
              icon={meta.icon}
              title={meta.title}
              text={
                [entry.reason ? `Motivo: ${entry.reason}` : "", ...lines]
                  .filter(Boolean)
                  .join(" · ") || "—"
              }
              date={formatDateTime(entry.createdAt)}
              actor={
                entry.actorName ??
                (entry.actorKind === "system" ? "Sistema" : "—")
              }
              tone={meta.tone}
            />
          );
        })}
      </div>
      {pages > 1 && (
        <div className="live-pager">
          <button
            className="button button-secondary button-sm"
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
          >
            Anterior
          </button>
          <span>
            Página {page} de {pages}
          </span>
          <button
            className="button button-secondary button-sm"
            disabled={page >= pages}
            onClick={() => setPage(page + 1)}
          >
            Siguiente
          </button>
        </div>
      )}
    </div>
  );
}
