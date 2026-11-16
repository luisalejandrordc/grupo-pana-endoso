import { Badge } from "../../shared/ui/Badge";
import type { SaleStateValue, TaskStatusValue, Temperature } from "./api";
import { stateLabels, temperatureLabels } from "./labels";

export function StateBadge({ state }: { state: SaleStateValue }) {
  const tone =
    state === "completed"
      ? "done"
      : state === "cancelled"
        ? "neutral"
        : "doing";
  return <Badge tone={tone}>{stateLabels[state]}</Badge>;
}

export function TempBadge({ temperature }: { temperature: Temperature }) {
  const tone =
    temperature === "hot"
      ? "brand"
      : temperature === "warm"
        ? "warn"
        : "neutral";
  return (
    <Badge tone={tone}>
      Prioridad {temperatureLabels[temperature].toLowerCase()}
    </Badge>
  );
}

const taskStatusLabels: Record<TaskStatusValue, string> = {
  pending: "Por iniciar",
  in_progress: "En progreso",
  completed: "Completada",
};

export function TaskBadge({
  status,
  overdue = false,
}: {
  status: TaskStatusValue;
  overdue?: boolean;
}) {
  if (overdue) return <Badge tone="late">Con retraso</Badge>;
  const tone =
    status === "completed"
      ? "done"
      : status === "in_progress"
        ? "doing"
        : "neutral";
  return <Badge tone={tone}>{taskStatusLabels[status]}</Badge>;
}

/** Thirteen thin segments: how far a sale has come, readable at a glance in a list. */
export function Progress({
  done,
  total = 13,
}: {
  done: number;
  total?: number;
}) {
  return (
    <span
      className="progress"
      role="img"
      aria-label={`${done} de ${total} tareas completadas`}
    >
      <span className="progress-bar">
        {Array.from({ length: total }, (_, index) => (
          <i key={index} className={index < done ? "is-done" : ""} />
        ))}
      </span>
      <span className="progress-count">
        {done}/{total}
      </span>
    </span>
  );
}
