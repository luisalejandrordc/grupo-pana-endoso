import { Link } from "react-router-dom";
import { EmptyState } from "@endosoflow/ui";

export function NotFoundPage() {
  return (
    <EmptyState
      page
      icon="map-pin"
      title="Esta página no existe"
      description="Revisa la dirección o vuelve al inicio."
      actions={
        <Link className="button button-primary" to="/sales">
          Ir a ventas
        </Link>
      }
    />
  );
}
