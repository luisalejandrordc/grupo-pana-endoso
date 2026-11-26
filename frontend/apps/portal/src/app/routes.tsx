import { Navigate, Route, Routes } from "react-router-dom";
import { AccountPage } from "../features/account/AccountPage";
import { MySalesPage } from "../features/sales/MySalesPage";
import { SaleDetailPage } from "../features/sales/SaleDetailPage";
import { NotFoundPage } from "./NotFoundPage";

/** The screens behind the login. */
export function PortalRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sales" replace />} />
      <Route path="/sales" element={<MySalesPage />} />
      <Route path="/sales/:saleId" element={<SaleDetailPage />} />
      <Route path="/account" element={<AccountPage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
