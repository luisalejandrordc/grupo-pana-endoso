import { errorText } from "@endosoflow/api";
import { Icon, useToast } from "@endosoflow/ui";
import { useState } from "react";
import { api } from "../../shared/api/client";
import { formatDay, formatMoment, formatSize } from "../../shared/lib/format";
import type {
  PortalDocument,
  PortalField,
  PortalSection,
} from "../../shared/types";
import { formatMoneyText } from "@endosoflow/ui";

/** How a value is written for a customer: money as USD, a calendar date in words, text as it is. */
function fieldValue(field: PortalField): string {
  if (field.kind === "money") return formatMoneyText(field.value);
  if (field.kind === "date") return formatDay(field.value);
  return field.value;
}

function DocumentRow({ document }: { document: PortalDocument }) {
  const { notify } = useToast();
  const [busy, setBusy] = useState(false);

  async function download() {
    if (busy) return;
    setBusy(true);
    // Opened now, filled in later: a tab opened after a network wait is what browsers treat as a pop-up.
    const tab = window.open("", "_blank");
    if (tab) tab.opener = null;
    try {
      const link = await api.documentUrl(document.id);
      if (tab) tab.location.href = link.url;
      else window.location.assign(link.url);
    } catch (error) {
      tab?.close();
      notify({
        tone: "error",
        title: "No pudimos abrir el documento",
        message: errorText(error),
      });
    } finally {
      setBusy(false);
    }
  }

  return (
    <li className="doc-row">
      <span className="doc-icon">
        <Icon name="file-text" size={18} />
      </span>
      <div className="doc-info">
        <strong>{document.label}</strong>
        <small>
          {document.fileName} · {formatSize(document.sizeBytes)} · Cargado el{" "}
          {formatMoment(document.uploadedAt)}
        </small>
      </div>
      <button
        type="button"
        className="button button-secondary button-sm"
        onClick={() => void download()}
        disabled={busy}
        aria-label={`Descargar ${document.label}`}
      >
        <Icon name="download" size={15} /> {busy ? "Abriendo…" : "Descargar"}
      </button>
    </li>
  );
}

/** What the customer reads about one completed task: its recorded data and its files. */
export function SectionCard({ section }: { section: PortalSection }) {
  return (
    <section className="section-card">
      <header className="section-head">
        <h4>{section.title}</h4>
        <span className="section-date">
          <Icon name="check-circle" size={14} /> Completado el{" "}
          {formatMoment(section.completedAt)}
        </span>
      </header>
      {section.fields.length > 0 && (
        <dl className="field-grid">
          {section.fields.map((field) => (
            <div key={field.key} className="field-item">
              <dt>{field.label}</dt>
              <dd className={field.kind === "money" ? "is-money" : undefined}>
                {fieldValue(field)}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {section.documents.length > 0 && (
        <ul className="doc-list" aria-label={`Documentos de ${section.title}`}>
          {section.documents.map((document) => (
            <DocumentRow key={document.id} document={document} />
          ))}
        </ul>
      )}
    </section>
  );
}
