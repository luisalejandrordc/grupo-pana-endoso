import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { customersApi, type CustomerLookupResult } from "../customers/api";
import { CustomerFormModal } from "../customers/CustomerFormModal";
import {
  customerTypeLabels,
  documentTypeLabels,
  type CustomerDocumentType,
} from "../customers/labels";
import { useCustomerList } from "../customers/queries";
import { useDebounced } from "../../shared/lib/useDebounced";
import { formatMoneyText } from "../../shared/lib/format";
import type { Role } from "../../shared/types";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import type { Toast } from "../../shared/ui/toast";
import { salesApi, type CreateSaleBody, type Temperature } from "./api";
import { errorText, fieldErrors, temperatureLabels } from "./labels";
import { useAdvisors, useSaleMutation, useSelectableCatalog } from "./queries";

interface Picked {
  id: string;
  displayName: string;
  customerType: string;
  detail: string;
  isActive: boolean;
}

const lookupTypes: CustomerDocumentType[] = ["DNI", "CE", "PAS", "CDI", "RUC"];

/** Registers a sale: the customer, the vehicle (from what the catalog offers today) and a few details. */
export function NewSalePage({
  role,
  userId,
  toast,
}: {
  role: Role;
  userId: string;
  toast: (toast: Toast) => void;
}) {
  const navigate = useNavigate();
  const onCancel = () => navigate("/sales");
  const management = role !== "sales_advisor";
  const [customer, setCustomer] = useState<Picked | null>(null);
  const [modelId, setModelId] = useState("");
  const [versionId, setVersionId] = useState("");
  const [colorId, setColorId] = useState("");
  const [temperature, setTemperature] = useState<Temperature>("warm");
  const [advisorId, setAdvisorId] = useState(management ? "" : userId);
  const [notes, setNotes] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [newCustomer, setNewCustomer] = useState(false);

  const catalog = useSelectableCatalog();
  const advisors = useAdvisors(management);
  const create = useSaleMutation((body: CreateSaleBody) =>
    salesApi.create(body),
  );

  const model = catalog.data?.find((item) => item.id === modelId);
  const version = model?.versions.find((item) => item.id === versionId);
  const color = model?.colors.find((item) => item.id === colorId);

  const ready =
    customer !== null &&
    version !== undefined &&
    color !== undefined &&
    (!management || advisorId !== "");

  async function submit() {
    if (!customer || !version || !color) return;
    setError(null);
    setErrors({});
    try {
      const sale = await create.mutateAsync({
        customerId: customer.id,
        versionId: version.id,
        colorId: color.id,
        temperature,
        advisorId: management ? advisorId : undefined,
        notes: notes.trim() || undefined,
      });
      toast({
        title: "Venta registrada",
        message: `${sale.code} · ${sale.customer.name} · ${sale.vehicle.model}`,
        tone: "success",
      });
      navigate(`/sales/${sale.id}`);
    } catch (caught) {
      setError(errorText(caught));
      setErrors(fieldErrors(caught));
    }
  }

  return (
    <div className="stack new-sale">
      <div className="crumbs">
        <button className="text-button" onClick={onCancel}>
          <Icon name="arrow-left" size={16} /> Volver a ventas
        </button>
      </div>
      <div className="page-head">
        <div>
          <h1>Registrar una venta</h1>
          <p>
            Elige al cliente y la unidad. Los datos del cliente y del vehículo
            quedan guardados tal como están hoy.
          </p>
        </div>
      </div>

      <section className="panel new-section">
        <h2>1. Cliente</h2>
        {customer ? (
          <div className="picked">
            <div>
              <strong>{customer.displayName}</strong>
              <span>
                {customerTypeLabels[
                  customer.customerType as keyof typeof customerTypeLabels
                ] ?? customer.customerType}
                {customer.detail ? ` · ${customer.detail}` : ""}
              </span>
              {!customer.isActive && (
                <small className="field-error">
                  Este cliente está inactivo y no puede recibir nuevas ventas.
                </small>
              )}
            </div>
            <button
              className="button button-secondary button-sm"
              onClick={() => setCustomer(null)}
            >
              Cambiar
            </button>
          </div>
        ) : (
          <CustomerPicker
            onPick={setCustomer}
            onNew={() => setNewCustomer(true)}
          />
        )}
        {errors.customerId && (
          <small className="field-error">{errors.customerId}</small>
        )}
      </section>

      <section className="panel new-section">
        <h2>2. Vehículo</h2>
        {catalog.isPending && (
          <div className="muted">Cargando el catálogo…</div>
        )}
        {catalog.isError && (
          <div className="callout callout-error" role="alert">
            <Icon name="alert" size={16} /> {errorText(catalog.error)}
          </div>
        )}
        {catalog.data && catalog.data.length === 0 && (
          <p className="muted">
            No hay modelos disponibles. Un administrador debe activar al menos
            un modelo con versión y color.
          </p>
        )}
        {catalog.data && catalog.data.length > 0 && (
          <div className="vehicle-grid">
            <div className="vehicle-fields">
              <Field label="Modelo">
                <select
                  value={modelId}
                  onChange={(event) => {
                    setModelId(event.target.value);
                    setVersionId("");
                    setColorId("");
                  }}
                >
                  <option value="">Selecciona un modelo</option>
                  {catalog.data.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Versión" error={errors.versionId}>
                <select
                  value={versionId}
                  disabled={!model}
                  onChange={(event) => setVersionId(event.target.value)}
                >
                  <option value="">Selecciona una versión</option>
                  {model?.versions.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name} · {formatMoneyText(item.referencePrice)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Color" error={errors.colorId}>
                <select
                  value={colorId}
                  disabled={!model}
                  onChange={(event) => setColorId(event.target.value)}
                >
                  <option value="">Selecciona un color</option>
                  {model?.colors.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <div className="vehicle-preview">
              {color ? (
                <img
                  src={color.mainImageUrl}
                  alt={`${model?.name} ${color.name}`}
                />
              ) : (
                <div className="vehicle-preview-empty">
                  <Icon name="car" size={32} />
                  <span>Elige un color para ver la unidad</span>
                </div>
              )}
              {version && (
                <p>
                  Precio de referencia:{" "}
                  <strong>{formatMoneyText(version.referencePrice)}</strong>
                </p>
              )}
            </div>
          </div>
        )}
      </section>

      <section className="panel new-section">
        <h2>3. Detalles</h2>
        <div className="form-grid">
          <Field label="Prioridad de la venta" error={errors.temperature}>
            <select
              value={temperature}
              onChange={(event) =>
                setTemperature(event.target.value as Temperature)
              }
            >
              {(Object.keys(temperatureLabels) as Temperature[]).map(
                (value) => (
                  <option key={value} value={value}>
                    {temperatureLabels[value]}
                  </option>
                ),
              )}
            </select>
          </Field>
          {management ? (
            <Field label="Asesor responsable" error={errors.advisorId}>
              <select
                value={advisorId}
                onChange={(event) => setAdvisorId(event.target.value)}
                disabled={advisors.isPending}
              >
                <option value="">
                  {advisors.isPending
                    ? "Cargando asesores…"
                    : "Selecciona un asesor"}
                </option>
                {(advisors.data ?? []).map((advisor) => (
                  <option key={advisor.id} value={advisor.id}>
                    {advisor.firstName} {advisor.lastName}
                  </option>
                ))}
              </select>
            </Field>
          ) : (
            <Field
              label="Asesor responsable"
              hint="Las ventas que registras quedan asignadas a ti."
            >
              <div className="readonly-value">Tú</div>
            </Field>
          )}
          <Field label="Notas internas (opcional)" className="span-2">
            <textarea
              rows={3}
              value={notes}
              maxLength={2000}
              onChange={(event) => setNotes(event.target.value)}
            />
          </Field>
        </div>
      </section>

      {error && (
        <div className="callout callout-error" role="alert">
          <Icon name="alert" size={16} /> {error}
        </div>
      )}
      <div className="new-footer">
        <button
          className="button button-secondary"
          onClick={onCancel}
          disabled={create.isPending}
        >
          Cancelar
        </button>
        <button
          className="button button-primary"
          onClick={submit}
          disabled={
            !ready ||
            create.isPending ||
            (customer ? !customer.isActive : false)
          }
        >
          {create.isPending ? "Registrando…" : "Registrar venta"}{" "}
        </button>
      </div>

      {newCustomer && (
        <CustomerFormModal
          mode={{ kind: "create" }}
          role={role}
          toast={toast}
          onClose={() => setNewCustomer(false)}
          onOpenExisting={(existing) => {
            setCustomer({
              id: existing.id,
              displayName: existing.displayName,
              customerType: existing.customerType,
              detail: `${existing.documentType} ${existing.documentNumber}`,
              isActive: existing.isActive,
            });
            setNewCustomer(false);
          }}
          onSaved={(saved, created) => {
            if (!created) return;
            setCustomer({
              id: saved.id,
              displayName: saved.displayName,
              customerType: saved.customerType,
              detail: `${saved.documentType} ${saved.documentNumber}`,
              isActive: saved.isActive,
            });
            setNewCustomer(false);
          }}
        />
      )}
    </div>
  );
}

/** Finds an existing customer by name or document, or by an exact document lookup (which also finds customers of other advisors). */
function CustomerPicker({
  onPick,
  onNew,
}: {
  onPick: (customer: Picked) => void;
  onNew: () => void;
}) {
  const [text, setText] = useState("");
  const term = useDebounced(text.trim());
  const list = useCustomerList({
    q: term,
    status: "all",
    pageSize: 6,
    sort: "displayName",
  });
  const [docType, setDocType] = useState<CustomerDocumentType>("DNI");
  const [docNumber, setDocNumber] = useState("");
  const [lookup, setLookup] = useState<{
    state: "idle" | "busy" | "done" | "failed";
    result?: CustomerLookupResult;
    message?: string;
  }>({
    state: "idle",
  });

  async function runLookup() {
    setLookup({ state: "busy" });
    try {
      const result = await customersApi.lookup(docType, docNumber.trim());
      setLookup({ state: "done", result });
    } catch (caught) {
      setLookup({ state: "failed", message: errorText(caught) });
    }
  }

  const found = lookup.result;
  return (
    <div className="picker">
      <Field label="Buscar cliente por nombre o documento">
        <input
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder="Escribe al menos 2 caracteres…"
        />
      </Field>
      {term.length >= 2 && (
        <div className="picker-results">
          {list.isPending && <span className="muted">Buscando…</span>}
          {list.data?.items.length === 0 && (
            <span className="muted">
              Sin resultados. Prueba con el documento exacto o registra al
              cliente.
            </span>
          )}
          {list.data?.items.map((item) => (
            <button
              key={item.id}
              className="picker-row"
              onClick={() =>
                onPick({
                  id: item.id,
                  displayName: item.displayName,
                  customerType: item.customerType,
                  detail: `${item.documentType} ${item.documentNumber}`,
                  isActive: item.isActive,
                })
              }
            >
              <strong>{item.displayName}</strong>
              <span>
                {customerTypeLabels[item.customerType]} · {item.documentType}{" "}
                {item.documentNumber}
                {!item.isActive ? " · inactivo" : ""}
              </span>
            </button>
          ))}
        </div>
      )}
      <div className="lookup">
        <Field label="¿No aparece? Busca por documento exacto">
          <div className="lookup-row">
            <select
              value={docType}
              onChange={(event) =>
                setDocType(event.target.value as CustomerDocumentType)
              }
            >
              {lookupTypes.map((type) => (
                <option key={type} value={type}>
                  {documentTypeLabels[type]}
                </option>
              ))}
            </select>
            <input
              value={docNumber}
              onChange={(event) => setDocNumber(event.target.value)}
              placeholder="Número de documento"
            />
            <button
              className="button button-secondary"
              onClick={runLookup}
              disabled={docNumber.trim() === "" || lookup.state === "busy"}
            >
              Buscar
            </button>
          </div>
        </Field>
        {lookup.state === "failed" && (
          <small className="field-error">{lookup.message}</small>
        )}
        {found && !found.exists && (
          <small className="muted">
            No existe un cliente con ese documento. Puedes registrarlo abajo.
          </small>
        )}
        {found?.exists && found.customer && (
          <button
            className="picker-row"
            onClick={() =>
              onPick({
                id: found.customer!.id,
                displayName: found.customer!.displayName,
                customerType: found.customer!.customerType,
                detail: `${found.customer!.documentType} ${found.customer!.documentNumber}`,
                isActive: found.customer!.isActive,
              })
            }
          >
            <strong>{found.customer.displayName}</strong>
            <span>Cliente encontrado · usar en esta venta</span>
          </button>
        )}
        {found?.exists && found.brief && (
          <button
            className="picker-row"
            onClick={() =>
              onPick({
                id: found.brief!.id,
                displayName: found.brief!.displayName,
                customerType: found.brief!.customerType,
                detail: `${docType} ${docNumber.trim()}`,
                isActive: found.brief!.isActive,
              })
            }
          >
            <strong>{found.brief.displayName}</strong>
            <span>Cliente de otro asesor · usar en esta venta</span>
          </button>
        )}
      </div>
      <button className="button button-secondary" onClick={onNew}>
        <Icon name="plus" size={16} /> Registrar cliente nuevo
      </button>
    </div>
  );
}
