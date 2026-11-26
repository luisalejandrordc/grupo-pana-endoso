import { errorText } from "@endosoflow/api";
import {
  Badge,
  EmptyState,
  ErrorState,
  Icon,
  PageHeader,
  useDocumentTitle,
} from "@endosoflow/ui";
import { Link } from "react-router-dom";
import { formatMoment } from "../../shared/lib/format";
import type { PortalSaleSummary } from "../../shared/types";
import { saleStateLabels, saleStateTones } from "./labels";
import { MilestoneTrack } from "./MilestoneTrack";
import { progressOf } from "./progress";
import { useSales } from "./queries";
import { VehicleImage } from "./VehicleImage";

function SaleCard({ sale }: { sale: PortalSaleSummary }) {
  const progress = progressOf(sale.milestones, sale.state);
  const vehicle = `${sale.vehicle.model} ${sale.vehicle.version}`;
  return (
    <li>
      <article className="sale-card">
        <VehicleImage
          className="sale-card-photo"
          src={sale.vehicle.mainImageUrl}
          alt={`${vehicle}, color ${sale.vehicle.color}`}
        />
        <div className="sale-card-body">
          <div className="sale-card-top">
            <span className="sale-code">{sale.code}</span>
            <Badge tone={saleStateTones[sale.state]}>
              {saleStateLabels[sale.state]}
            </Badge>
          </div>
          <h2 className="sale-card-title">
            <Link to={`/sales/${sale.id}`} className="sale-card-link">
              {vehicle}
            </Link>
          </h2>
          <p className="sale-card-meta">
            Color {sale.vehicle.color} · Registrada el{" "}
            {formatMoment(sale.createdAt)}
          </p>
          <MilestoneTrack milestones={sale.milestones} state={sale.state} />
          <p className="sale-card-progress">
            <strong>{progress.summary}</strong>
            {sale.state === "active" && (
              <span>
                {" "}
                · {progress.done} de {progress.total} etapas completadas
              </span>
            )}
          </p>
        </div>
        <span className="sale-card-go" aria-hidden="true">
          <Icon name="chevron-right" size={20} />
        </span>
      </article>
    </li>
  );
}

function Loading() {
  return (
    <div className="sale-list" aria-busy="true" role="status">
      <span className="sr-only">Cargando sus compras…</span>
      {[0, 1].map((key) => (
        <div className="skeleton sale-skeleton" key={key} />
      ))}
    </div>
  );
}

/** The home of the portal: every purchase of the signed-in customer, newest first, with its progress. */
export function MySalesPage() {
  useDocumentTitle("Mis compras");
  const sales = useSales();

  return (
    <div className="stack">
      <PageHeader
        title="Mis compras"
        description="Siga el avance de cada una de sus compras en Grupo Pana."
      />
      {sales.isPending ? (
        <Loading />
      ) : sales.isError ? (
        <ErrorState
          title="No pudimos cargar sus compras"
          message={errorText(sales.error)}
          onRetry={() => void sales.refetch()}
        />
      ) : sales.data.items.length === 0 ? (
        <EmptyState
          page
          icon="car"
          title="Aún no tiene compras registradas"
          description="Cuando su asesor de Grupo Pana registre su compra, la verá aquí y podrá seguir su avance."
        />
      ) : (
        <ul className="sale-list">
          {sales.data.items.map((sale) => (
            <SaleCard key={sale.id} sale={sale} />
          ))}
        </ul>
      )}
    </div>
  );
}
