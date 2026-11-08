import { useEffect, useMemo, useState } from "react";
import { formatDateTime, formatMoneyText } from "../../../shared/lib/format";
import type { Role } from "../../../shared/types";
import { Field } from "../../../shared/ui/Field";
import { Icon } from "../../../shared/ui/Icon";
import type { Toast } from "../../../shared/ui/toast";
import { TaskPill } from "../components/pills";
import {
  tasksApi,
  type ApiSaleDetail,
  type ApiTask,
  type ApiTaskDetail,
} from "./api";
import { ReasonDialog } from "./dialogs";
import { errorText, fieldErrors, phaseLabels } from "./labels";
import { canWork, isManagement } from "./permissions";
import { useDocuments, useSaleMutation, useTask } from "./queries";
import { TaskDocuments } from "./TaskDocuments";
import {
  amountsDiffer,
  isDirty,
  lockedKeys,
  taskForms,
  toDraft,
  toPayload,
  type Draft,
  type FieldDef,
} from "./taskForms";

/** One task of the open sale: its data form, its documents and the actions its state allows. */
export function TaskWorkspace({
  sale,
  number,
  tasks,
  role,
  userId,
  toast,
}: {
  sale: ApiSaleDetail;
  number: number;
  tasks: ApiTask[];
  role: Role;
  userId: string;
  toast: (toast: Toast) => void;
}) {
  const query = useTask(sale.id, number);
  if (query.isPending)
    return (
      <div className="task-detail-panel">
        <div className="live-loading">Cargando la tarea…</div>
      </div>
    );
  if (query.isError)
    return (
      <div className="task-detail-panel">
        <div className="list-error" role="alert">
          <h3>No se pudo cargar la tarea</h3>
          <p>{errorText(query.error)}</p>
          <button
            className="button button-secondary"
            onClick={() => query.refetch()}
          >
            <Icon name="refresh" size={15} /> Reintentar
          </button>
        </div>
      </div>
    );
  return (
    <TaskBody
      key={query.data.id}
      sale={sale}
      task={query.data}
      tasks={tasks}
      role={role}
      userId={userId}
      toast={toast}
    />
  );
}

type Dialog = "restart" | "reopen" | "correct" | null;

