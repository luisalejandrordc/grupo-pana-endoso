import { useState } from "react";
import { LoginPage } from "../features/auth/LoginPage";
import { useAuth } from "../features/auth/useAuth";
import { CatalogPage } from "../features/catalog/CatalogPage";
import { CustomersPage } from "../features/customers/CustomersPage";
import { ProfilePage } from "../features/profile/ProfilePage";
import { ConfirmModal } from "../features/sales/ConfirmModal";
import { NewSaleWizard } from "../features/sales/NewSaleWizard";
import { SaleDetail } from "../features/sales/SaleDetail";
import { LiveNewSale } from "../features/sales/live/LiveNewSale";
import { LiveSaleDetail } from "../features/sales/live/LiveSaleDetail";
import { LiveSalesWorkspace } from "../features/sales/live/LiveSalesWorkspace";
import { SalesWorkspace } from "../features/sales/SalesWorkspace";
import type { ModalAction } from "../features/sales/types";
import { useSaleActionModal } from "../features/sales/useSaleActionModal";
import { StaffPage } from "../features/staff/StaffPage";
import { demoMode } from "../shared/config";
import type { Customer, DocumentType } from "../shared/types";
import { ApiGap } from "../shared/ui/ApiGap";
import { Brand } from "../shared/ui/Brand";
import { ToastView } from "../shared/ui/ToastView";
import { useToast } from "../shared/ui/useToast";
import { AppShell } from "./layout/AppShell";
import type { View } from "./navigation";
import { useDemoWorkspace } from "./useDemoWorkspace";

