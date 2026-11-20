import { useState } from "react";
import { formatDateTime, Field, Icon, type Toast } from "@endosoflow/ui";
import { documentsApi, salesApi, type ApiSale } from "./api";
import { describeChanges, describeEvent, errorText } from "./labels";
import { useDocuments, useHistory, useSaleMutation } from "./queries";

/** Every file of the sale, by task, with the same download as the task screen. */
export function DocumentsTab({
  sale,
  onOpenTask,
  toast,
}: {
  sale: ApiSale;
  onOpenTask: (number: number) => void;
  toast: (toast: Toast) => void;
}) {
  const slots = useDocuments(sale.id);
  if (slots.isPending)
    return (
      <div className="skeleton-rows">
        <span className="skeleton" />
        <span className="skeleton" />
      </div>
    );
  if (slots.isError)
    return (
      <div className="empty" role="alert">
        <Icon name="alert" size={22} />
        <strong>No se pudieron cargar los documentos</strong>
        <p>{errorText(slots.error)}</p>
      </div>
    );
  const items = slots.data;
  const uploaded = items.filter((slot) => slot.file).length;
  const missing = items.filter((slot) => slot.required && !slot.file).length;

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
    <div className="stack-tight">
      <p className="tab-summary">
        <strong>{uploaded}</strong> cargados · <strong>{missing}</strong>{" "}
        obligatorios pendientes
      </p>
      {sale.tasks
        .filter((task) => items.some((slot) => slot.taskNumber === task.number))
        .map((task) => (
          <section className="doc-group" key={task.number}>
            <header>
              <h3>
                {task.number}. {task.name}
              </h3>
              <button
                className="text-button"
                onClick={() => onOpenTask(task.number)}
              >
                Abrir tarea
              </button>
            </header>
            {items
              .filter((slot) => slot.taskNumber === task.number)
              .map((slot) => (
                <div className="doc-row" key={slot.slotCode}>
                  <span
                    className={`doc-icon ${slot.file ? "is-uploaded" : ""}`}
                  >
                    <Icon
                      name={slot.file ? "file-check" : "file-text"}
                      size={17}
                    />
                  </span>
                  <div className="doc-copy">
                    <strong>
                      {slot.label}
                      {slot.required && <em> · obligatorio</em>}
                    </strong>
                    <span>
                      {slot.file
                        ? `${slot.file.fileName} · ${slot.file.uploadedBy} · ${formatDateTime(slot.file.uploadedAt)}`
                        : "Sin archivo"}
                    </span>
                  </div>
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
              ))}
          </section>
        ))}
    </div>
  );
}

/** The sale's own notes, saved with the sale's version so two people never overwrite each other unseen. */
export function NotesTab({
  sale,
  toast,
}: {
  sale: ApiSale;
  toast: (toast: Toast) => void;
}) {
  const [value, setValue] = useState(sale.notes ?? "");
  const save = useSaleMutation((text: string) =>
    salesApi.updateNotes(sale.id, sale.version, text.trim() || null),
  );
  const editable = sale.permissions.canEditNotes;
  const changed = value.trim() !== (sale.notes ?? "").trim();
  return (
    <div className="stack-tight">
      <Field
        label="Notas de la venta"
        hint="Visibles solo para el equipo interno."
      >
        <textarea
          rows={7}
          value={value}
          disabled={!editable}
          maxLength={2000}
          onChange={(event) => setValue(event.target.value)}
        />
      </Field>
      {editable ? (
        <div>
          <button
            className="button button-primary"
            disabled={!changed || save.isPending}
            onClick={async () => {
              try {
                await save.mutateAsync(value);
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
            {save.isPending ? "Guardando…" : "Guardar notas"}
          </button>
        </div>
      ) : (
        <p className="muted">Esta venta está en modo lectura.</p>
      )}
    </div>
  );
}

const PAGE_SIZE = 20;

export function HistoryTab({ sale }: { sale: ApiSale }) {
  const [page, setPage] = useState(1);
  const history = useHistory(sale.id, page, PAGE_SIZE);
  const slots = useDocuments(sale.id);

  if (history.isPending)
    return (
      <div className="skeleton-rows">
        <span className="skeleton" />
        <span className="skeleton" />
        <span className="skeleton" />
      </div>
    );
  if (history.isError)
    return (
      <div className="empty" role="alert">
        <Icon name="alert" size={22} />
        <strong>No se pudo cargar el historial</strong>
        <p>{errorText(history.error)}</p>
      </div>
    );

  const taskNames = new Map(sale.tasks.map((task) => [task.number, task.name]));
  const slotLabels = new Map(
    (slots.data ?? []).map((slot) => [slot.slotCode, slot.label]),
  );
  const { items, total } = history.data;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="stack-tight">
      <ol className="timeline">
        {items.map((entry) => {
          const taskLabel = entry.taskNumber
            ? `Tarea ${entry.taskNumber} · ${taskNames.get(entry.taskNumber) ?? ""}`.trim()
            : undefined;
          const rawSlot = entry.changes?.slot;
          const meta = describeEvent(
            entry,
            taskLabel,
            typeof rawSlot === "string" ? slotLabels.get(rawSlot) : undefined,
          );
          const lines = [
            entry.reason ? `Motivo: ${entry.reason}` : "",
            ...describeChanges(entry.changes),
          ].filter(Boolean);
          return (
            <li key={entry.id} className={`timeline-item tone-${meta.tone}`}>
              <span className="timeline-dot" />
              <div>
                <strong>{meta.title}</strong>
                {lines.length > 0 && <p>{lines.join(" · ")}</p>}
                <small>
                  {entry.isSystem ? "Sistema" : entry.actorName} ·{" "}
                  {formatDateTime(entry.at)}
                </small>
              </div>
            </li>
          );
        })}
      </ol>
      {pages > 1 && (
        <footer className="pager">
          <span>
            Página {page} de {pages}
          </span>
          <span className="pager-buttons">
            <button
              className="button button-secondary button-sm"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              &lt;
            </button>
            <button
              className="button button-secondary button-sm"
              disabled={page >= pages}
              onClick={() => setPage(page + 1)}
            >
              &gt;
            </button>
          </span>
        </footer>
      )}
    </div>
  );
}
