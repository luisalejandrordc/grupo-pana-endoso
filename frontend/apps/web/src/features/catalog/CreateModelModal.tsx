import { useState, type FormEvent } from "react";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { Modal } from "../../shared/ui/Modal";
import type { Toast } from "../../shared/ui/toast";
import { catalogApi, type ApiModel } from "./api";
import {
  createErrorsFrom,
  emptyModelForm,
  toCreateModelData,
  validateCreateModel,
  type CreateModelErrors,
  type CreateModelForm,
} from "./form";
import { imageProblem } from "./images";
import { ImagePicker } from "./ImagePicker";
import { categories, categoryLabels, type VehicleCategory } from "./labels";
import { useCatalogMutation } from "./queries";

/** Creates a model with its first version, first color and that color's Main view image, all at once. */
export function CreateModelModal({
  onClose,
  onCreated,
  toast,
}: {
  onClose: () => void;
  onCreated: (model: ApiModel) => void;
  toast: (toast: Toast) => void;
}) {
  const [form, setForm] = useState<CreateModelForm>(emptyModelForm());
  const [image, setImage] = useState<File | null>(null);
  const [errors, setErrors] = useState<CreateModelErrors>({});
  const [banner, setBanner] = useState("");
  const create = useCatalogMutation(
    (input: { form: CreateModelForm; image: File }) =>
      catalogApi.create(toCreateModelData(input.form), input.image),
  );

  function set<K extends keyof CreateModelForm>(
    key: K,
    value: CreateModelForm[K],
  ) {
    setForm((old) => ({ ...old, [key]: value }));
    setErrors((old) => (old[key] ? { ...old, [key]: undefined } : old));
  }

  function toggleCategory(category: VehicleCategory) {
    set(
      "categories",
      form.categories.includes(category)
        ? form.categories.filter((c) => c !== category)
        : [...form.categories, category],
    );
  }

  function pickImage(file: File | null) {
    const problem = file ? imageProblem(file) : null;
    if (problem) {
      setImage(null);
      setErrors((old) => ({ ...old, image: problem }));
      return;
    }
    setImage(file);
    setErrors((old) => ({ ...old, image: undefined }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBanner("");
    const problems = validateCreateModel(form, image !== null);
    if (Object.keys(problems).length > 0 || !image) {
      setErrors(problems);
      setBanner("Revisa los campos marcados.");
      return;
    }
    try {
      const model = await create.mutateAsync({ form, image });
      toast({
        title: "Modelo creado",
        message: `${model.name} ya está disponible en el catálogo.`,
        tone: "success",
      });
      onCreated(model);
    } catch (error) {
      setErrors(createErrorsFrom(error));
      setBanner(
        error instanceof Error ? error.message : "No se pudo crear el modelo.",
      );
    }
  }

  return (
    <Modal
      title="Nuevo modelo"
      description="Un modelo necesita al menos una versión y un color con su imagen principal."
      wide
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
            form="create-model-form"
            className="button button-primary"
            disabled={create.isPending}
          >
            {create.isPending ? "Creando…" : "Crear modelo"}
          </button>
        </>
      }
    >
      <form
        id="create-model-form"
        className="form-section"
        onSubmit={submit}
        noValidate
      >
        {banner && (
          <div className="form-alert" role="alert">
            <Icon name="alert" size={15} />
            <span>{banner}</span>
          </div>
        )}
        <p className="form-section-title">Modelo</p>
        <Field label="Nombre del modelo" error={errors.name}>
          <input
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            placeholder="Hilux"
          />
        </Field>
        <Field label="Descripción (opcional)" error={errors.description}>
          <textarea
            rows={2}
            value={form.description}
            onChange={(e) => set("description", e.target.value)}
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
                  checked={form.categories.includes(category)}
                  onChange={() => toggleCategory(category)}
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

        <p className="form-section-title">Primera versión</p>
        <div className="form-grid-two">
          <Field label="Nombre de la versión" error={errors.versionName}>
            <input
              value={form.versionName}
              onChange={(e) => set("versionName", e.target.value)}
              placeholder="SR 4x2"
            />
          </Field>
          <Field
            label="Precio de referencia (USD)"
            error={errors.referencePrice}
          >
            <input
              inputMode="decimal"
              value={form.referencePrice}
              onChange={(e) => set("referencePrice", e.target.value)}
              placeholder="35000.00"
            />
          </Field>
        </div>

        <p className="form-section-title">Primer color</p>
        <Field label="Nombre del color" error={errors.colorName}>
          <input
            value={form.colorName}
            onChange={(e) => set("colorName", e.target.value)}
            placeholder="Blanco"
          />
        </Field>
        <ImagePicker
          label="Imagen de la vista principal"
          file={image}
          onPick={pickImage}
          error={errors.image}
        />
      </form>
    </Modal>
  );
}
