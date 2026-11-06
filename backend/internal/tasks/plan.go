package tasks

import (
	"encoding/json"
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/money"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// colVal is one column to write. cast is the PostgreSQL type of the parameter, for enums, dates and numerics.
type colVal struct {
	col  string
	val  any
	cast string
}

// write is one statement worth of changes. Sale-level tables (financials, due diligence, fulfillment) have one
// row per sale; payments and invoices have one row per kind and are created on first save.
type write struct {
	table   string
	kindCol string // "payment_kind" or "invoice_kind" for per-kind tables, "" otherwise
	kindVal string
	keyCol  string // "sale_id", or "id" for sale_registers
	cols    []colVal
	changed []string
}

// planner turns one task's input into validated writes. It never touches the database: the service gives it
// the data already saved, and it returns what to write, which fields changed and the dates it saw.
type planner struct {
	problems []apperr.FieldError
	writes   []write
	dates    []DateEntry
	changed  []string
	today    time.Time
}

func (p *planner) fail(field, code, message string) {
	p.problems = append(p.problems, apperr.FieldError{Field: field, Code: code, Message: message})
}

func (p *planner) failed() bool { return len(p.problems) > 0 }

// decode reads the task-specific object, refusing fields the task does not have.
func decode(raw []byte, into any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after the object")
	}
	return nil
}

// --- field helpers: each returns (provided, value) so a nil input leaves the column alone ---

func (p *planner) amount(field string, in *string) (bool, any) {
	if in == nil {
		return false, nil
	}
	if strings.TrimSpace(*in) == "" {
		return true, nil
	}
	cents, ok := money.Parse(*in)
	if !ok || cents <= 0 {
		p.fail(field, "INVALID_AMOUNT", "El monto debe ser mayor que cero, con hasta dos decimales (por ejemplo 1250.50).")
		return false, nil
	}
	return true, money.Format(cents)
}

func (p *planner) text(field, label string, in *string, max int) (bool, any) {
	if in == nil {
		return false, nil
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return true, nil
	}
	if utf8.RuneCountInString(v) > max {
		p.fail(field, "TOO_LONG", label+" es demasiado largo.")
		return false, nil
	}
	return true, v
}

func (p *planner) oneOf(field, label string, in *string, allowed ...string) (bool, any) {
	if in == nil {
		return false, nil
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return true, nil
	}
	for _, a := range allowed {
		if v == a {
			return true, v
		}
	}
	p.fail(field, "INVALID_VALUE", label+" no es válido. Opciones: "+strings.Join(allowed, ", ")+".")
	return false, nil
}

func (p *planner) code(field, label string, in *string, normalize func(string) (string, bool), message string) (bool, any) {
	if in == nil {
		return false, nil
	}
	if strings.TrimSpace(*in) == "" {
		return true, nil
	}
	n, ok := normalize(*in)
	if !ok {
		p.fail(field, "INVALID_FORMAT", label+" "+message)
		return false, nil
	}
	return true, n
}

func (p *planner) date(field, label string, group int, in *string) (bool, any) {
	if in == nil {
		return false, nil
	}
	if strings.TrimSpace(*in) == "" {
		return true, nil
	}
	d, problem, ok := ParseDate(*in, p.today)
	if !ok {
		switch problem {
		case DateTooOld:
			p.fail(field, "INVALID_DATE", label+" no puede ser anterior al año 2000.")
		case DateTooFarAhead:
			p.fail(field, "INVALID_DATE", label+" está demasiado lejos en el futuro: revisa el año.")
		default:
			p.fail(field, "INVALID_DATE", label+" no es una fecha válida (usa AAAA-MM-DD).")
		}
		return false, nil
	}
	p.dates = append(p.dates, DateEntry{Field: field, Group: group, Date: d})
	return true, d
}

// add appends a column when the field was provided.
func (w *write) add(provided bool, field, col string, val any, cast string) {
	if !provided {
		return
	}
	w.cols = append(w.cols, colVal{col: col, val: val, cast: cast})
	w.changed = append(w.changed, field)
}

func (p *planner) commit(w *write) {
	if len(w.cols) > 0 {
		p.writes = append(p.writes, *w)
		p.changed = append(p.changed, w.changed...)
	}
}

