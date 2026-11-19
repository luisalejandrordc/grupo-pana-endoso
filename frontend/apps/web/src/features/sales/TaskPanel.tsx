import { useEffect, useMemo, useState } from "react";
import { formatDateTime, formatMoneyText } from "../../shared/lib/format";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import type { Toast } from "../../shared/ui/toast";
import { tasksApi, type ApiSale, type ApiTaskDetail } from "./api";
import { ReasonDialog } from "./dialogs";
import { errorText, fieldErrors, phaseLabels } from "./labels";
import { TaskBadge } from "./pills";
import { useSaleMutation, useTask } from "./queries";
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

/** One task of the open sale: its data form, its documents and the actions the server says the person may take. */
export function TaskPanel({
  sale,
  number,
  toast,
  onDirtyChange,
}: {
  sale: ApiSale;
  number: number;
  toast: (toast: Toast) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const query = useTask(sale.id, number);
  if (query.isPending)
    return (
      <section className="task-panel" aria-busy="true">
        <span className="skeleton skeleton-title" />
        <span className="skeleton" />
        <span className="skeleton" />
      </section>
    );
  if (query.isError)
    return (
      <section className="task-panel">
        <div className="empty" role="alert">
          <Icon name="alert" size={22} />
          <strong>No se pudo cargar la tarea</strong>
          <p>{errorText(query.error)}</p>
          <button
            className="button button-secondary"
            onClick={() => query.refetch()}
          >
            <Icon name="refresh" size={15} /> Reintentar
          </button>
        </div>
      </section>
    );
  return (
    <TaskBody
      key={`${sale.id}-${number}`}
      sale={sale}
      task={query.data}
      toast={toast}
      onDirtyChange={onDirtyChange}
    />
  );
}

type Dialog = "restart" | "reopen" | "correct" | null;

function TaskBody({
  sale,
  task,
  toast,
  onDirtyChange,
}: {
  sale: ApiSale;
  task: ApiTaskDetail;
  toast: (toast: Toast) => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const number = task.number;
  const can = task.permissions;
  const data = task.data as Record<string, unknown>;
  const saved = useMemo<Draft>(
    () => toDraft(number, data, task.calculated),
    [number, data, task.calculated],
  );
  const [draft, setDraft] = useState<Draft>(saved);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [banner, setBanner] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const warnings = task.warnings.map((warning) => warning.message);

  // A new version or status from the server replaces whatever was typed: it is now what the server holds.
  // (State adjusted while rendering, which React prefers to an effect that would render twice.)
  const [seen, setSeen] = useState({
    version: task.version,
    status: task.status,
  });
  if (seen.version !== task.version || seen.status !== task.status) {
    setSeen({ version: task.version, status: task.status });
    setDraft(saved);
    setEditing(false);
  }

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

  const highestCompleted = sale.tasks.reduce(
    (max, t) => (t.status === "completed" ? Math.max(max, t.number) : max),
    0,
  );
  const locked =
    task.status === "completed" ? lockedKeys(number, highestCompleted) : [];
  const editable =
    can.canEditData && (task.status === "in_progress" || editing);
  const dirty = isDirty(number, saved, draft);
  const busy =
    start.isPending ||
    complete.isPending ||
    save.isPending ||
    restart.isPending ||
    reopen.isPending;
  const groups = taskForms[number] ?? [];

  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);

  function failure(error: unknown) {
    setErrors(fieldErrors(error));
    setBanner(errorText(error));
  }

  /** Sends what changed. Throws when the server refuses, so callers decide how to show it. */
  async function submitSave(reason?: string) {
    const payload = toPayload(number, saved, draft);
    if (Object.keys(payload).length === 0) return;
    await save.mutateAsync({ data: payload, reason });
    setErrors({});
    setBanner(null);
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
          <div className="readonly-value">
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
  const subtitle =
    task.status === "completed"
      ? `Completada el ${task.completedAt ? formatDateTime(task.completedAt) : "—"}${
          task.completionPhase ? ` · ${phaseLabels[task.completionPhase]}` : ""
        }`
      : task.status === "in_progress"
        ? task.dueDate
          ? `En progreso · plazo ${formatDateTime(task.dueDate)}`
          : "En progreso"
        : task.isEligible
          ? "Lista para comenzar"
          : `Se habilita al completar la tarea ${Math.max(1, number - 1)}`;
  const overdue =
    task.status === "in_progress" &&
    sale.currentTask?.number === number &&
    sale.currentTask.isOverdue;

  return (
    <section
      className="task-panel"
      aria-label={`Tarea ${number}: ${task.name}`}
    >
      <header className="task-panel-head">
        <div>
          <span className="task-panel-step">Tarea {number} de 13</span>
          <h2>{task.name}</h2>
          <p>{subtitle}</p>
        </div>
        <TaskBadge status={task.status} overdue={overdue} />
      </header>

      {sale.state !== "active" && (
        <div className="callout callout-quiet">
          <Icon name="lock" size={17} />
          <span>
            {sale.state === "cancelled"
              ? "La venta está cancelada: las tareas están en modo lectura."
              : "La venta está completada: las tareas solo pueden corregirse con un motivo."}
          </span>
        </div>
      )}

      {task.status === "pending" &&
        sale.state === "active" &&
        !task.isEligible && (
          <div className="callout callout-quiet">
            <Icon name="lock" size={17} />
            <span>
              Esta tarea se habilitará cuando se complete la anterior.
            </span>
          </div>
        )}

      {task.status === "pending" && can.canStart && (
        <div className="callout callout-action">
          <div>
            <strong>Todo listo para empezar</strong>
            <p>
              Al iniciar podrás completar los campos y adjuntar los documentos
              de esta tarea.
            </p>
          </div>
          <button
            className="button button-primary"
            onClick={handleStart}
            disabled={busy}
          >
            Iniciar tarea
          </button>
        </div>
      )}

      {task.status !== "pending" && (
        <>
          {(banner || messages.length > 0) && (
            <div className="callout callout-error" role="alert">
              <Icon name="alert" size={17} />
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
            <div className="callout callout-warn" role="status">
              <Icon name="info" size={17} />
              <ul>
                {warnings.map((message) => (
                  <li key={message}>{message}</li>
                ))}
              </ul>
            </div>
          )}
          {groups.map((group, index) => (
            <fieldset className="form-group" key={group.prefix ?? index}>
              {group.title && <legend>{group.title}</legend>}
              <div className="form-grid">
                {group.fields.map((def) =>
                  renderField(
                    def,
                    group.prefix ? `${group.prefix}.${def.key}` : def.key,
                  ),
                )}
              </div>
            </fieldset>
          ))}
          {groups.length === 0 && (
            <p className="muted">Esta tarea solo requiere sus documentos.</p>
          )}
          <TaskDocuments
            saleId={sale.id}
            number={number}
            status={task.status}
            slots={task.documents}
            canUpload={can.canUploadDocuments}
            canRemove={can.canRemoveDocuments}
            errors={documentErrors ? errors : {}}
            toast={toast}
          />
          <TaskNotes
            key={task.notes ?? ""}
            saleId={sale.id}
            number={number}
            notes={task.notes ?? ""}
            editable={can.canSaveNotes}
            toast={toast}
          />
        </>
      )}

      {(can.canComplete ||
        can.canRestart ||
        can.canReopen ||
        can.canEditData) &&
        task.status !== "pending" && (
          <footer className="task-panel-foot">
            <span className="task-panel-foot-note">
              {task.status === "in_progress"
                ? dirty
                  ? "Tienes cambios sin guardar."
                  : "Los datos se validan al guardar y al completar."
                : "Corregir o reabrir una tarea queda registrado con su motivo."}
            </span>
            <div className="task-panel-actions">
              {can.canRestart && (
                <button
                  className="button button-ghost"
                  onClick={() => setDialog("restart")}
                  disabled={busy}
                >
                  Reiniciar
                </button>
              )}
              {can.canReopen && (
                <button
                  className="button button-secondary"
                  onClick={() => setDialog("reopen")}
                  disabled={busy}
                >
                  Reabrir tarea
                </button>
              )}
              {task.status === "completed" &&
                can.canEditData &&
                groups.length > 0 &&
                !editing && (
                  <button
                    className="button button-secondary"
                    onClick={() => setEditing(true)}
                    disabled={busy}
                  >
                    <Icon name="edit" size={15} /> Corregir datos
                  </button>
                )}
              {task.status === "completed" && editing && (
                <>
                  <button
                    className="button button-ghost"
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
                    className="button button-primary"
                    onClick={handleSave}
                    disabled={busy || !dirty}
                  >
                    Guardar corrección
                  </button>
                </>
              )}
              {task.status === "in_progress" && can.canEditData && (
                <button
                  className="button button-secondary"
                  onClick={handleSave}
                  disabled={busy || !dirty}
                >
                  {save.isPending ? "Guardando…" : "Guardar"}
                </button>
              )}
              {can.canComplete && (
                <button
                  className="button button-primary"
                  onClick={handleComplete}
                  disabled={busy}
                >
                  {complete.isPending ? "Completando…" : "Completar tarea"}
                </button>
              )}
            </div>
          </footer>
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
          description="La tarea vuelve a estar en curso con su plazo original. Si es la tarea 13, la venta vuelve a estar activa."
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
    </section>
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
    <div className="task-notes">
      <Field
        label="Notas de la tarea"
        hint="Solo para el equipo interno. No cambian la versión de los datos."
      >
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
