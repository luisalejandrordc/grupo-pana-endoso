import { useState } from "react";
import { ApiError } from "@endosoflow/api";
import type { Role } from "../../shared/types";
import { Field, Icon, Modal, type Toast } from "@endosoflow/ui";
import {
  catalogApi,
  type ApiColor,
  type ApiModel,
  type ApiVersion,
} from "./api";
import { ConfirmDialog, ReasonDialog } from "./CatalogDialogs";
import {
  AddColorDialog,
  ColorImagesDialog,
  RenameColorDialog,
  VersionDialog,
} from "./CatalogEntryDialogs";
import { simpleErrorsFrom } from "./form";
import {
  categories,
  categoryLabels,
  formatPrice,
  viewpoints,
  type VehicleCategory,
} from "./labels";
import { useCatalogMutation, useModel } from "./queries";

type Dialog =
  | { kind: "addVersion" }
  | { kind: "editVersion"; version: ApiVersion }
  | { kind: "deleteVersion"; version: ApiVersion }
  | { kind: "addColor" }
  | { kind: "renameColor"; colorId: string }
  | { kind: "images"; colorId: string }
  | { kind: "deleteColor"; color: ApiColor }
  | { kind: "archiveModel" }
  | { kind: "deleteModel" };

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Inténtalo nuevamente.";
}

