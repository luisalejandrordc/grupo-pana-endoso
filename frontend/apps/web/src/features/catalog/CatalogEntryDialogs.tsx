import { useState, type FormEvent } from "react";
import { ApiError } from "../../shared/api/client";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { Modal } from "../../shared/ui/Modal";
import type { Toast } from "../../shared/ui/toast";
import { catalogApi, type ApiColor, type ApiVersion } from "./api";
import { normalizePrice, priceProblem, simpleErrorsFrom } from "./form";
import { imageProblem } from "./images";
import { ImagePicker } from "./ImagePicker";
import { viewpointLabels, viewpoints, type Viewpoint } from "./labels";
import { useCatalogMutation } from "./queries";

function messageOf(error: unknown, fallback = "Inténtalo nuevamente.") {
  return error instanceof Error ? error.message : fallback;
}

// ---- add or edit a version ----

type VersionErrors = Partial<Record<"name" | "referencePrice", string>>;

export function VersionDialog({
  modelId,
  version,
  onClose,
  toast,
}: {
  modelId: string;
  /** Present when editing. */
  version?: ApiVersion;
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const [name, setName] = useState(version?.name ?? "");
  const [price, setPrice] = useState(version?.referencePrice ?? "");
  const [errors, setErrors] = useState<VersionErrors>({});
  const [banner, setBanner] = useState("");
  const save = useCatalogMutation(async () => {
    if (!version)
      return catalogApi.createVersion(modelId, {
        name: name.trim(),
        referencePrice: normalizePrice(price),
      });
    const body: { version: number; name?: string; referencePrice?: string } = {
      version: version.version,
    };
    if (name.trim() !== version.name) body.name = name.trim();
    if (normalizePrice(price) !== version.referencePrice)
      body.referencePrice = normalizePrice(price);
    return catalogApi.updateVersion(version.id, body);
  });

  const unchanged =
    version !== undefined &&
    name.trim() === version.name &&
    normalizePrice(price || "0") === version.referencePrice;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner("");
    const problems: VersionErrors = {};
    if (!name.trim()) problems.name = "El nombre de la versión es obligatorio.";
    const priceError = priceProblem(price);
    if (priceError) problems.referencePrice = priceError;
    if (Object.keys(problems).length > 0) {
      setErrors(problems);
      return;
    }
    try {
      await save.mutateAsync(undefined);
      toast({
        title: version ? "Versión actualizada" : "Versión agregada",
        message: name.trim(),
        tone: "success",
      });
      onClose();
    } catch (error) {
      setErrors(simpleErrorsFrom(error, ["name", "referencePrice"] as const));
      setBanner(
        error instanceof ApiError && error.code === "VERSION_CONFLICT"
          ? "Otra persona modificó esta versión. Cierra esta ventana y vuelve a abrirla para ver los cambios."
          : messageOf(error),
      );
    }
  }

  return (
    <Modal
      title={version ? "Editar versión" : "Agregar versión"}
      description={
        version
          ? version.name
          : "Se agrega al modelo y queda disponible para ventas."
      }
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="button button-secondary"
            onClick={onClose}
          >
            Cancelar
          </button>
          <button
            type="submit"
            form="version-form"
            className="button button-primary"
            disabled={save.isPending || unchanged}
          >
            {save.isPending ? "Guardando…" : "Guardar"}
          </button>
        </>
      }
    >
      <form
        id="version-form"
        className="form-section"
        onSubmit={submit}
        noValidate
      >
        {banner && (
          <div className="form-alert" role="alert">
            {banner}
          </div>
        )}
        <Field label="Nombre de la versión" error={errors.name}>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="SR 4x2"
          />
        </Field>
        <Field
          label="Precio de referencia (USD)"
          error={errors.referencePrice}
          hint={
            version
              ? "Las ventas ya creadas conservan el precio con el que se registraron."
              : undefined
          }
        >
          <input
            inputMode="decimal"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            placeholder="35000.00"
          />
        </Field>
      </form>
    </Modal>
  );
}

