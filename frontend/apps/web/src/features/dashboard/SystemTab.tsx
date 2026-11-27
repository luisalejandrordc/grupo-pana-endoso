import { formatDateTime } from "@endosoflow/ui";
import { ChartCard, Freshness, KpiCard, KpiGrid, TabBody } from "./components";
import { useSystem } from "./queries";
import type { DashboardParams } from "./api";

export function SystemTab({ params }: { params: DashboardParams }) {
  const query = useSystem(params, true);
  return (
    <TabBody query={query}>
      {(data) => {
        const { staff, notifications: n, security: s } = data;
        return (
          <>
            <h2 className="dash-section">Colaboradores</h2>
            <KpiGrid>
              <KpiCard label="Administradores" value={staff.admins} snapshot />
              <KpiCard
                label="Gerentes de ventas"
                value={staff.managers}
                snapshot
              />
              <KpiCard label="Asesores" value={staff.advisors} snapshot />
              <KpiCard
                label="Cuentas inactivas"
                value={staff.inactive}
                snapshot
              />
              <KpiCard
                label="Cuentas bloqueadas"
                value={staff.lockedNow}
                tone={staff.lockedNow > 0 ? "warn" : "neutral"}
                hint="Por intentos fallidos de ingreso"
                snapshot
              />
            </KpiGrid>

            <h2 className="dash-section">Correos</h2>
            <KpiGrid>
              <KpiCard
                label="Pendientes de envío"
                value={n.pendingNow}
                tone={n.pendingNow > 0 ? "warn" : "neutral"}
                hint={
                  n.oldestPendingAt
                    ? `El más antiguo: ${formatDateTime(n.oldestPendingAt)}`
                    : "No hay correos en cola"
                }
                snapshot
              />
              <KpiCard
                label="Enviados"
                value={n.sent}
                hint="Puestos en cola en el periodo"
              />
              <KpiCard
                label="Fallidos"
                value={n.failed}
                tone={n.failed > 0 ? "late" : "neutral"}
                hint="Puestos en cola en el periodo"
              />
              <KpiCard
                label="Omitidos"
                value={n.skipped}
                hint="Puestos en cola en el periodo"
              />
            </KpiGrid>

            <ChartCard
              title="Ingresos de colaboradores"
              subtitle="Eventos de acceso en el periodo"
            >
              <KpiGrid>
                <KpiCard
                  label="Ingresos correctos"
                  value={s.successfulLogins}
                  tone="done"
                />
                <KpiCard
                  label="Ingresos fallidos"
                  value={s.failedLogins}
                  tone={s.failedLogins > 0 ? "warn" : "neutral"}
                  hint={`${s.lockedAttempts} contra cuentas bloqueadas`}
                />
                <KpiCard
                  label="Contraseñas restablecidas"
                  value={s.passwordResets}
                />
              </KpiGrid>
            </ChartCard>
            <Freshness at={data.generatedAt} />
          </>
        );
      }}
    </TabBody>
  );
}
