import { Icon } from "@endosoflow/ui";
import type { ApiSale } from "./api";

const taskStateText = {
  pending: "por iniciar",
  in_progress: "en progreso",
  completed: "completada",
} as const;

/**
 * The sale as a route: six stages, thirteen tasks. What is done is filled, the task in hand is ringed, and any
 * task can be opened from here. This is the one picture of where a sale stands.
 */
export function Route({
  sale,
  selected,
  onSelect,
}: {
  sale: ApiSale;
  selected: number | null;
  onSelect: (number: number) => void;
}) {
  const current = sale.currentTask?.number ?? null;
  const done = sale.tasks.filter((task) => task.status === "completed").length;
  return (
    <section className="route-card" aria-label="Ruta de la venta">
      <header className="route-head">
        <h2>Ruta de la operación</h2>
        <span>
          <strong>{done}</strong> de 13 tareas completadas
        </span>
      </header>
      <ol className="route">
        {sale.milestones.map((milestone) => {
          const tasks = sale.tasks.filter(
            (task) => task.milestoneNumber === milestone.number,
          );
          return (
            <li
              key={milestone.number}
              className={`route-stop is-${milestone.state}`}
            >
              <span className="route-name">
                {milestone.state === "completed" && (
                  <Icon name="check" size={14} />
                )}
                {milestone.name}
              </span>
              <span className="route-tasks">
                {tasks.map((task) => {
                  const isCurrent = task.number === current;
                  const late = isCurrent && sale.currentTask?.isOverdue;
                  return (
                    <button
                      key={task.number}
                      className={[
                        "route-dot",
                        `is-${task.status}`,
                        isCurrent ? "is-current" : "",
                        late ? "is-late" : "",
                        selected === task.number ? "is-selected" : "",
                      ].join(" ")}
                      onClick={() => onSelect(task.number)}
                      aria-label={`Tarea ${task.number}: ${task.name}, ${taskStateText[task.status]}${late ? ", con retraso" : ""}`}
                      title={`${task.number}. ${task.name}`}
                    >
                      {task.status === "completed" ? (
                        <Icon name="check" size={12} />
                      ) : (
                        task.number
                      )}
                    </button>
                  );
                })}
              </span>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
