import { useState } from "react";
import { formatDateTime } from "../../../shared/lib/format";
import { Icon } from "../../../shared/ui/Icon";
import type { Toast } from "../../../shared/ui/toast";
import { documentsApi, type ApiSlot } from "./api";
import { ConfirmDialog, ReasonDialog } from "./dialogs";
import { errorText } from "./labels";
import { useSaleMutation } from "./queries";

const MAX_BYTES = 10 * 1024 * 1024;

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** A quick check in the browser; the server checks the real content again. */
function checkFile(
  file: File,
  format: ApiSlot["allowedFormat"],
): string | null {
  const name = file.name.toLowerCase();
  if (file.size === 0) return "El archivo está vacío.";
  if (file.size > MAX_BYTES) return "El archivo supera el máximo de 10 MB.";
  if (format === "pdf" && !name.endsWith(".pdf"))
    return "Este documento debe ser un PDF.";
  if (format === "excel" && !(name.endsWith(".xlsx") || name.endsWith(".xls")))
    return "Este documento debe ser una hoja de Excel (.xlsx o .xls).";
  return null;
}

const formatLabel = (slot: ApiSlot) =>
  slot.allowedFormat === "pdf" ? "PDF" : "Excel (.xlsx, .xls)";

/**
 * The files a task needs. While the task is in progress they can be uploaded, replaced and removed; on a completed
 * task a Manager or Admin can only replace one, with a written reason.
 */