/** The full model: edit its data, and manage its versions, colors and images. */
export function ModelDetailModal({
  modelId,
  role,
  onClose,
  toast,
}: {
  modelId: string;
  role: Role;
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const query = useModel(modelId);
  const model = query.data;
  const canManage = role === "system_admin" || role === "sales_manager";
  const isAdmin = role === "system_admin";
  const [dialog, setDialog] = useState<Dialog | null>(null);

  // A quick action (archive, reactivate) that reports its own result.
  const act = useCatalogMutation(
    async (input: {
      run: () => Promise<unknown>;
      title: string;
      message?: string;
    }) => {
      await input.run();
      return input;
    },
  );
  async function quick(
    run: () => Promise<unknown>,
    title: string,
    message?: string,
  ) {
    try {
      await act.mutateAsync({ run, title, message });
      toast({ title, message, tone: "success" });
    } catch (error) {
      toast({
        title: "No se pudo completar la acción",
        message: messageOf(error),
        tone: "error",
      });
    }
  }

  if (query.isPending)
    return (
      <Modal title="Modelo" wide onClose={onClose}>
        <span className="skeleton-line" />
        <span className="skeleton-line" style={{ marginTop: 10 }} />
      </Modal>
    );
  if (query.isError || !model)
    return (
      <Modal title="Modelo" wide onClose={onClose}>
        <div className="empty is-error" role="alert">
          <h3>No se pudo cargar el modelo</h3>
          <p>{messageOf(query.error)}</p>
          <button
            className="button button-secondary"
            onClick={() => query.refetch()}
          >
            <Icon name="refresh" size={15} /> Reintentar
          </button>
        </div>
      </Modal>
    );

  const colorById = (id: string) => model.colors.find((c) => c.id === id);

  return (
    <>
      <Modal
        title={model.name}
        description={
          model.isActive
            ? "Modelo activo"
            : "Modelo archivado: no se ofrece en nuevas ventas"
        }
        wide
        onClose={onClose}
        footer={
          <button className="button button-secondary" onClick={onClose}>
            Cerrar
          </button>
        }
      >
        <InfoSection
          model={model}
          canManage={canManage}
          refetch={() => query.refetch()}
          toast={toast}
        />

        <section className="detail-section" aria-label="Versiones">
          <div className="detail-section-heading">
            <h3>Versiones ({model.versions.length})</h3>
            {canManage && (
              <button
                className="button button-secondary button-sm"
                onClick={() => setDialog({ kind: "addVersion" })}
              >
                <Icon name="plus" size={14} /> Agregar versión
              </button>
            )}
          </div>
          {model.versions.map((version) => (
            <div
              key={version.id}
              className={`version-row ${version.isActive ? "" : "is-archived"}`.trim()}
            >
              <span>
                <strong>{version.name}</strong>
                {!version.isActive && <span className="chip"> Archivada</span>}
              </span>
              <strong>{formatPrice(version.referencePrice)}</strong>
              {canManage ? (
                <span className="row-actions">
                  <button
                    className="button button-secondary button-sm"
                    onClick={() => setDialog({ kind: "editVersion", version })}
                    aria-label={`Editar versión ${version.name}`}
                  >
                    Editar
                  </button>
                  <button
                    className="button button-secondary button-sm"
                    onClick={() =>
                      quick(
                        () =>
                          version.isActive
                            ? catalogApi.archiveVersion(version.id)
                            : catalogApi.unarchiveVersion(version.id),
                        version.isActive
                          ? "Versión archivada"
                          : "Versión reactivada",
                        version.name,
                      )
                    }
                    aria-label={`${version.isActive ? "Archivar" : "Reactivar"} versión ${version.name}`}
                  >
                    {version.isActive ? "Archivar" : "Reactivar"}
                  </button>
                  {isAdmin && (
                    <button
                      className="button button-danger button-sm"
                      onClick={() =>
                        setDialog({ kind: "deleteVersion", version })
                      }
                      aria-label={`Eliminar versión ${version.name}`}
                    >
                      Eliminar
                    </button>
                  )}
                </span>
              ) : (
                <span />
              )}
            </div>
          ))}
        </section>

        <section className="detail-section" aria-label="Colores">
          <div className="detail-section-heading">
            <h3>Colores ({model.colors.length})</h3>
            {canManage && (
              <button
                className="button button-secondary button-sm"
                onClick={() => setDialog({ kind: "addColor" })}
              >
                <Icon name="plus" size={14} /> Agregar color
              </button>
            )}
          </div>
          <div className="color-grid">
            {model.colors.map((color) => (
              <article
                key={color.id}
                className={`color-card ${color.isActive ? "" : "is-archived"}`.trim()}
              >
                <img
                  src={color.mainImageUrl}
                  alt={`${color.name}, vista principal`}
                />
                <div className="color-card-body">
                  <strong>
                    {color.name}
                    {!color.isActive && <span className="chip">Archivado</span>}
                  </strong>
                  <span>
                    {color.images.length} de {viewpoints.length} imágenes
                  </span>
                  <span className="row-actions">
                    <button
                      className="button button-secondary button-sm"
                      onClick={() =>
                        setDialog({ kind: "images", colorId: color.id })
                      }
                      aria-label={`${canManage ? "Gestionar" : "Ver"} imágenes de ${color.name}`}
                    >
                      {canManage ? "Imágenes" : "Ver imágenes"}
                    </button>
                    {canManage && (
                      <>
                        <button
                          className="button button-secondary button-sm"
                          onClick={() =>
                            setDialog({
                              kind: "renameColor",
                              colorId: color.id,
                            })
                          }
                          aria-label={`Renombrar color ${color.name}`}
                        >
                          Renombrar
                        </button>
                        <button
                          className="button button-secondary button-sm"
                          onClick={() =>
                            quick(
                              () =>
                                color.isActive
                                  ? catalogApi.archiveColor(color.id)
                                  : catalogApi.unarchiveColor(color.id),
                              color.isActive
                                ? "Color archivado"
                                : "Color reactivado",
                              color.name,
                            )
                          }
                          aria-label={`${color.isActive ? "Archivar" : "Reactivar"} color ${color.name}`}
                        >
                          {color.isActive ? "Archivar" : "Reactivar"}
                        </button>
                        {isAdmin && (
                          <button
                            className="button button-danger button-sm"
                            onClick={() =>
                              setDialog({ kind: "deleteColor", color })
                            }
                            aria-label={`Eliminar color ${color.name}`}
                          >
                            Eliminar
                          </button>
                        )}
                      </>
                    )}
                  </span>
                </div>
              </article>
            ))}
          </div>
        </section>

        {canManage && (
          <div className="admin-zone">
            <div className="admin-zone-row">
              <div>
                <strong>
                  {model.isActive ? "Archivar modelo" : "Reactivar modelo"}
                </strong>
                {model.isActive
                  ? "Deja de ofrecerse en nuevas ventas. Las ventas existentes no cambian."
                  : "Vuelve a ofrecerse en nuevas ventas."}
              </div>
              <button
                className={`button button-sm ${model.isActive ? "button-secondary" : "button-primary"}`}
                onClick={() =>
                  model.isActive
                    ? setDialog({ kind: "archiveModel" })
                    : void quick(
                        () => catalogApi.unarchive(model.id),
                        "Modelo reactivado",
                        model.name,
                      )
                }
              >
                {model.isActive ? "Archivar" : "Reactivar"}
              </button>
            </div>
            {isAdmin && (
              <div className="admin-zone-row">
                <div>
                  <strong>Eliminar modelo</strong>
                  Permanente. Solo si ninguna venta lo usa.
                </div>
                <button
                  className="button button-danger button-sm"
                  onClick={() => setDialog({ kind: "deleteModel" })}
                >
                  Eliminar…
                </button>
              </div>
            )}
          </div>
        )}
      </Modal>

      {dialog?.kind === "addVersion" && (
        <VersionDialog
          modelId={model.id}
          toast={toast}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "editVersion" && (
        <VersionDialog
          modelId={model.id}
          version={dialog.version}
          toast={toast}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "addColor" && (
        <AddColorDialog
          modelId={model.id}
          toast={toast}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "renameColor" && colorById(dialog.colorId) && (
        <RenameColorDialog
          color={colorById(dialog.colorId)!}
          toast={toast}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "images" && colorById(dialog.colorId) && (
        <ColorImagesDialog
          color={colorById(dialog.colorId)!}
          canEdit={canManage}
          toast={toast}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "archiveModel" && (
        <ConfirmDialog
          title="Archivar modelo"
          description={model.name}
          confirmLabel="Archivar"
          onClose={() => setDialog(null)}
          onConfirm={async () => {
            await act.mutateAsync({
              run: () => catalogApi.archive(model.id),
              title: "Modelo archivado",
            });
            toast({
              title: "Modelo archivado",
              message: "Ya no se ofrece en nuevas ventas.",
              tone: "success",
            });
          }}
        >
          <p>
            Un modelo archivado desaparece del formulario de nuevas ventas y de
            las listas de los asesores. Las ventas que ya lo usan no cambian, y
            puedes reactivarlo cuando quieras.
          </p>
        </ConfirmDialog>
      )}
      {dialog?.kind === "deleteVersion" && (
        <ReasonDialog
          title="Eliminar versión"
          description={dialog.version.name}
          warning="Es permanente y solo se permite si ninguna venta usa esta versión. Queda registrada en la auditoría."
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            const version = dialog.version;
            await act.mutateAsync({
              run: () => catalogApi.removeVersion(version.id, reason),
              title: "Versión eliminada",
            });
            toast({
              title: "Versión eliminada",
              message: version.name,
              tone: "success",
            });
          }}
        />
      )}
      {dialog?.kind === "deleteColor" && (
        <ReasonDialog
          title="Eliminar color"
          description={dialog.color.name}
          warning="Es permanente, borra también sus imágenes y solo se permite si ninguna venta usa este color."
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            const color = dialog.color;
            await act.mutateAsync({
              run: () => catalogApi.removeColor(color.id, reason),
              title: "Color eliminado",
            });
            toast({
              title: "Color eliminado",
              message: color.name,
              tone: "success",
            });
          }}
        />
      )}
      {dialog?.kind === "deleteModel" && (
        <ReasonDialog
          title="Eliminar modelo"
          description={model.name}
          warning="Es permanente: borra sus versiones, colores e imágenes, y solo se permite si ninguna venta usa el modelo."
          onClose={() => setDialog(null)}
          onConfirm={async (reason) => {
            await act.mutateAsync({
              run: () => catalogApi.remove(model.id, reason),
              title: "Modelo eliminado",
            });
            toast({
              title: "Modelo eliminado",
              message: model.name,
              tone: "success",
            });
            onClose();
          }}
        />
      )}
    </>
  );
}

// ---- the model's own data ----

function InfoSection({
  model,
  canManage,
  refetch,
  toast,
}: {
  model: ApiModel;
  canManage: boolean;
  refetch: () => void;
  toast: (toast: Toast) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(model.name);
  const [description, setDescription] = useState(model.description ?? "");
  const [cats, setCats] = useState<VehicleCategory[]>(model.categories);
  const [errors, setErrors] = useState<
    Partial<Record<"name" | "description" | "categories", string>>
  >({});
  const [banner, setBanner] = useState("");
  const [stale, setStale] = useState(false);
  const save = useCatalogMutation(
    (body: Parameters<typeof catalogApi.update>[1]) =>
      catalogApi.update(model.id, body),
  );

  function start() {
    setName(model.name);
    setDescription(model.description ?? "");
    setCats(model.categories);
    setErrors({});
    setBanner("");
    setStale(false);
    setEditing(true);
  }

  const body = {
    version: model.version,
    ...(name.trim() !== model.name ? { name: name.trim() } : {}),
    ...(description.trim() !== (model.description ?? "")
      ? { description: description.trim() }
      : {}),
    ...(cats.slice().sort().join() !== model.categories.slice().sort().join()
      ? { categories: cats }
      : {}),
  };
  const changed = Object.keys(body).length > 1;

  async function submit() {
    setBanner("");
    const problems: typeof errors = {};
    if (!name.trim()) problems.name = "El nombre del modelo es obligatorio.";
    if (cats.length === 0)
      problems.categories = "Elige al menos una categoría.";
    if (Object.keys(problems).length > 0) {
      setErrors(problems);
      return;
    }
    try {
      await save.mutateAsync(body);
      toast({
        title: "Modelo actualizado",
        message: name.trim(),
        tone: "success",
      });
      setEditing(false);
    } catch (error) {
      setErrors(
        simpleErrorsFrom(error, ["name", "description", "categories"] as const),
      );
      if (error instanceof ApiError && error.code === "VERSION_CONFLICT") {
        setStale(true);
        setBanner("Otra persona modificó este modelo mientras lo editabas.");
      } else setBanner(messageOf(error));
    }
  }

  if (!editing)
    return (
      <section className="detail-section" aria-label="Datos del modelo">
        <div className="detail-section-heading">
          <h3>Datos del modelo</h3>
          {canManage && (
            <button
              className="button button-secondary button-sm"
              onClick={start}
            >
              <Icon name="edit" size={14} /> Editar
            </button>
          )}
        </div>
        <p>{model.description || "Sin descripción."}</p>
        <div className="detail-chips">
          {model.categories.map((c) => (
            <span key={c} className="chip">
              {categoryLabels[c]}
            </span>
          ))}
        </div>
      </section>
    );

  return (
    <section className="detail-section" aria-label="Editar datos del modelo">
      <div className="detail-section-heading">
        <h3>Editar datos del modelo</h3>
      </div>
      {banner && (
        <div className="form-alert" role="alert">
          <Icon name="alert" size={15} />
          <span>{banner}</span>
          {stale && (
            <button
              className="button button-secondary button-sm"
              onClick={() => {
                refetch();
                setEditing(false);
              }}
            >
              Cargar los cambios
            </button>
          )}
        </div>
      )}
      <Field label="Nombre del modelo" error={errors.name}>
        <input value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label="Descripción" error={errors.description}>
        <textarea
          rows={2}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>
      <fieldset
        className={`category-picker ${errors.categories ? "has-error" : ""}`.trim()}
      >
        <legend className="field-label">Categorías</legend>
        <div className="category-options">
          {categories.map((category) => (
            <label key={category} className="category-option">
              <input
                type="checkbox"
                checked={cats.includes(category)}
                onChange={() =>
                  setCats((old) =>
                    old.includes(category)
                      ? old.filter((c) => c !== category)
                      : [...old, category],
                  )
                }
              />
              <span>{categoryLabels[category]}</span>
            </label>
          ))}
        </div>
        {errors.categories && (
          <small className="field-error" role="alert">
            {errors.categories}
          </small>
        )}
      </fieldset>
      <div className="row-actions">
        <button
          className="button button-secondary button-sm"
          onClick={() => setEditing(false)}
        >
          Cancelar
        </button>
        <button
          className="button button-primary button-sm"
          disabled={save.isPending || !changed || stale}
          onClick={submit}
        >
          {save.isPending ? "Guardando…" : "Guardar cambios"}
        </button>
      </div>
    </section>
  );
}
