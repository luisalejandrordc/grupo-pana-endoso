import { EmptyState, useDocumentTitle } from "@endosoflow/ui";
import { Link } from "react-router-dom";

export function NotFoundPage() {
  useDocumentTitle("Página no encontrada");
  return (
    <EmptyState
      page
      icon="search"
      title="No encontramos esta página"
      description="Es posible que el enlace esté incompleto."
      actions={
        <Link className="button button-primary" to="/">
          Ir a mis compras
        </Link>
      }
    />
  );
}