export function TaskDocuments({
  saleId,
  number,
  status,
  slots,
  canEdit,
  canReplaceCompleted,
  errors,
  toast,
}: {
  saleId: string;
  number: number;
  status: "pending" | "in_progress" | "completed";
  slots: ApiSlot[];
  canEdit: boolean;
  canReplaceCompleted: boolean;
  errors: Record<string, string>;
  toast: (toast: Toast) => void;
}) {
  const [pendingReplace, setPendingReplace] = useState<{
    slot: ApiSlot;
    file: File;
  } | null>(null);
  const [removing, setRemoving] = useState<ApiSlot | null>(null);
  const [busySlot, setBusySlot] = useState<string | null>(null);

  const upload = useSaleMutation(
    (input: { slot: string; file: File; reason?: string }) =>
      documentsApi.upload(saleId, number, input.slot, input.file, input.reason),
  );
  const remove = useSaleMutation((slot: string) =>
    documentsApi.remove(saleId, number, slot),
  );

  const mayUpload =
    (status === "in_progress" && canEdit) ||
    (status === "completed" && canReplaceCompleted);
  const mayRemove = status === "in_progress" && canEdit;

  async function choose(slot: ApiSlot, file: File | undefined) {
    if (!file) return;
    const problem = checkFile(file, slot.allowedFormat);
    if (problem) {
      toast({
        title: "No se puede adjuntar este archivo",
        message: problem,
        tone: "error",
      });
      return;
    }
    if (status === "completed") {
      setPendingReplace({ slot, file });
      return;
    }
    setBusySlot(slot.slotCode);
    try {
      await upload.mutateAsync({ slot: slot.slotCode, file });
      toast({
        title: slot.file ? "Documento reemplazado" : "Documento cargado",
        message: slot.label,
        tone: "success",
      });
    } catch (error) {
      toast({
        title: "No se pudo cargar el documento",
        message: errorText(error),
        tone: "error",
      });
    } finally {
      setBusySlot(null);
    }
  }

  async function download(slot: ApiSlot) {
    if (!slot.file) return;
    try {
      const result = await documentsApi.downloadUrl(slot.file.id);
      window.open(result.url, "_blank", "noopener,noreferrer");
    } catch (error) {
      toast({
        title: "No se pudo abrir el documento",
        message: errorText(error),
        tone: "error",
      });
    }
  }

  if (slots.length === 0) return null;
  return (
    <div className="live-docs">
      <div className="live-section-title">
        <h4>Documentos de la tarea</h4>
        <span>PDF o Excel · máximo 10 MB</span>
      </div>
      {slots.map((slot) => {
        const problem = errors[`documents.${slot.slotCode}`];
        return (
          <div
            className={`live-doc-row ${problem ? "has-error" : ""}`}
            key={slot.slotCode}
          >
            <span className={`document-icon ${slot.file ? "uploaded" : ""}`}>
              <Icon name={slot.file ? "file-check" : "file-text"} size={17} />
            </span>
            <div className="live-doc-copy">
              <strong>
                {slot.label}
                {slot.required && (
                  <em className="live-required"> · obligatorio</em>
                )}
              </strong>
              {slot.file ? (
                <span>
                  {slot.file.fileName} · {formatSize(slot.file.sizeBytes)} ·{" "}
                  {slot.file.uploadedBy} ·{" "}
                  {formatDateTime(slot.file.uploadedAt)}
                </span>
              ) : (
                <span>
                  {formatLabel(slot)} ·{" "}
                  {slot.customerVisible
                    ? "visible para el cliente"
                    : "uso interno"}
                </span>
              )}
              {problem && <small className="field-error">{problem}</small>}
            </div>
            <div className="live-doc-actions">
              {slot.file && (
                <button
                  className="icon-button"
                  title="Descargar"
                  aria-label={`Descargar ${slot.label}`}
                  onClick={() => download(slot)}
                >
                  <Icon name="download" size={16} />
                </button>
              )}
              {mayUpload && (
                <label
                  className="icon-button"
                  title={slot.file ? "Reemplazar archivo" : "Adjuntar archivo"}
                  aria-label={`${slot.file ? "Reemplazar" : "Adjuntar"} ${slot.label}`}
                >
                  <Icon
                    name={busySlot === slot.slotCode ? "clock" : "upload"}
                    size={16}
                  />
                  <input
                    type="file"
                    hidden
                    accept={
                      slot.allowedFormat === "pdf"
                        ? ".pdf,application/pdf"
                        : ".xlsx,.xls"
                    }
                    disabled={busySlot !== null}
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      event.target.value = "";
                      void choose(slot, file);
                    }}
                  />
                </label>
              )}
              {mayRemove && slot.file && (
                <button
                  className="icon-button"
                  title="Quitar archivo"
                  aria-label={`Quitar ${slot.label}`}
                  onClick={() => setRemoving(slot)}
                >
                  <Icon name="x" size={16} />
                </button>
              )}
            </div>
          </div>
        );
      })}
      {status === "completed" && !canReplaceCompleted && (
        <p className="live-muted">
          Solo Gerencia puede reemplazar un documento de una tarea completada.
        </p>
      )}
      {pendingReplace && (
        <ReasonDialog
          title="Reemplazar documento"
          description={`${pendingReplace.slot.label}: ${pendingReplace.file.name}`}
          label="Motivo del reemplazo"
          confirmLabel="Reemplazar documento"
          onClose={() => setPendingReplace(null)}
          onConfirm={async (reason) => {
            await upload.mutateAsync({
              slot: pendingReplace.slot.slotCode,
              file: pendingReplace.file,
              reason,
            });
            toast({
              title: "Documento reemplazado",
              message: pendingReplace.slot.label,
              tone: "success",
            });
          }}
        />
      )}
      {removing && (
        <ConfirmDialog
          title="Quitar documento"
          message={`Se quitará «${removing.label}» de la tarea. Podrás cargarlo de nuevo mientras la tarea siga en curso.`}
          confirmLabel="Quitar documento"
          danger
          onClose={() => setRemoving(null)}
          onConfirm={async () => {
            await remove.mutateAsync(removing.slotCode);
            toast({
              title: "Documento quitado",
              message: removing.label,
              tone: "success",
            });
          }}
        />
      )}
    </div>
  );
}
