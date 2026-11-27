import { ChartCard, Freshness, KpiCard, KpiGrid, TabBody } from "./components";
import { RowChart, color } from "./charts";
import { percent } from "./labels";
import { useMarket } from "./queries";
import { customerTypeLabels, type CustomerType } from "../customers/labels";
import { temperatureLabels } from "../sales/labels";
import type { DashboardParams, MixItem } from "./api";

const mixSeries = [
  { key: "active", label: "En curso", color: color.active },
  { key: "completed", label: "Completadas", color: color.completed },
  { key: "cancelled", label: "Canceladas", color: color.cancelled },
];

function MixChart({
  title,
  subtitle,
  items,
  label = (item) => item.label,
}: {
  title: string;
  subtitle?: string;
  items: MixItem[];
  label?: (item: MixItem) => string;
}) {
  const data = items.map((item) => ({
    label: label(item),
    active: item.active,
    completed: item.completed,
    cancelled: item.cancelled,
  }));
  return (
    <ChartCard
      title={title}
      subtitle={subtitle}
      empty={items.length === 0}
      emptyText="No se crearon ventas en este periodo."
    >
      <RowChart
        data={data}
        stacked
        labelWidth={190}
        summary={`${title}: ${items
          .map((i) => `${label(i)}, ${i.total} ventas`)
          .join("; ")}`}
        series={mixSeries}
      />
    </ChartCard>
  );
}

export function MarketTab({ params }: { params: DashboardParams }) {
  const query = useMarket(params, true);
  return (
    <TabBody query={query}>
      {(data) => (
        <>
          <KpiGrid>
            {data.temperatures.map((t) => (
              <KpiCard
                key={t.temperature}
                label={`Temperatura ${temperatureLabels[t.temperature].toLowerCase()}`}
                value={percent(t.closingRate)}
                hint={`${t.total} ventas · ${t.completed} completadas · ${t.cancelled} canceladas · ${t.active} en curso`}
              />
            ))}
          </KpiGrid>
          <p className="dash-note">
            Ventas creadas en el periodo y cómo están hoy. La tasa de cada
            temperatura cuenta solo las ventas ya resueltas (completadas ÷
            completadas + canceladas), por eso puede cambiar mientras las demás
            terminan.
          </p>
          <div className="dash-grid">
            <MixChart
              title="Modelos más vendidos"
              subtitle="Los 8 con más ventas creadas"
              items={data.models}
            />
            <MixChart
              title="Versiones"
              subtitle="Las 8 con más ventas creadas"
              items={data.versions}
            />
            <MixChart
              title="Colores"
              subtitle="Los 8 más pedidos"
              items={data.colors}
            />
            <MixChart
              title="Tipo de cliente"
              items={data.customerTypes}
              label={(item) =>
                customerTypeLabels[item.key as CustomerType] ?? item.label
              }
            />
          </div>
          <Freshness at={data.generatedAt} />
        </>
      )}
    </TabBody>
  );
}
