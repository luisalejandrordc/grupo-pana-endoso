import { ApiError, errorText } from "@endosoflow/api";
import {
  BackLink,
  Badge,
  Callout,
  EmptyState,
  ErrorState,
  formatMoneyText,
  useDocumentTitle,
} from "@endosoflow/ui";
import { Link, useParams } from "react-router-dom";
import { formatMoment } from "../../shared/lib/format";
import type { PortalSaleDetail } from "../../shared/types";
import { AdvisorCard } from "./AdvisorCard";
import {
  categoryLabels,
  labelOf,
  saleStateLabels,
  saleStateTones,
} from "./labels";
import { MilestoneTrack } from "./MilestoneTrack";
import { progressOf } from "./progress";
import { useSale } from "./queries";
import { Timeline } from "./Timeline";
import { VehicleGallery } from "./VehicleGallery";

function Loading() {
  return (
    <div className="stack" aria-busy="true" role="status">
      <span className="sr-only">Cargando su compra…</span>
      <div className="skeleton skeleton-title" />
      <div className="skeleton skeleton-block detail-skeleton" />
      <div className="skeleton skeleton-block" />
    </div>
  );
}

function StateBanner({ sale }: { sale: PortalSaleDetail }) {
  if (sale.state === "cancelled")
    return (
      <Callout tone="quiet" icon="info">
        <strong>Esta compra fue cancelada.</strong> Sus documentos siguen
        disponibles para consulta. Si tiene dudas, comuníquese con su asesor.
      </Callout>
    );
  if (sale.state === "completed")
    return (
      <Callout tone="done" icon="circle-check" role="status">
        <strong>Su compra está completa.</strong>{" "}
        {sale.completedAt && `Finalizó el ${formatMoment(sale.completedAt)}. `}
        Gracias por confiar en Grupo Pana.
      </Callout>
    );
  return null;
}

function Detail({ sale }: { sale: PortalSaleDetail }) {
  const { vehicle } = sale;
  const name = `${vehicle.model} ${vehicle.version}`;
  const progress = progressOf(sale.milestones, sale.state);
  useDocumentTitle(`${name} · ${sale.code}`);

  return (
    <div className="stack">
      <BackLink to="/">Mis compras</BackLink>

      <section className="panel hero" aria-labelledby="sale-title">
        <VehicleGallery vehicle={vehicle} />
        <div className="hero-info">
          <div className="hero-top">
            <span className="sale-code">{sale.code}</span>
            <Badge tone={saleStateTones[sale.state]}>
              {saleStateLabels[sale.state]}
            </Badge>
          </div>
          <h1 id="sale-title">{name}</h1>
          <p className="hero-color">Color {vehicle.color}</p>
          {vehicle.categories.length > 0 && (
            <ul className="hero-chips" aria-label="Categorías">
              {vehicle.categories.map((category) => (
                <li key={category}>
                  <span className="chip">
                    {labelOf(categoryLabels, category)}
                  </span>
                </li>
              ))}
            </ul>
          )}
          <dl className="hero-facts">
            <div>
              <dt>Registrada el</dt>
              <dd>{formatMoment(sale.createdAt)}</dd>
            </div>
            <div>
              <dt>Precio de referencia</dt>
              <dd>{formatMoneyText(vehicle.referencePrice)}</dd>
            </div>
          </dl>
          <div className="hero-progress">
            <MilestoneTrack milestones={sale.milestones} state={sale.state} />
            <p>
              <strong>{progress.summary}</strong>
              {sale.state === "active" && (
                <span>
                  {" "}
                  · {progress.done} de {progress.total} etapas completadas
                </span>
              )}
            </p>
          </div>
        </div>
      </section>

      <StateBanner sale={sale} />

      <div className="detail-columns">
        <section aria-labelledby="progress-title" className="detail-main">
          <h2 id="progress-title" className="section-title">
            Avance de su compra
          </h2>
          <Timeline
            milestones={sale.milestones}
            sections={sale.sections}
            state={sale.state}
          />
        </section>
        <aside className="detail-side">
          <AdvisorCard advisor={sale.advisor} />
          <p className="reference-note">
            El precio de referencia es el vigente al momento de registrar su
            compra. El precio final cotizado aparece en la etapa de cotización
            cuando esté lista.
          </p>
        </aside>
      </div>
    </div>
  );
}

/** One purchase in full: the vehicle, the six stages with what was recorded in each, and the advisor. */
export function SaleDetailPage() {
  const { saleId = "" } = useParams();
  const sale = useSale(saleId);

  if (sale.isPending) return <Loading />;

  if (sale.isError) {
    // A purchase that is someone else's answers exactly like one that does not exist: the page says the same.
    if (sale.error instanceof ApiError && sale.error.status === 404)
      return (
        <EmptyState
          page
          icon="search"
          title="No encontramos esta compra"
          description="Es posible que el enlace esté incompleto o que la compra no sea suya."
          actions={
            <Link className="button button-primary" to="/">
              Ver mis compras
            </Link>
          }
        />
      );
    return (
      <ErrorState
        page
        title="No pudimos cargar esta compra"
        message={errorText(sale.error)}
        onRetry={() => void sale.refetch()}
        actions={
          <Link className="button button-ghost" to="/">
            Mis compras
          </Link>
        }
      />
    );
  }

  return <Detail sale={sale.data} />;
}
