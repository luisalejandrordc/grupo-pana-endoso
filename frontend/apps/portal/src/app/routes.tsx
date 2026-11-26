import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { AccountPage } from "../features/account/AccountPage";
import { MySalesPage } from "../features/sales/MySalesPage";
import { SaleDetailPage } from "../features/sales/SaleDetailPage";
import { NotFoundPage } from "./NotFoundPage";

/** Old documentation and bookmarks speak Spanish; the emails speak `/sales/:id`. Both reach the same place. */
function LegacyPurchase() {
  const { saleId = "" } = useParams();
  return <Navigate to={`/sales/${saleId}`} replace />;
}

/** The screens behind the login. */
export function PortalRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sales" replace />} />
      <Route path="/sales" element={<MySalesPage />} />
      <Route path="/sales/:saleId" element={<SaleDetailPage />} />
      <Route path="/compras" element={<Navigate to="/" replace />} />
      <Route path="/compras/:saleId/*" element={<LegacyPurchase />} />
      <Route path="/account" element={<AccountPage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
