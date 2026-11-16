import { useState } from "react";
import { Link, Navigate, Route, Routes } from "react-router-dom";
import { CatalogPage } from "../features/catalog/CatalogPage";
import { CustomersPage } from "../features/customers/CustomersPage";
import { LoginPage } from "../features/auth/LoginPage";
import { useAuth } from "../features/auth/useAuth";
import { ProfilePage } from "../features/profile/ProfilePage";
import { NewSalePage } from "../features/sales/NewSalePage";
import { SaleDetailPage } from "../features/sales/SaleDetailPage";
import { SalesPage } from "../features/sales/SalesPage";
import { StaffPage } from "../features/staff/StaffPage";
import { ToastView } from "../shared/ui/ToastView";
import { useToast } from "../shared/ui/useToast";
import { AppShell } from "./layout/AppShell";

export default function App() {
  const { user, authLoading, authError, signIn, signOut, reload } = useAuth();
  const [query, setQuery] = useState("");
  const { toast, notify, dismiss } = useToast();

  if (authLoading)
    return (
      <div className="boot" role="status" aria-label="Cargando">
        <img src="/endosoflow-logo-square.png" alt="" width={72} height={72} />
      </div>
    );

  if (!user)
    return (
      <>
        <LoginPage onLogin={signIn} busy={authLoading} error={authError} />
        {toast && <ToastView toast={toast} onClose={dismiss} />}
      </>
    );

  return (
    <>
      <AppShell
        user={user}
        query={query}
        setQuery={setQuery}
        onSignOut={signOut}
      >
        <Routes>
          <Route path="/" element={<Navigate to="/sales" replace />} />
          <Route
            path="/sales"
            element={<SalesPage role={user.role} query={query} />}
          />
          <Route
            path="/sales/new"
            element={
              <NewSalePage role={user.role} userId={user.id} toast={notify} />
            }
          />
          <Route
            path="/sales/:saleId"
            element={<SaleDetailPage toast={notify} />}
          />
          <Route
            path="/sales/:saleId/:tab"
            element={<SaleDetailPage toast={notify} />}
          />
          <Route
            path="/customers"
            element={
              <CustomersPage query={query} role={user.role} toast={notify} />
            }
          />
          <Route
            path="/catalog"
            element={
              <CatalogPage query={query} role={user.role} toast={notify} />
            }
          />
          <Route
            path="/staff"
            element={
              user.role === "system_admin" ? (
                <StaffPage user={user} toast={notify} />
              ) : (
                <Navigate to="/sales" replace />
              )
            }
          />
          <Route
            path="/profile"
            element={
              <ProfilePage
                user={user}
                onSignOut={signOut}
                onPhotoChanged={reload}
                toast={notify}
              />
            }
          />
          <Route
            path="*"
            element={
              <div className="empty empty-page">
                <strong>Esta página no existe</strong>
                <p>Revisa la dirección o vuelve al inicio.</p>
                <Link className="button button-primary" to="/sales">
                  Ir a ventas
                </Link>
              </div>
            }
          />
        </Routes>
      </AppShell>
      {toast && <ToastView toast={toast} onClose={dismiss} />}
    </>
  );
}
