import { useCallback, useRef, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { formatDate, formatMoneyText } from "../../shared/lib/format";
import { Avatar } from "../../shared/ui/Avatar";
import { ErrorState } from "../../shared/ui/EmptyState";
import { Icon } from "../../shared/ui/Icon";
import { Menu, type MenuItem } from "../../shared/ui/Menu";
import { BackLink } from "../../shared/ui/PageHeader";
import { Tabs } from "../../shared/ui/Tabs";
import type { Toast } from "../../shared/ui/toast";
import { salesApi, type ApiSale } from "./api";
import {
  CancelSaleDialog,
  ConfirmDialog,
  DeleteSaleDialog,
  ReasonDialog,
  ReassignDialog,
} from "./dialogs";
import { cancelLabels, errorText } from "./labels";
import { StateBadge, TempBadge } from "./pills";
import { useAdvisors, useSale, useSaleMutation } from "./queries";
import { Route } from "./Route";
import { DocumentsTab, HistoryTab, NotesTab } from "./SaleTabs";
import { TaskPanel } from "./TaskPanel";

const tabs = [
  { id: "tasks", label: "Tareas" },
  { id: "documents", label: "Documentos" },
  { id: "notes", label: "Notas" },
  { id: "history", label: "Historial" },
] as const;
type TabId = (typeof tabs)[number]["id"];

type Dialog = "cancel" | "reassign" | "reopen" | "delete" | null;

export function SaleDetailPage({ toast }: { toast: (toast: Toast) => void }) {
  const { saleId = "", tab = "tasks" } = useParams();
  const sale = useSale(saleId);
  if (sale.isPending)
    return (
      <div className="stack" aria-busy="true">
        <span className="skeleton skeleton-title" />
        <span className="skeleton skeleton-block" />
        <span className="skeleton skeleton-block" />
      </div>
    );
  if (sale.isError)
    return (
      <ErrorState
        page
        title="No se encontró la venta"
        message={`${errorText(sale.error)} Puede que no exista o que no tengas acceso a ella.`}
        actions={
          <Link className="button button-primary" to="/sales">
            Volver a ventas
          </Link>
        }
      />
    );
  return (
    <Loaded
      sale={sale.data}
      tab={(tabs.some((t) => t.id === tab) ? tab : "tasks") as TabId}
      toast={toast}
    />
  );
}

function Loaded({
  sale,
  tab,
  toast,
}: {
  sale: ApiSale;
  tab: TabId;
  toast: (toast: Toast) => void;
}) {
  const navigate = useNavigate();
  const [search] = useSearchParams();
  const [dialog, setDialog] = useState<Dialog>(null);
  const [leaving, setLeaving] = useState<null | (() => void)>(null);
  const dirty = useRef(false);
  const advisors = useAdvisors(
    sale.permissions.canReassign && dialog === "reassign",
  );
  const can = sale.permissions;

  const requested = Number(search.get("task"));
  const selected =
    requested >= 1 && requested <= 13
      ? requested
      : (sale.currentTask?.number ?? (sale.state === "completed" ? 13 : 1));

  const cancel = useSaleMutation(
    (input: {
      category: Parameters<typeof salesApi.cancel>[1];
      text: string;
    }) => salesApi.cancel(sale.id, input.category, input.text),
  );
  const reassign = useSaleMutation(
    (input: { advisorId: string; reason: string }) =>
      salesApi.reassign(sale.id, input.advisorId, input.reason),
  );
  const reopen = useSaleMutation((reason: string) =>
    salesApi.reopen(sale.id, reason),
  );
  const remove = useSaleMutation((input: { reason: string; confirm: string }) =>
    salesApi.remove(sale.id, input.reason, input.confirm),
  );

  const onDirty = useCallback((value: boolean) => {
    dirty.current = value;
  }, []);

  /** Runs `go` now, or after the person agrees to drop the changes they have not saved. */
  function guarded(go: () => void) {
    if (dirty.current) setLeaving(() => go);
    else go();
  }
  const openTask = (number: number) =>
    guarded(() => navigate(`/sales/${sale.id}/tasks?task=${number}`));
  const openTab = (id: TabId) =>
    guarded(() =>
      navigate(
        id === "tasks"
          ? `/sales/${sale.id}?task=${selected}`
          : `/sales/${sale.id}/${id}`,
      ),
    );

  const items: MenuItem[] = [
    ...(can.canReassign
      ? [
          {
            label: "Reasignar asesor",
            icon: "users" as const,
            onSelect: () => setDialog("reassign"),
          },
        ]
      : []),
    ...(can.canReopen
      ? [
          {
            label: "Reabrir venta",
            icon: "refresh" as const,
            onSelect: () => setDialog("reopen"),
          },
        ]
      : []),
    ...(can.canCancel
      ? [
          {
            label: "Cancelar venta",
            icon: "circle-x" as const,
            danger: true,
            onSelect: () => setDialog("cancel"),
          },
        ]
      : []),
    ...(can.canDelete
      ? [
          {
            label: "Eliminar definitivamente",
            icon: "alert" as const,
            danger: true,
            onSelect: () => setDialog("delete"),
          },
        ]
      : []),
  ];
  const current = sale.currentTask;
  const noun = `${sale.customer.name} · ${sale.vehicle.model}`;

  return (
    <div className="stack">
      <BackLink to="/sales">Ventas</BackLink>

      <header className="sale-head">
        <div className="sale-head-main">
          <div className="sale-head-tags">
            <span className="code">{sale.code}</span>
            <StateBadge state={sale.state} />
            {sale.temperature === "hot" && <TempBadge temperature="hot" />}
          </div>
          <h1>{sale.customer.name}</h1>
          <p className="sale-head-meta">
            <span>
              <Icon name="car" size={15} />
              {sale.vehicle.model} {sale.vehicle.version} · {sale.vehicle.color}
            </span>
            <span>
              <Icon name="calendar" size={15} />
              Registrada el {formatDate(sale.createdAt)}
            </span>
          </p>
        </div>
        <div className="sale-head-actions">
          {sale.state === "active" && current && tab !== "tasks" && (
            <button
              className="button button-primary"
              onClick={() =>
                navigate(`/sales/${sale.id}?task=${current.number}`)
              }
            >
              Continuar: tarea {current.number}
            </button>
          )}
          <Menu label="Acciones de la venta" items={items}>
            Acciones
          </Menu>
        </div>
      </header>

      {sale.state === "cancelled" && sale.cancellation && (
        <div className="callout callout-error">
          <Icon name="circle-x" size={18} />
          <div>
            <strong>
              Cancelada el {formatDate(sale.cancellation.cancelledAt)} por{" "}
              {sale.cancellation.cancelledByName}
            </strong>
            <p>
              {cancelLabels[sale.cancellation.category]}:{" "}
              {sale.cancellation.text} (en la tarea{" "}
              {sale.cancellation.atTaskNumber})
            </p>
          </div>
        </div>
      )}
      {sale.state === "completed" && (
        <div className="callout callout-done">
          <Icon name="circle-check" size={18} />
          <div>
            <strong>
              Venta completada
              {sale.completedAt ? ` el ${formatDate(sale.completedAt)}` : ""}
            </strong>
            <p>
              Las tareas pueden corregirse con un motivo; reabre la tarea 13 si
              la venta debe seguir en curso.
            </p>
          </div>
        </div>
      )}

      <Route
        sale={sale}
        selected={tab === "tasks" ? selected : null}
        onSelect={openTask}
      />

      <div className="detail-layout">
        <div className="detail-main">
          <Tabs
            label="Secciones de la venta"
            items={tabs}
            value={tab}
            onChange={openTab}
          />
          {tab === "tasks" && (
            <TaskPanel
              sale={sale}
              number={selected}
              toast={toast}
              onDirtyChange={onDirty}
            />
          )}
          {tab === "documents" && (
            <DocumentsTab sale={sale} onOpenTask={openTask} toast={toast} />
          )}
          {tab === "notes" && (
            <NotesTab key={sale.version} sale={sale} toast={toast} />
          )}
          {tab === "history" && <HistoryTab sale={sale} />}
        </div>

        <aside className="detail-side" aria-label="Datos de la venta">
          <section className="side-card">
            {sale.vehicle.imageUrl ? (
              <img
                className="side-vehicle"
                src={sale.vehicle.imageUrl}
                alt={`${sale.vehicle.model} ${sale.vehicle.color}`}
              />
            ) : (
              <div className="side-vehicle side-vehicle-empty">
                <Icon name="car" size={28} />
              </div>
            )}
            <h3>
              {sale.vehicle.model} {sale.vehicle.version}
            </h3>
            <dl>
              <div>
                <dt>Color</dt>
                <dd>{sale.vehicle.color}</dd>
              </div>
              <div>
                <dt>Precio de lista</dt>
                <dd>{formatMoneyText(sale.vehicle.referencePrice)}</dd>
              </div>
              {sale.quotedFinalPrice && (
                <div>
                  <dt>Precio final cotizado</dt>
                  <dd>{formatMoneyText(sale.quotedFinalPrice)}</dd>
                </div>
              )}
              {sale.vehicle.series && (
                <div>
                  <dt>Serie</dt>
                  <dd>{sale.vehicle.series}</dd>
                </div>
              )}
              {sale.vehicle.vin && (
                <div>
                  <dt>VIN</dt>
                  <dd className="mono">{sale.vehicle.vin}</dd>
                </div>
              )}
              {sale.vehicle.licensePlate && (
                <div>
                  <dt>Placa</dt>
                  <dd>{sale.vehicle.licensePlate}</dd>
                </div>
              )}
            </dl>
          </section>
          <section className="side-card">
            <h3>Cliente</h3>
            <p className="side-name">{sale.customer.name}</p>
            <p className="side-sub">
              {sale.customer.documentType} {sale.customer.documentNumber}
            </p>
          </section>
          <section className="side-card">
            <h3>Asesor responsable</h3>
            <div className="side-person">
              <Avatar
                id={sale.advisor.id}
                name={sale.advisor.name}
                hasPhoto={sale.advisor.hasPhoto}
                size={40}
              />
              <div>
                <p className="side-name">{sale.advisor.name}</p>
                <p className="side-sub">
                  <a href={`mailto:${sale.advisor.email}`}>
                    {sale.advisor.email}
                  </a>
                </p>
                <p className="side-sub">{sale.advisor.phone}</p>
              </div>
            </div>
          </section>
        </aside>
      </div>

      {dialog === "cancel" && (
        <CancelSaleDialog
          onClose={() => setDialog(null)}
          onConfirm={async (category, text) => {
            await cancel.mutateAsync({ category, text });
            toast({ title: "Venta cancelada", message: noun, tone: "success" });
          }}
        />
      )}
      {dialog === "reassign" && (
        <ReassignDialog
          sale={sale}
          advisors={advisors.data ?? []}
          loading={advisors.isPending}
          onClose={() => setDialog(null)}
          onConfirm={async (advisorId, reason) => {
            await reassign.mutateAsync({ advisorId, reason });
            toast({
              title: "Asesor reasignado",
              message: noun,
              tone: "success",
            });
          }}
        />
      )}
      {dialog === "reopen" && (
        <ReasonDialog
          title="Reabrir venta"
          description="La venta vuelve a estar en curso. Sus plazos retoman el tiempo que les quedaba al cancelarla."
          label="Motivo de la reapertura"
          confirmLabel="Reabrir venta"
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            await reopen.mutateAsync(reason);
            toast({ title: "Venta reabierta", message: noun, tone: "success" });
          }}
        />
      )}
      {dialog === "delete" && (
        <DeleteSaleDialog
          sale={sale}
          onClose={() => setDialog(null)}
          onConfirm={async (reason, confirm) => {
            await remove.mutateAsync({ reason, confirm });
            toast({
              title: "Venta eliminada",
              message: sale.code,
              tone: "success",
            });
            navigate("/sales", { replace: true });
          }}
        />
      )}
      {leaving && (
        <ConfirmDialog
          title="Hay cambios sin guardar"
          message="Si continúas, los cambios que escribiste en esta tarea se perderán."
          confirmLabel="Descartar y continuar"
          danger
          onClose={() => setLeaving(null)}
          onConfirm={async () => {
            dirty.current = false;
            const go = leaving;
            setLeaving(null);
            go();
          }}
        />
      )}
    </div>
  );
}
