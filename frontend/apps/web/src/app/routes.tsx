import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth, useCurrentUser } from "../features/auth/useAuth";
import { useToast } from "@endosoflow/ui";
import { RequireRole } from "./guards/RequireRole";
import { NotFoundPage } from "./NotFoundPage";

/*
 * Every page is loaded the first time it is visited, so the login and the first screen stay light.
 * The route components below are the only place that knows where a page's inputs come from (the
 * session, the address, the notifications): the pages themselves just receive them as props.
 */
const SalesPage = lazy(() =>
  import("../features/sales/SalesPage").then((m) => ({ default: m.SalesPage })),
);
const NewSalePage = lazy(() =>
  import("../features/sales/NewSalePage").then((m) => ({
    default: m.NewSalePage,
  })),
);
const SaleDetailPage = lazy(() =>
  import("../features/sales/SaleDetailPage").then((m) => ({
    default: m.SaleDetailPage,
  })),
);
const CustomersPage = lazy(() =>
  import("../features/customers/CustomersPage").then((m) => ({
    default: m.CustomersPage,
  })),
);
const CatalogPage = lazy(() =>
  import("../features/catalog/CatalogPage").then((m) => ({
    default: m.CatalogPage,
  })),
);
const StaffPage = lazy(() =>
  import("../features/staff/StaffPage").then((m) => ({ default: m.StaffPage })),
);
const ProfilePage = lazy(() =>
  import("../features/profile/ProfilePage").then((m) => ({
    default: m.ProfilePage,
  })),
);

function PageFallback() {
  return (
    <div className="stack" aria-busy="true" aria-label="Cargando">
      <span className="skeleton skeleton-title" />
      <span className="skeleton skeleton-block" />
      <span className="skeleton skeleton-block" />
    </div>
  );
}

function SalesRoute() {
  const { role } = useCurrentUser();
  return <SalesPage role={role} />;
}

function NewSaleRoute() {
  const { role, id } = useCurrentUser();
  const { notify } = useToast();
  return <NewSalePage role={role} userId={id} toast={notify} />;
}

function SaleDetailRoute() {
  const { notify } = useToast();
  return <SaleDetailPage toast={notify} />;
}

function CustomersRoute() {
  const { role } = useCurrentUser();
  const { notify } = useToast();
  return <CustomersPage role={role} toast={notify} />;
}

function CatalogRoute() {
  const { role } = useCurrentUser();
  const { notify } = useToast();
  return <CatalogPage role={role} toast={notify} />;
}

function StaffRoute() {
  const user = useCurrentUser();
  const { reload } = useAuth();
  const { notify } = useToast();
  return <StaffPage user={user} toast={notify} onSelfUpdated={reload} />;
}

function ProfileRoute() {
  const user = useCurrentUser();
  const { signOut, reload } = useAuth();
  const { notify } = useToast();
  return (
    <ProfilePage
      user={user}
      onSignOut={signOut}
      onPhotoChanged={reload}
      toast={notify}
    />
  );
}

export function AppRoutes() {
  return (
    <Suspense fallback={<PageFallback />}>
      <Routes>
        <Route path="/" element={<Navigate to="/sales" replace />} />
        <Route path="/sales" element={<SalesRoute />} />
        <Route path="/sales/new" element={<NewSaleRoute />} />
        <Route path="/sales/:saleId" element={<SaleDetailRoute />} />
        <Route path="/sales/:saleId/:tab" element={<SaleDetailRoute />} />
        <Route path="/customers" element={<CustomersRoute />} />
        <Route path="/catalog" element={<CatalogRoute />} />
        <Route
          path="/staff"
          element={
            <RequireRole roles={["system_admin"]}>
              <StaffRoute />
            </RequireRole>
          }
        />
        <Route path="/profile" element={<ProfileRoute />} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  );
}