function TaskBody({
  sale,
  task,
  tasks,
  role,
  userId,
  toast,
}: {
  sale: ApiSaleDetail;
  task: ApiTaskDetail;
  tasks: ApiTask[];
  role: Role;
  userId: string;
  toast: (toast: Toast) => void;
}) {
  const number = task.number;
  const slots = useDocuments(sale.id);
  const data = task.data as Record<string, unknown>;
  const saved = useMemo<Draft>(() => toDraft(number, data), [number, data]);
  const [draft, setDraft] = useState<Draft>(saved);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [banner, setBanner] = useState<string | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [editing, setEditing] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);

  // A new version or status from the server replaces whatever was typed: it is now what the server holds.
  useEffect(() => {
    setDraft(saved);
    setEditing(false);
  }, [task.version, task.status]);

  const start = useSaleMutation(() => tasksApi.start(sale.id, number));
  const complete = useSaleMutation(() => tasksApi.complete(sale.id, number));
  const save = useSaleMutation(
    (input: { data: Record<string, unknown>; reason?: string }) =>
      tasksApi.saveData(sale.id, number, {
        version: task.version,
        data: input.data,
        reason: input.reason,
      }),
  );
  const restart = useSaleMutation((reason: string) =>
    tasksApi.restart(sale.id, number, reason),
  );
  const reopen = useSaleMutation((reason: string) =>
    tasksApi.reopen(sale.id, number, reason),
  );

  const working = canWork(role, userId, sale);
  const management = isManagement(role);
  const highestCompleted = tasks.reduce(
    (max, t) => (t.status === "completed" ? Math.max(max, t.number) : max),
    0,
  );
  const locked =
    task.status === "completed" ? lockedKeys(number, highestCompleted) : [];
  const editable =
    working &&
    (task.status === "in_progress" ||
      (task.status === "completed" && editing && management));
  const dirty = isDirty(number, saved, draft);
  const busy =
    start.isPending ||
    complete.isPending ||
    save.isPending ||
    restart.isPending ||
    reopen.isPending;
  const taskSlots = (slots.data ?? []).filter(
    (slot) => slot.taskNumber === number,
  );
  const displayStatus =
    task.status === "pending" && task.eligible ? "eligible" : task.status;
  const groups = taskForms[number] ?? [];

  function failure(error: unknown) {
    setErrors(fieldErrors(error));
    setBanner(errorText(error));
  }

  /** Sends what changed. Throws when the server refuses, so callers decide how to show it. */
  async function submitSave(reason?: string) {
    const payload = toPayload(number, saved, draft);
    if (Object.keys(payload).length === 0) return;
    const result = await save.mutateAsync({ data: payload, reason });
    setErrors({});
    setBanner(null);
    setWarnings(result.warnings.map((w) => w.message));
  }

  async function handleSave() {
    if (task.status === "completed") {
      setDialog("correct");
      return;
    }
    try {
      await submitSave();
      toast({ title: "Datos guardados", message: task.name, tone: "success" });
    } catch (error) {
      failure(error);
    }
  }

  async function handleStart() {
    try {
      await start.mutateAsync(undefined);
      setBanner(null);
      toast({ title: "Tarea iniciada", message: task.name, tone: "success" });
    } catch (error) {
      failure(error);
    }
  }

  async function handleComplete() {
    setBanner(null);
    setErrors({});
    try {
      if (dirty) await submitSave();
      await complete.mutateAsync(undefined);
      toast({ title: "Tarea completada", message: task.name, tone: "success" });
    } catch (error) {
      failure(error);
    }
  }

  function renderField(def: FieldDef, key: string) {
    const value = draft[key] ?? "";
    const disabled = !editable || locked.includes(def.key);
    let hint = def.hint;
    if (locked.includes(def.key))
      hint =
        "Bloqueado: hay tareas posteriores completadas. Reabre las tareas en orden inverso para corregirlo.";
    if (def.key === "mismatchReason") {
      const amountKey = key.replace(
        "mismatchReason",
        number === 4 || number === 5 ? "depositedAmount" : "amount",
      );
      const expectedKey = key.replace("mismatchReason", "expectedAmount");
      if (
        amountsDiffer(draft[amountKey], saved[expectedKey]) &&
        value.trim() === ""
      )
        hint =
          "El monto no coincide con el esperado: escribe el motivo antes de guardar.";
    }
    const className = def.wide ? "span-2" : "";
    if (def.readOnly)
      return (
        <Field
          key={key}
          label={def.label}
          hint={def.hint}
          className={className}
        >
          <div className="readonly-money">
            {def.kind === "money"
              ? formatMoneyText(saved[key])
              : saved[key] || "—"}
          </div>
        </Field>
      );
    const set = (next: string) => setDraft((old) => ({ ...old, [key]: next }));
    return (
      <Field
        key={key}
        label={def.label}
        hint={hint}
        error={errors[key]}
        className={className}
      >
        {def.kind === "select" ? (
          <select
            value={value}
            disabled={disabled}
            onChange={(event) => set(event.target.value)}
          >
            <option value="">Selecciona…</option>
            {def.options?.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        ) : (
          <input
            type={def.kind === "date" ? "date" : "text"}
            inputMode={def.kind === "money" ? "decimal" : undefined}
            value={value}
            disabled={disabled}
            placeholder={def.placeholder}
            onChange={(event) => set(event.target.value)}
          />
        )}
      </Field>
    );
  }

  const messages = Array.from(new Set(Object.values(errors)));
  const documentErrors = Object.keys(errors).some((key) =>
    key.startsWith("documents."),
  );

  return (
    <div className="task-detail-panel">
      <div className="task-detail-heading">
        <div className="task-detail-title">
          <span className="task-number large">
            {String(number).padStart(2, "0")}
          </span>
          <div>
            <span className="section-kicker">TAREA {number} DE 13</span>
            <h3>{task.name}</h3>
            <p>
              {task.status === "completed"
                ? `Completada el ${task.completedAt ? formatDateTime(task.completedAt) : "—"}${
                    task.completionPhase
                      ? ` · ${phaseLabels[task.completionPhase]}`
                      : ""
                  }.`
                : task.status === "in_progress"
                  ? `Completa los datos y los documentos para avanzar.${
                      task.dueDate
                        ? ` Plazo: ${formatDateTime(task.dueDate)}.`
                        : ""
                    }`
                  : task.eligible
                    ? "Esta tarea está habilitada y lista para comenzar."
                    : `Primero debe completarse la tarea ${Math.max(1, number - 1)}.`}
            </p>
          </div>
        </div>
        <TaskPill status={displayStatus} />
      </div>

      {sale.state !== "active" && (
        <div className="task-locked-message">
          <Icon name="lock" size={17} />
          <span>
            {sale.state === "cancelled"
              ? "La venta está cancelada: las tareas están en modo lectura."
              : "La venta está completada: las tareas están en modo lectura."}
          </span>
        </div>
      )}

      {task.status === "pending" &&
        sale.state === "active" &&
        !task.eligible && (
          <div className="task-locked-message">
            <Icon name="lock" size={17} />
            <span>
              Esta tarea se habilitará cuando finalice la tarea anterior.
            </span>
          </div>
        )}

      {task.status === "pending" && task.eligible && working && (
        <div className="task-ready-panel">
          <span className="ready-icon">
            <Icon name="sparkles" size={18} />
          </span>
          <div>
            <strong>Todo listo para empezar</strong>
            <p>
              Al iniciar podrás editar los campos y adjuntar los documentos de
              esta tarea.
            </p>
          </div>
          <button
            className="button button-primary button-sm"
            onClick={handleStart}
            disabled={busy}
          >
            Iniciar tarea <Icon name="arrow-right" size={15} />
          </button>
        </div>
      )}

      {task.status !== "pending" && (
        <>
          {(banner || messages.length > 0) && (
            <div className="form-alert live-form-alert" role="alert">
              <Icon name="alert" size={16} />
              <div>
                <strong>{banner ?? "Revisa los datos ingresados."}</strong>
                {messages.length > 1 && (
                  <ul>
                    {messages.slice(0, 8).map((message) => (
                      <li key={message}>{message}</li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          )}
          {warnings.length > 0 && (
            <div className="live-warning" role="status">
              <Icon name="info" size={16} />
              <ul>
                {warnings.map((message) => (
                  <li key={message}>{message}</li>
                ))}
              </ul>
            </div>
          )}
          {groups.map((group, index) => (
            <div className="live-group" key={group.prefix ?? index}>
              {group.title && (
                <h4 className="live-group-title">{group.title}</h4>
              )}
              <div className="task-form-grid">
                {group.fields.map((def) =>
                  renderField(
                    def,
                    group.prefix ? `${group.prefix}.${def.key}` : def.key,
                  ),
                )}
              </div>
            </div>
          ))}
          {groups.length === 0 && (
            <p className="live-muted live-pad">
              Esta tarea solo requiere sus documentos.
            </p>
          )}
          <TaskDocuments
            saleId={sale.id}
            number={number}
            status={task.status}
            slots={taskSlots}
            canEdit={working}
            canReplaceCompleted={working && management}
            errors={documentErrors ? errors : {}}
            toast={toast}
          />
          <TaskNotes
            key={task.notes ?? ""}
            saleId={sale.id}
            number={number}
            notes={task.notes ?? ""}
            editable={working && (management || task.status === "in_progress")}
            toast={toast}
          />
        </>
      )}

      {task.status === "in_progress" && working && (
        <div className="task-panel-footer">
          <span>
            <Icon name="shield-check" size={15} /> Los datos se validan en el
            servidor al guardar y al completar.
          </span>
          <div>
            {management && (
              <button
                className="button button-secondary button-sm"
                onClick={() => setDialog("restart")}
                disabled={busy}
              >
                Reiniciar tarea
              </button>
            )}
            <button
              className="button button-secondary button-sm"
              onClick={handleSave}
              disabled={busy || !dirty}
            >
              {save.isPending ? "Guardando…" : "Guardar datos"}
            </button>
            <button
              className="button button-primary button-sm"
              onClick={handleComplete}
              disabled={busy}
            >
              {complete.isPending ? "Completando…" : "Completar tarea"}{" "}
              <Icon name="arrow-right" size={15} />
            </button>
          </div>
        </div>
      )}

      {task.status === "completed" && working && management && (
        <div className="task-panel-footer">
          <span>
            <Icon name="shield-check" size={15} /> Corregir o reabrir una tarea
            completada queda registrado con su motivo.
          </span>
          <div>
            {number === highestCompleted && (
              <button
                className="button button-secondary button-sm"
                onClick={() => setDialog("reopen")}
                disabled={busy}
              >
                Reabrir tarea
              </button>
            )}
            {groups.length > 0 && !editing && (
              <button
                className="button button-secondary button-sm"
                onClick={() => setEditing(true)}
                disabled={busy}
              >
                <Icon name="edit" size={14} /> Corregir datos
              </button>
            )}
            {editing && (
              <>
                <button
                  className="button button-secondary button-sm"
                  onClick={() => {
                    setDraft(saved);
                    setEditing(false);
                    setErrors({});
                    setBanner(null);
                  }}
                  disabled={busy}
                >
                  Descartar
                </button>
                <button
                  className="button button-primary button-sm"
                  onClick={handleSave}
                  disabled={busy || !dirty}
                >
                  Guardar corrección
                </button>
              </>
            )}
          </div>
        </div>
      )}

      {dialog === "restart" && (
        <ReasonDialog
          title="Reiniciar tarea"
          description="La tarea vuelve a pendiente. Sus datos se conservan en modo lectura hasta que se inicie de nuevo."
          label="Motivo del reinicio"
          confirmLabel="Reiniciar tarea"
          danger
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            await restart.mutateAsync(reason);
            toast({
              title: "Tarea reiniciada",
              message: task.name,
              tone: "success",
            });
          }}
        />
      )}
      {dialog === "reopen" && (
        <ReasonDialog
          title="Reabrir tarea"
          description="La tarea vuelve a estar en curso con su plazo original. Si es la Tarea 13, la venta vuelve a estar activa."
          label="Motivo de la reapertura"
          confirmLabel="Reabrir tarea"
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            await reopen.mutateAsync(reason);
            toast({
              title: "Tarea reabierta",
              message: task.name,
              tone: "success",
            });
          }}
        />
      )}
      {dialog === "correct" && (
        <ReasonDialog
          title="Registrar corrección"
          description="Se guardarán los datos corregidos de una tarea ya completada."
          label="Motivo de la corrección"
          confirmLabel="Guardar corrección"
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            try {
              await submitSave(reason);
            } catch (error) {
              setErrors(fieldErrors(error));
              throw error;
            }
            setEditing(false);
            toast({
              title: "Corrección registrada",
              message: task.name,
              tone: "success",
            });
          }}
        />
      )}
    </div>
  );
}

function TaskNotes({
  saleId,
  number,
  notes,
  editable,
  toast,
}: {
  saleId: string;
  number: number;
  notes: string;
  editable: boolean;
  toast: (toast: Toast) => void;
}) {
  const [value, setValue] = useState(notes);
  const mutation = useSaleMutation((text: string) =>
    tasksApi.saveNotes(saleId, number, text),
  );
  if (!editable && notes === "") return null;
  return (
    <div className="live-notes">
      <Field label="Notas de la tarea" hint="Solo para el equipo interno.">
        <textarea
          rows={2}
          value={value}
          disabled={!editable}
          maxLength={2000}
          onChange={(event) => setValue(event.target.value)}
        />
      </Field>
      {editable && (
        <button
          className="button button-secondary button-sm"
          disabled={mutation.isPending || value.trim() === notes.trim()}
          onClick={async () => {
            try {
              await mutation.mutateAsync(value);
              toast({ title: "Notas guardadas", tone: "success" });
            } catch (error) {
              toast({
                title: "No se pudieron guardar las notas",
                message: errorText(error),
                tone: "error",
              });
            }
          }}
        >
          Guardar notas
        </button>
      )}
    </div>
  );
}
