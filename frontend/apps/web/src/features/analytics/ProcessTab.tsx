import type { ProcessData, TaskTiming } from "./api";
import {
  Card,
  EmptyNote,
  KpiCard,
  Legend,
  PeriodNote,
  RankList,
  StackBar,
  colors,
} from "./components";
import { channelLabels, labelFor } from "./labels";
import { formatCount, formatDuration, formatPercent } from "./format";

const lateColor = "var(--late-ink)";

function taskName(t: TaskTiming): string {
  return `${t.taskNumber}. ${t.name}`;
}

/** Where time goes: how long each task takes, how long it waits, who misses deadlines and what is redone. */
export function ProcessTab({ data }: { data: ProcessData }) {
  const { tasks, onTimeRate, customs } = data;
  const completed = tasks.reduce((sum, t) => sum + t.completedCount, 0);
  const rework = tasks.reduce((sum, t) => sum + t.reworkCount, 0);
  const timed = tasks.filter((t) => t.completedCount > 0);
  const scaleMax = Math.max(
    ...tasks.map((t) =>
      Math.max(
        (t.workMedianHours ?? 0) + (t.waitMedianHours ?? 0),
        t.recommendedHours ?? 0,
      ),
    ),
    1,
  );
  const withDeadline = tasks.filter((t) => t.deadlines !== null);
  const loaded = tasks.filter((t) => t.currentCount > 0);
  const redone = tasks.filter((t) => t.reworkCount > 0);

  return (
    <div className="stack an-sections">
      <PeriodNote meta={data.meta} />
      <div className="an-kpis">
        <KpiCard
          label="Plazos cumplidos"
          value={formatPercent(onTimeRate.value)}
          tag="Periodo"
          tone={
            onTimeRate.value !== null && onTimeRate.value < 0.8
              ? "warn"
              : "done"
          }
          note={`${formatCount(onTimeRate.numerator)} de ${formatCount(onTimeRate.denominator)} tareas con plazo · antes ${formatPercent(onTimeRate.previous)}`}
          definition="Tareas con plazo recomendado completadas a tiempo (antes de vencer o consumiendo ya el 80 % del plazo) entre todas las tareas con plazo completadas en el periodo."
        />
        <KpiCard
          label="Tareas completadas"
          value={formatCount(completed)}
          tag="Periodo"
          note="Todas las tareas, con o sin plazo"
        />
        <KpiCard
          label="Reprocesos"
          value={formatCount(rework)}
          tag="Periodo"
          tone={rework > 0 ? "warn" : "neutral"}
          note="Reinicios, reaperturas y correcciones"
          definition="Eventos de auditoría que rehacen una tarea ya iniciada o completada: reiniciar, reabrir y corregir."
        />
      </div>

      <Card
        title="Tiempo por tarea"
        subtitle="Mediana de espera antes de iniciar y de trabajo, frente al plazo recomendado"
      >
        {timed.length === 0 ? (
          <EmptyNote>No hay tareas completadas en este periodo.</EmptyNote>
        ) : (
          <>
            <Legend
              items={[
                { label: "Espera antes de iniciar", color: colors.idle },
                { label: "Trabajo", color: colors.onTime },
              ]}
            />
            <ul className="an-time">
              {tasks.map((t) => {
                const wait = t.waitMedianHours ?? 0;
                const work = t.workMedianHours ?? 0;
                return (
                  <li
                    key={t.taskNumber}
                    className={t.completedCount === 0 ? "is-empty" : ""}
                  >
                    <span className="an-time-label">{taskName(t)}</span>
                    <div className="an-time-track">
                      <StackBar
                        max={scaleMax}
                        label={`${taskName(t)}: espera ${formatDuration(t.waitMedianHours)}, trabajo ${formatDuration(t.workMedianHours)}`}
                        segments={[
                          { label: "Espera", value: wait, color: colors.idle },
                          {
                            label: "Trabajo",
                            value: work,
                            color: colors.onTime,
                          },
                        ]}
                      />
                      {t.recommendedHours !== null ? (
                        <span
                          className="an-marker"
                          style={{
                            left: `${(t.recommendedHours / scaleMax) * 100}%`,
                          }}
                          title={`Plazo recomendado: ${t.recommendedHours} h`}
                          aria-hidden="true"
                        />
                      ) : null}
                    </div>
                    <span className="an-time-figures">
                      {t.completedCount === 0 ? (
                        "Sin datos"
                      ) : (
                        <>
                          Trabajo {formatDuration(t.workMedianHours)} · Espera{" "}
                          {formatDuration(t.waitMedianHours)}
                          {t.recommendedHours !== null
                            ? ` · Plazo ${t.recommendedHours} h`
                            : ""}
                          <span className="an-time-n">
                            {" "}
                            ({formatCount(t.completedCount)})
                          </span>
                        </>
                      )}
                    </span>
                  </li>
                );
              })}
            </ul>
            <p className="an-footnote">
              La línea vertical marca el plazo recomendado de la tarea. Entre
              paréntesis, las tareas completadas en el periodo.
            </p>
          </>
        )}
      </Card>

      <div className="an-pair">
        <Card
          title="Cumplimiento de plazos"
          subtitle="Cómo se completaron las tareas que tienen plazo"
        >
          <Legend
            items={[
              { label: "Antes de tiempo", color: colors.done },
              { label: "Ya por vencer", color: colors.approaching },
              { label: "Con retraso", color: colors.overdue },
              { label: "Retraso recurrente", color: lateColor },
            ]}
          />
          <ul className="an-sla">
            {withDeadline.map((t) => {
              const d = t.deadlines;
              if (!d) return null;
              const total =
                d.early + d.inApproaching + d.overdue + d.recurringOverdue;
              return (
                <li
                  key={t.taskNumber}
                  className={total === 0 ? "is-empty" : ""}
                >
                  <span className="an-time-label">{taskName(t)}</span>
                  <div className="an-time-track">
                    <StackBar
                      max={total}
                      label={`${taskName(t)}: ${d.early} antes de tiempo, ${d.inApproaching} ya por vencer, ${d.overdue} con retraso, ${d.recurringOverdue} con retraso recurrente`}
                      segments={[
                        {
                          label: "Antes de tiempo",
                          value: d.early,
                          color: colors.done,
                        },
                        {
                          label: "Ya por vencer",
                          value: d.inApproaching,
                          color: colors.approaching,
                        },
                        {
                          label: "Con retraso",
                          value: d.overdue,
                          color: colors.overdue,
                        },
                        {
                          label: "Retraso recurrente",
                          value: d.recurringOverdue,
                          color: lateColor,
                        },
                      ]}
                    />
                  </div>
                  <span className="an-time-figures">
                    {total === 0
                      ? "Sin datos"
                      : `${formatCount(d.early + d.inApproaching)} de ${formatCount(total)} a tiempo`}
                  </span>
                </li>
              );
            })}
          </ul>
        </Card>

        <Card
          title="Carga actual por tarea"
          subtitle="Ventas activas que están en cada tarea ahora"
        >
          <RankList
            empty="No hay ventas activas con estos filtros."
            rows={loaded.map((t) => ({
              key: String(t.taskNumber),
              label: taskName(t),
              value: t.currentCount,
              display: formatCount(t.currentCount),
              color: t.overdueCount > 0 ? colors.overdue : colors.onTime,
              detail:
                t.overdueCount > 0
                  ? `${formatCount(t.overdueCount)} con retraso`
                  : undefined,
            }))}
          />
        </Card>
      </div>

      <div className="an-pair">
        <Card
          title="Reprocesos por tarea"
          subtitle="Veces que se reinició, reabrió o corrigió"
        >
          <RankList
            empty="No hubo reprocesos en este periodo."
            rows={redone.map((t) => ({
              key: String(t.taskNumber),
              label: taskName(t),
              value: t.reworkCount,
              display: formatCount(t.reworkCount),
              color: colors.approaching,
            }))}
          />
        </Card>

        <Card
          title="Aduana"
          subtitle="Tarea 8 completada, por canal de control"
        >
          <RankList
            empty="No se completó ninguna gestión aduanera en este periodo."
            rows={customs.map((c) => ({
              key: c.channel,
              label: labelFor(channelLabels, c.channel),
              value: c.count,
              display: `${formatCount(c.count)} · mediana ${formatDuration(c.workMedianHours)}`,
              color:
                c.channel === "green"
                  ? colors.done
                  : c.channel === "red"
                    ? colors.overdue
                    : colors.approaching,
            }))}
          />
        </Card>
      </div>
    </div>
  );
}