// ---- add a color, or rename one ----

export function AddColorDialog({
  modelId,
  onClose,
  toast,
}: {
  modelId: string;
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const [name, setName] = useState("");
  const [image, setImage] = useState<File | null>(null);
  const [errors, setErrors] = useState<{ name?: string; image?: string }>({});
  const [banner, setBanner] = useState("");
  const create = useCatalogMutation((input: { name: string; image: File }) =>
    catalogApi.createColor(modelId, input.name, input.image),
  );

  function pick(file: File | null) {
    const problem = file ? imageProblem(file) : null;
    setImage(problem ? null : file);
    setErrors((old) => ({ ...old, image: problem ?? undefined }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner("");
    const problems: { name?: string; image?: string } = {};
    if (!name.trim()) problems.name = "El nombre del color es obligatorio.";
    if (!image)
      problems.image = "La imagen de la vista principal es obligatoria.";
    if (problems.name || problems.image || !image) {
      setErrors((old) => ({ ...old, ...problems }));
      return;
    }
    try {
      await create.mutateAsync({ name: name.trim(), image });
      toast({ title: "Color agregado", message: name.trim(), tone: "success" });
      onClose();
    } catch (error) {
      setErrors(simpleErrorsFrom(error, ["name", "image"] as const));
      setBanner(messageOf(error));
    }
  }

  return (
    <Modal
      title="Agregar color"
      description="Cada color necesita su imagen de la vista principal."
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="button button-secondary"
            onClick={onClose}
          >
            Cancelar
          </button>
          <button
            type="submit"
            form="color-form"
            className="button button-primary"
            disabled={create.isPending}
          >
            {create.isPending ? "Guardando…" : "Agregar color"}
          </button>
        </>
      }
    >
      <form
        id="color-form"
        className="form-section"
        onSubmit={submit}
        noValidate
      >
        {banner && (
          <div className="form-alert" role="alert">
            {banner}
          </div>
        )}
        <Field label="Nombre del color" error={errors.name}>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Blanco perla"
          />
        </Field>
        <ImagePicker
          label="Imagen de la vista principal"
          file={image}
          onPick={pick}
          error={errors.image}
        />
      </form>
    </Modal>
  );
}

export function RenameColorDialog({
  color,
  onClose,
  toast,
}: {
  color: ApiColor;
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const [name, setName] = useState(color.name);
  const [error, setError] = useState("");
  const [banner, setBanner] = useState("");
  const save = useCatalogMutation((value: string) =>
    catalogApi.updateColor(color.id, { version: color.version, name: value }),
  );

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner("");
    if (!name.trim()) {
      setError("El nombre del color es obligatorio.");
      return;
    }
    try {
      await save.mutateAsync(name.trim());
      toast({
        title: "Color actualizado",
        message: name.trim(),
        tone: "success",
      });
      onClose();
    } catch (e) {
      setError(simpleErrorsFrom(e, ["name"] as const).name ?? "");
      setBanner(
        e instanceof ApiError && e.code === "VERSION_CONFLICT"
          ? "Otra persona modificó este color. Cierra esta ventana y vuelve a abrirla para ver los cambios."
          : messageOf(e),
      );
    }
  }

  return (
    <Modal
      title="Renombrar color"
      description={color.name}
      onClose={onClose}
      footer={
        <>
          <button
            type="button"
            className="button button-secondary"
            onClick={onClose}
          >
            Cancelar
          </button>
          <button
            type="submit"
            form="rename-color-form"
            className="button button-primary"
            disabled={save.isPending || name.trim() === color.name}
          >
            {save.isPending ? "Guardando…" : "Guardar"}
          </button>
        </>
      }
    >
      <form
        id="rename-color-form"
        className="form-section"
        onSubmit={submit}
        noValidate
      >
        {banner && (
          <div className="form-alert" role="alert">
            {banner}
          </div>
        )}
        <Field label="Nombre del color" error={error}>
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      </form>
    </Modal>
  );
}

// ---- the six images of a color ----

export function ColorImagesDialog({
  color,
  canEdit,
  onClose,
  toast,
}: {
  color: ApiColor;
  canEdit: boolean;
  onClose: () => void;
  toast: (toast: Toast) => void;
}) {
  const [busy, setBusy] = useState<Viewpoint | null>(null);
  const [banner, setBanner] = useState("");
  const upload = useCatalogMutation(
    (input: { viewpoint: Viewpoint; file: File }) =>
      catalogApi.putImage(color.id, input.viewpoint, input.file),
  );
  const remove = useCatalogMutation((viewpoint: Viewpoint) =>
    catalogApi.deleteImage(color.id, viewpoint),
  );

  async function pick(viewpoint: Viewpoint, file: File | undefined) {
    if (!file) return;
    setBanner("");
    const problem = imageProblem(file);
    if (problem) {
      setBanner(`${viewpointLabels[viewpoint]}: ${problem}`);
      return;
    }
    setBusy(viewpoint);
    try {
      await upload.mutateAsync({ viewpoint, file });
      toast({
        title: "Imagen guardada",
        message: viewpointLabels[viewpoint],
        tone: "success",
      });
    } catch (error) {
      setBanner(`${viewpointLabels[viewpoint]}: ${messageOf(error)}`);
    } finally {
      setBusy(null);
    }
  }

  async function quit(viewpoint: Viewpoint) {
    setBanner("");
    setBusy(viewpoint);
    try {
      await remove.mutateAsync(viewpoint);
      toast({
        title: "Imagen eliminada",
        message: viewpointLabels[viewpoint],
        tone: "success",
      });
    } catch (error) {
      setBanner(`${viewpointLabels[viewpoint]}: ${messageOf(error)}`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <Modal
      title={`Imágenes · ${color.name}`}
      description="Hasta seis vistas por color. La vista principal es obligatoria: se puede reemplazar, no quitar."
      wide
      onClose={onClose}
      footer={
        <button className="button button-secondary" onClick={onClose}>
          Cerrar
        </button>
      }
    >
      {banner && (
        <div className="form-alert" role="alert">
          <Icon name="alert" size={15} />
          <span>{banner}</span>
        </div>
      )}
      <div className="image-slots">
        {viewpoints.map((viewpoint) => {
          const image = color.images.find(
            (item) => item.viewpoint === viewpoint,
          );
          const working = busy === viewpoint;
          return (
            <div className="image-slot" key={viewpoint} aria-busy={working}>
              {image ? (
                <img
                  src={image.url}
                  alt={`${color.name}: ${viewpointLabels[viewpoint]}`}
                />
              ) : (
                <div className="image-slot-empty">Sin imagen</div>
              )}
              <div className="image-slot-footer">
                <strong>{viewpointLabels[viewpoint]}</strong>
                {canEdit && (
                  <span className="row-actions">
                    <label
                      className={`button button-secondary button-sm ${working ? "is-disabled" : ""}`}
                    >
                      <Icon name="upload" size={13} />
                      {working ? "Subiendo…" : image ? "Reemplazar" : "Subir"}
                      <input
                        type="file"
                        hidden
                        accept="image/jpeg,image/png,image/webp"
                        aria-label={`${image ? "Reemplazar" : "Subir"} imagen: ${viewpointLabels[viewpoint]}`}
                        disabled={busy !== null}
                        onChange={(event) => {
                          void pick(viewpoint, event.target.files?.[0]);
                          event.target.value = "";
                        }}
                      />
                    </label>
                    {image && viewpoint !== "main_view" && (
                      <button
                        type="button"
                        className="button button-secondary button-sm"
                        disabled={busy !== null}
                        onClick={() => void quit(viewpoint)}
                        aria-label={`Quitar imagen: ${viewpointLabels[viewpoint]}`}
                      >
                        Quitar
                      </button>
                    )}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </Modal>
  );
}