export default function App() {
  const auth = useAuth();
  const { toast, notify, dismiss } = useToast();
  const workspace = useDemoWorkspace();
  const {
    sales,
    customers,
    vehicles,
    employees,
    activeSale,
    setActiveSaleId,
    updateSale,
    saveNotes,
    createSale,
  } = workspace;
  const saleModal = useSaleActionModal({
    sales,
    employees,
    updateSale,
    notify,
  });

  const [page, setPage] = useState<View>("sales");
  const [query, setQuery] = useState("");
  const [loginBusy, setLoginBusy] = useState(false);
  const [showMobileNav, setShowMobileNav] = useState(false);
  const [presetCustomer, setPresetCustomer] = useState<Customer | null>(null);
  // The sale open in the real (non-demo) workspace.
  const [liveSaleId, setLiveSaleId] = useState<string | null>(null);

  const currentUser = auth.user;

  function navigate(nextPage: View, saleId?: string) {
    if (saleId) {
      setActiveSaleId(saleId);
      setLiveSaleId(saleId);
    }
    setPage(nextPage);
    setShowMobileNav(false);
    setQuery("");
  }

  async function login(input: {
    documentType: DocumentType;
    documentNumber: string;
    password: string;
    rememberMe: boolean;
  }) {
    setLoginBusy(true);
    try {
      await auth.signIn(input);
      setPage("sales");
      notify({
        title: demoMode ? "Bienvenida al modo demo" : "Sesión iniciada",
        message: "Tu espacio de trabajo está listo.",
        tone: "success",
      });
    } finally {
      setLoginBusy(false);
    }
  }

  async function signOut() {
    await auth.signOut();
    setPage("sales");
    notify({
      title: "Sesión cerrada",
      message: "Has salido de EndosoFlow de forma segura.",
      tone: "success",
    });
  }

  const handleTaskAction = (action: ModalAction, taskNumber: number) =>
    saleModal.open(action, activeSale.id, taskNumber);

  if (auth.authLoading)
    return (
      <div className="loading-screen">
        <Brand />
        <div className="loading-pulse" />
        <p>Conectando con el portal de colaboradores…</p>
      </div>
    );
  if (!currentUser)
    return (
      <LoginPage
        onLogin={login}
        onDemoLogin={() => {
          auth.signInDemo();
          setPage("sales");
          notify({
            title: "Bienvenida al modo demo",
            message: "Usa el selector de rol para probar distintas vistas.",
            tone: "success",
          });
        }}
        busy={loginBusy}
        error={auth.authError}
      />
    );

  return (
    <>
      <AppShell
        page={page}
        user={currentUser}
        query={query}
        setQuery={setQuery}
        onNavigate={navigate}
        onSignOut={signOut}
        onRoleChange={auth.changeDemoRole}
        mobileOpen={showMobileNav}
        onMenuClick={() => setShowMobileNav(true)}
        onCloseMobile={() => setShowMobileNav(false)}
      >
        {page === "sales" &&
          (demoMode ? (
            <SalesWorkspace
              sales={sales}
              role={currentUser.role}
              userId={currentUser.id}
              query={query}
              onCreate={() => navigate("new-sale")}
              onOpen={(sale) => navigate("sale-detail", sale.id)}
              demo={demoMode}
            />
          ) : (
            <LiveSalesWorkspace
              role={currentUser.role}
              query={query}
              onCreate={() => navigate("new-sale")}
              onOpen={(saleId) => navigate("sale-detail", saleId)}
            />
          ))}
        {page === "sale-detail" &&
          (demoMode ? (
            activeSale ? (
              <SaleDetail
                key={activeSale.id}
                sale={activeSale}
                role={currentUser.role}
                onBack={() => navigate("sales")}
                onTaskAction={handleTaskAction}
                onSaleAction={(action) => saleModal.open(action, activeSale.id)}
                onSaveNotes={(notes) => saveNotes(activeSale.id, notes)}
                toast={notify}
              />
            ) : (
              <ApiGap
                title="Venta no encontrada"
                detail="La venta solicitada no está disponible en tu ámbito de acceso."
              />
            )
          ) : liveSaleId ? (
            <LiveSaleDetail
              key={liveSaleId}
              saleId={liveSaleId}
              role={currentUser.role}
              userId={currentUser.id}
              onBack={() => navigate("sales")}
              toast={notify}
            />
          ) : (
            <ApiGap
              title="Venta no encontrada"
              detail="Vuelve a la lista de ventas y abre una operación."
            />
          ))}
        {page === "new-sale" &&
          (demoMode ? (
            <NewSaleWizard
              customers={customers}
              vehicles={vehicles}
              employees={employees}
              role={currentUser.role}
              userId={currentUser.id}
              initialCustomer={presetCustomer}
              onCancel={() => {
                setPresetCustomer(null);
                navigate("sales");
              }}
              onCreate={(draft) => {
                const sale = createSale(draft);
                setPresetCustomer(null);
                setPage("sale-detail");
                notify({
                  title: "Venta creada correctamente",
                  message: `${sale.code} ya tiene sus 13 tareas inicializadas.`,
                  tone: "success",
                });
              }}
              toast={notify}
            />
          ) : (
            <LiveNewSale
              role={currentUser.role}
              userId={currentUser.id}
              onCancel={() => navigate("sales")}
              onCreated={(saleId) => navigate("sale-detail", saleId)}
              toast={notify}
            />
          ))}
        {page === "customers" && (
          <CustomersPage
            customers={customers}
            query={query}
            onNewSale={(customer) => {
              setPresetCustomer(customer || null);
              navigate("new-sale");
            }}
            demo={demoMode}
            role={currentUser.role}
            toast={notify}
          />
        )}
        {page === "catalog" && (
          <CatalogPage
            vehicles={vehicles}
            query={query}
            demo={demoMode}
            role={currentUser.role}
            toast={notify}
          />
        )}
        {page === "staff" && (
          <StaffPage
            key={currentUser.role}
            user={currentUser}
            demo={demoMode}
            toast={notify}
          />
        )}
        {page === "profile" && (
          <ProfilePage
            user={currentUser}
            demo={demoMode}
            onSignOut={signOut}
            toast={notify}
          />
        )}
      </AppShell>
      {saleModal.modal && activeSale && (
        <ConfirmModal
          modal={saleModal.modal}
          sale={
            sales.find((sale) => sale.id === saleModal.modal?.saleId) ||
            activeSale
          }
          reason={saleModal.reason}
          setReason={saleModal.setReason}
          category={saleModal.cancelCategory}
          setCategory={saleModal.setCancelCategory}
          reassignee={saleModal.reassignee}
          setReassignee={saleModal.setReassignee}
          employees={employees}
          onClose={saleModal.close}
          onConfirm={saleModal.confirm}
        />
      )}
      {toast && <ToastView toast={toast} onClose={dismiss} />}
    </>
  );
}
