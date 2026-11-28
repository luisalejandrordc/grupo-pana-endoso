import { useQuery } from "@tanstack/react-query";
import { api } from "../../shared/api/client";
import { type Schemas } from "@endosoflow/api";
import { Badge } from "@endosoflow/ui";
import type { SystemData } from "./api";
import {
  Card,
  EmptyNote,
  KpiCard,
  PeriodNote,
  RankList,
  colors,
} from "./components";
import { formatCount } from "./format";
import { labelFor, notificationTypeLabels } from "./labels";

type Quota = Schemas["IdentityQuota"];

/** The monthly Codart usage; the endpoint is the one the customer lookup already uses. */
function useQuota() {
  return useQuery({
    queryKey: ["analytics", "codart-quota"] as const,
    queryFn: () => api.request<Quota>("/lookups/identity/quota"),
    staleTime: 60_000,
  });
}

/** How the system itself is doing: email delivery, sign-ins, accounts and storage upkeep. Admins only. */
export function SystemTab({ data }: { data: SystemData }) {
  const quota = useQuota();
  const { notifications, security, accounts, storage } = data;
  const quotaText = quota.data
    ? `${formatCount(quota.data.used)} de ${formatCount(quota.data.limit)}`
    : "—";

  return (
    <div className="stack an-sections">
      <PeriodNote meta={data.meta} compare={false} />
      <div className="an-kpis">
        <KpiCard
          label="Consultas Codart del mes"
          value={quotaText}
          tag="Ahora"
          tone={
            quota.data && quota.data.used >= quota.data.limit * 0.9
              ? "warn"
              : "neutral"
          }
          note="Cuota mensual de consultas de identidad"
        />
        <KpiCard
          label="Correos pendientes"
          value={formatCount(notifications.pending)}
          tag="Ahora"
          tone={notifications.pending > 20 ? "warn" : "neutral"}
          note="En cola para enviarse"
        />
        <KpiCard
          label="Correos fallidos"
          value={formatCount(notifications.failed)}
          tag="Periodo"
          tone={notifications.failed > 0 ? "late" : "neutral"}
          note={`${formatCount(notifications.sent)} enviados · ${formatCount(notifications.skipped)} omitidos`}
        />
        <KpiCard
          label="Inicios de sesión fallidos"
          value={formatCount(security.failedSignIns)}
          tag="Periodo"
          tone={security.failedSignIns > 0 ? "warn" : "neutral"}
          note={`${formatCount(security.successfulSignIns)} correctos`}
        />
        <KpiCard
          label="Cuentas bloqueadas"
          value={formatCount(security.lockedAccounts)}
          tag="Ahora"
          tone={security.lockedAccounts > 0 ? "warn" : "neutral"}
          note="Colaboradores con bloqueo vigente"
        />
        <KpiCard
          label="Colaboradores activos"
          value={formatCount(accounts.activeEmployees)}
          tag="Ahora"
          note={`${formatCount(accounts.notActivatedEmployees)} sin activar su cuenta`}
          definition="Cuentas activas de colaboradores. Sin activar son las que aún no definieron su contraseña."
        />
        <KpiCard
          label="Archivos por eliminar"
          value={formatCount(storage.pendingDeletions)}
          tag="Ahora"
          tone={storage.pendingDeletions > 0 ? "warn" : "neutral"}
          note="Eliminaciones que fallaron y esperan reintento"
        />
      </div>

      <Card
        title="Correos fallidos por tipo"
        subtitle="Mensajes que no pudieron enviarse en el periodo"
        aside={
          notifications.failed > 0 ? (
            <Badge tone="late">Requiere revisión</Badge>
          ) : null
        }
      >
        {notifications.failedByType.length === 0 ? (
          <EmptyNote>No hubo correos fallidos en este periodo.</EmptyNote>
        ) : (
          <RankList
            empty=""
            rows={notifications.failedByType.map((f) => ({
              key: f.type,
              label: labelFor(notificationTypeLabels, f.type),
              value: f.count,
              display: formatCount(f.count),
              color: colors.overdue,
            }))}
          />
        )}
      </Card>
    </div>
  );
}