// signerDocumentOK checks a person's document number for its type (same formats as customers).
func signerDocumentOK(docType, number string) bool {
	n := normalizeCode(number)
	switch docType {
	case "DNI":
		return len(n) == 8 && digits(n)
	case "CE":
		return len(n) == 9 && alnum(n)
	case "PAS":
		return len(n) >= 6 && len(n) <= 12 && alnum(n)
	case "CDI":
		return len(n) >= 8 && len(n) <= 12 && alnum(n)
	}
	return false
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func alnum(s string) bool {
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return s != ""
}

// plan validates the input for task n against the data already saved (current) and returns the writes. It
// reports every problem at once. Unknown fields are refused.
func (p *planner) plan(n int, raw []byte, current any, financials financialSnapshot) error {
	switch n {
	case 1:
		var in Task1Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_financials", keyCol: "sale_id"}
		for _, f := range []struct {
			name, col string
			in        *string
		}{
			{"initialToyotaAmount", "initial_toyota_amount", in.InitialToyotaAmount},
			{"initialCommission", "initial_commission", in.InitialCommission},
			{"accessoryAmount", "accessory_amount", in.AccessoryAmount},
			{"releaseProcessingAmount", "release_processing_amount", in.ReleaseProcessingAmount},
		} {
			ok, v := p.amount(f.name, f.in)
			w.add(ok, f.name, f.col, v, "numeric")
		}
		p.commit(&w)
	case 2:
		var in Task2Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		cur := current.(*Task2Data)
		w := write{table: "sale_due_diligence", keyCol: "sale_id"}
		okT, vT := p.oneOf("signerDocumentType", "El tipo de documento del firmante", in.SignerDocumentType, "DNI", "CE", "PAS", "CDI")
		w.add(okT, "signerDocumentType", "signer_document_type", vT, "customer_document_type")
		okN, vN := p.code("signerDocumentNumber", "El número de documento del firmante", in.SignerDocumentNumber,
			func(s string) (string, bool) { return normalizeCode(s), normalizeCode(s) != "" }, "es obligatorio.")
		w.add(okN, "signerDocumentNumber", "signer_document_number", vN, "")
		// The type and number must fit together, whichever of them changed.
		docType := valueOr(cur.SignerDocumentType)
		if okT && vT != nil {
			docType = vT.(string)
		} else if okT {
			docType = ""
		}
		docNumber := valueOr(cur.SignerDocumentNumber)
		if okN && vN != nil {
			docNumber = vN.(string)
		} else if okN {
			docNumber = ""
		}
		if (okT || okN) && docType != "" && docNumber != "" && !signerDocumentOK(docType, docNumber) {
			p.fail("signerDocumentNumber", "INVALID_DOCUMENT", "El número de documento del firmante no es válido para su tipo.")
		}
		ok, v := p.text("signerFirstName", "El nombre del firmante", in.SignerFirstName, 80)
		w.add(ok, "signerFirstName", "signer_first_name", v, "")
		ok, v = p.text("signerLastName", "El apellido del firmante", in.SignerLastName, 80)
		w.add(ok, "signerLastName", "signer_last_name", v, "")
		ok, v = p.oneOf("signerNameSource", "El origen del nombre", in.SignerNameSource, "manual", "looked_up")
		w.add(ok, "signerNameSource", "signer_name_source", v, "name_source")
		ok, v = p.oneOf("pepStatus", "La condición PEP", in.PepStatus, "no_pep", "pep", "related_to_pep")
		w.add(ok, "pepStatus", "pep_status", v, "pep_status")
		ok, v = p.oneOf("ddcRegime", "El régimen de debida diligencia", in.DDCRegime, "general", "reinforced")
		w.add(ok, "ddcRegime", "ddc_regime", v, "ddc_regime")
		p.commit(&w)
	case 3:
		var in Task3Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		sale := write{table: "sale_registers", keyCol: "id"}
		ok, v := p.code("vehicleSeries", "El número de serie", in.VehicleSeries, NormalizeSeries, "es obligatorio (máximo 40 caracteres).")
		sale.add(ok, "vehicleSeries", "vehicle_series", v, "")
		ok, v = p.code("vin", "El VIN", in.VIN, NormalizeVIN, "debe tener 17 caracteres alfanuméricos, sin las letras I, O ni Q.")
		sale.add(ok, "vin", "vin", v, "")
		p.commit(&sale)
		fin := write{table: "sale_financials", keyCol: "sale_id"}
		okA, vA := p.amount("finalToyotaAmount", in.FinalToyotaAmount)
		fin.add(okA, "finalToyotaAmount", "final_toyota_amount", vA, "numeric")
		if okA && vA != nil && financials.initialToyota != nil && financials.initialCommission != nil {
			final, _ := money.Parse(vA.(string))
			if *financials.initialToyota+*financials.initialCommission-final <= 0 {
				p.fail("finalToyotaAmount", "FINAL_COMMISSION_NOT_POSITIVE",
					"Con este monto final de Toyota la comisión final no sería positiva. Revisa el monto.")
			}
		}
		p.commit(&fin)
	case 4, 5:
		var in PaymentData
		if err := decode(raw, &in); err != nil {
			return err
		}
		kind, group := "pana", GroupPanaPayment
		if n == 5 {
			kind, group = "toyota", GroupToyotaPayment
		}
		w := write{table: "sale_payments", kindCol: "payment_kind", kindVal: kind, keyCol: "sale_id"}
		ok, v := p.oneOf("bank", "El banco", in.Bank, "bcp", "bbva", "scotiabank", "interbank")
		w.add(ok, "bank", "bank", v, "bank")
		ok, v = p.date("paymentDate", "La fecha del pago", group, in.PaymentDate)
		w.add(ok, "paymentDate", "payment_date", v, "date")
		ok, v = p.code("transactionNumber", "El número de operación", in.TransactionNumber, NormalizeTransaction, "es obligatorio (máximo 40 caracteres).")
		w.add(ok, "transactionNumber", "transaction_number", v, "")
		ok, v = p.amount("depositedAmount", in.DepositedAmount)
		w.add(ok, "depositedAmount", "deposited_amount", v, "numeric")
		ok, v = p.text("mismatchReason", "El motivo", in.MismatchReason, 500)
		w.add(ok, "mismatchReason", "mismatch_reason", v, "")
		p.commit(&w)
	case 6:
		var in Task6Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		for _, k := range []struct {
			kind, name string
			in         *InvoiceData
		}{
			{"toyota", "toyota", in.Toyota},
			{"commission", "commission", in.Commission},
			{"accessories", "accessories", in.Accessories},
			{"release_processing", "releaseProcessing", in.ReleaseProcessing},
		} {
			if k.in == nil {
				continue
			}
			w := write{table: "sale_invoices", kindCol: "invoice_kind", kindVal: k.kind, keyCol: "sale_id"}
			ok, v := p.code(k.name+".number", "El número de factura", k.in.Number, NormalizeInvoiceNumber, "debe tener el formato F001-000123 (4 caracteres, guion y hasta 8 dígitos).")
			w.add(ok, k.name+".number", "invoice_number", v, "")
			ok, v = p.date(k.name+".date", "La fecha de la factura", GroupInvoices, k.in.Date)
			w.add(ok, k.name+".date", "invoice_date", v, "date")
			ok, v = p.amount(k.name+".amount", k.in.Amount)
			w.add(ok, k.name+".amount", "invoice_amount", v, "numeric")
			ok, v = p.text(k.name+".mismatchReason", "El motivo", k.in.MismatchReason, 500)
			w.add(ok, k.name+".mismatchReason", "mismatch_reason", v, "")
			p.commit(&w)
		}
	case 7:
		var in Task7Data
		return decode(raw, &in)
	case 8:
		var in Task8Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v := p.code("damNumber", "El número de DAM", in.DAMNumber, NormalizeDAM, "es obligatorio (máximo 40 caracteres).")
		w.add(ok, "damNumber", "dam_number", v, "")
		ok, v = p.oneOf("customsOffice", "La aduana", in.CustomsOffice, "callao_maritima", "chancay")
		w.add(ok, "customsOffice", "customs_office", v, "customs_office")
		ok, v = p.oneOf("controlChannel", "El canal de control", in.ControlChannel, "green", "orange", "red")
		w.add(ok, "controlChannel", "control_channel", v, "control_channel")
		p.commit(&w)
	case 9:
		var in Task9Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v := p.text("personInCharge", "El responsable", in.PersonInCharge, 120)
		w.add(ok, "personInCharge", "release_person_in_charge", v, "")
		p.commit(&w)
	case 10:
		var in Task10Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		sale := write{table: "sale_registers", keyCol: "id"}
		ok, v := p.code("licensePlate", "La placa", in.LicensePlate, NormalizePlate, "debe tener 6 caracteres alfanuméricos.")
		sale.add(ok, "licensePlate", "license_plate", v, "")
		p.commit(&sale)
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v = p.date("registrationDate", "La fecha de inscripción", GroupRegistration, in.RegistrationDate)
		w.add(ok, "registrationDate", "registration_date", v, "date")
		p.commit(&w)
	case 11:
		var in Task11Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v := p.date("pdsTransferDate", "La fecha de traslado al PDS", GroupPDSTransfer, in.PDSTransferDate)
		w.add(ok, "pdsTransferDate", "pds_transfer_date", v, "date")
		p.commit(&w)
	case 12:
		var in Task12Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v := p.text("serviceOrderNumber", "El número de orden de servicio", in.ServiceOrderNumber, 40)
		w.add(ok, "serviceOrderNumber", "equipment_service_order_number", v, "")
		ok, v = p.date("equipmentCompletionDate", "La fecha de finalización del equipamiento", GroupEquipment, in.EquipmentCompletionDate)
		w.add(ok, "equipmentCompletionDate", "equipment_completion_date", v, "date")
		p.commit(&w)
	case 13:
		var in Task13Data
		if err := decode(raw, &in); err != nil {
			return err
		}
		w := write{table: "sale_fulfillment", keyCol: "sale_id"}
		ok, v := p.date("surquilloTransferDate", "La fecha de traslado a Surquillo", GroupSurquillo, in.SurquilloTransferDate)
		w.add(ok, "surquilloTransferDate", "surquillo_transfer_date", v, "date")
		ok, v = p.date("deliveryDate", "La fecha de entrega", GroupDelivery, in.DeliveryDate)
		w.add(ok, "deliveryDate", "vehicle_delivery_date", v, "date")
		p.commit(&w)
	default:
		return fmt.Errorf("task %d does not exist", n)
	}
	return nil
}

func valueOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// financialSnapshot is what task 3's validation needs from task 1, in cents (nil when not saved).
type financialSnapshot struct {
	initialToyota     *int64
	initialCommission *int64
}
