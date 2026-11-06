package tasks

import (
	"endosoflow/internal/platform/apperr"
	"endosoflow/internal/platform/money"
	"strings"
)

func blank(p *string) bool { return p == nil || strings.TrimSpace(*p) == "" }

func cents(p *string) *int64 {
	if blank(p) {
		return nil
	}
	c, ok := money.Parse(*p)
	if !ok {
		return nil
	}
	return &c
}

func str(c *int64) any {
	if c == nil {
		return nil
	}
	return money.Format(*c)
}

// expectedPayment is the amount a deposit should have: task 4 the quoted final price, task 5 Toyota's final
// amount.
func expectedPayment(n int, f fin) *int64 {
	if n == 4 {
		return f.quoted
	}
	return f.finalToyota
}

// expectedInvoice is the amount an invoice should have, by kind.
func expectedInvoice(kind string, f fin) *int64 {
	switch kind {
	case "toyota":
		return f.finalToyota
	case "commission":
		return f.finalCommission
	case "accessories":
		return f.accessory
	case "release_processing":
		return f.release
	}
	return nil
}

// calculated returns the values the interface shows next to the fields. They are never stored as typed in.
func calculated(n int, data any, f fin) map[string]any {
	out := map[string]any{}
	switch n {
	case 1:
		out["quotedFinalPrice"] = str(f.quoted)
	case 3:
		out["quotedFinalPrice"] = str(f.quoted)
		out["finalCommission"] = str(f.finalCommission)
	case 4, 5:
		d := data.(*PaymentData)
		exp := expectedPayment(n, f)
		out["expectedAmount"] = str(exp)
		out["amountsMatch"] = matches(cents(d.DepositedAmount), exp)
	case 6:
		d := data.(*Task6Data)
		for _, k := range []struct {
			kind, name string
			inv        *InvoiceData
		}{
			{"toyota", "toyota", d.Toyota},
			{"commission", "commission", d.Commission},
			{"accessories", "accessories", d.Accessories},
			{"release_processing", "releaseProcessing", d.ReleaseProcessing},
		} {
			exp := expectedInvoice(k.kind, f)
			out[k.name] = map[string]any{"expectedAmount": str(exp), "amountsMatch": matches(cents(k.inv.Amount), exp)}
		}
	}
	return out
}

// matches is nil while either amount is unknown.
func matches(actual, expected *int64) any {
	if actual == nil || expected == nil {
		return nil
	}
	return *actual == *expected
}

// completionProblems lists everything that stops task n from being completed: missing fields, a missing
// reason for an amount that differs from the expected one, and missing required documents. Every problem
// is reported at once.
func completionProblems(n int, data any, f fin, docs []DocumentSlot) []apperr.FieldError {
	var out []apperr.FieldError
	missing := func(field, label string) {
		out = append(out, apperr.FieldError{Field: field, Code: "REQUIRED", Message: label + " es obligatorio."})
	}
	need := func(p *string, field, label string) {
		if blank(p) {
			missing(field, label)
		}
	}
	reason := func(actual *string, expected *int64, reasonText *string, field string) {
		a := cents(actual)
		if a != nil && expected != nil && *a != *expected && blank(reasonText) {
			out = append(out, apperr.FieldError{
				Field: field, Code: "MISMATCH_REASON_REQUIRED",
				Message: "El monto no coincide con el esperado (" + money.Format(*expected) + "): escribe el motivo.",
			})
		}
	}

	switch n {
	case 1:
		d := data.(*Task1Data)
		need(d.InitialToyotaAmount, "initialToyotaAmount", "El monto inicial de Toyota")
		need(d.InitialCommission, "initialCommission", "La comisión inicial")
		need(d.AccessoryAmount, "accessoryAmount", "El monto de accesorios")
		need(d.ReleaseProcessingAmount, "releaseProcessingAmount", "El monto del trámite de liberación")
	case 2:
		d := data.(*Task2Data)
		need(d.SignerDocumentType, "signerDocumentType", "El tipo de documento del firmante")
		need(d.SignerDocumentNumber, "signerDocumentNumber", "El número de documento del firmante")
		need(d.SignerFirstName, "signerFirstName", "El nombre del firmante")
		need(d.SignerLastName, "signerLastName", "El apellido del firmante")
		need(d.PepStatus, "pepStatus", "La condición PEP")
		need(d.DDCRegime, "ddcRegime", "El régimen de debida diligencia")
	case 3:
		d := data.(*Task3Data)
		need(d.VehicleSeries, "vehicleSeries", "El número de serie")
		need(d.VIN, "vin", "El VIN")
		need(d.FinalToyotaAmount, "finalToyotaAmount", "El monto final de Toyota")
	case 4, 5:
		d := data.(*PaymentData)
		need(d.Bank, "bank", "El banco")
		need(d.PaymentDate, "paymentDate", "La fecha del pago")
		need(d.TransactionNumber, "transactionNumber", "El número de operación")
		need(d.DepositedAmount, "depositedAmount", "El monto depositado")
		reason(d.DepositedAmount, expectedPayment(n, f), d.MismatchReason, "mismatchReason")
	case 6:
		d := data.(*Task6Data)
		for _, k := range []struct {
			kind, name string
			inv        *InvoiceData
		}{
			{"toyota", "toyota", d.Toyota},
			{"commission", "commission", d.Commission},
			{"accessories", "accessories", d.Accessories},
			{"release_processing", "releaseProcessing", d.ReleaseProcessing},
		} {
			need(k.inv.Number, k.name+".number", "El número de factura")
			need(k.inv.Date, k.name+".date", "La fecha de la factura")
			need(k.inv.Amount, k.name+".amount", "El monto de la factura")
			reason(k.inv.Amount, expectedInvoice(k.kind, f), k.inv.MismatchReason, k.name+".mismatchReason")
		}
	case 8:
		d := data.(*Task8Data)
		need(d.DAMNumber, "damNumber", "El número de DAM")
		need(d.CustomsOffice, "customsOffice", "La aduana")
		need(d.ControlChannel, "controlChannel", "El canal de control")
	case 9:
		d := data.(*Task9Data)
		need(d.PersonInCharge, "personInCharge", "El responsable")
	case 10:
		d := data.(*Task10Data)
		need(d.LicensePlate, "licensePlate", "La placa")
		need(d.RegistrationDate, "registrationDate", "La fecha de inscripción")
	case 11:
		d := data.(*Task11Data)
		need(d.PDSTransferDate, "pdsTransferDate", "La fecha de traslado al PDS")
	case 12:
		d := data.(*Task12Data)
		need(d.ServiceOrderNumber, "serviceOrderNumber", "El número de orden de servicio")
		need(d.EquipmentCompletionDate, "equipmentCompletionDate", "La fecha de finalización del equipamiento")
	case 13:
		d := data.(*Task13Data)
		need(d.DeliveryDate, "deliveryDate", "La fecha de entrega")
		// The Surquillo transfer date is optional.
	}

	for _, slot := range docs {
		if slot.Required && slot.File == nil {
			out = append(out, apperr.FieldError{
				Field: "documents." + slot.SlotCode, Code: "DOCUMENT_REQUIRED",
				Message: "Falta el documento: " + slot.Label + ".",
			})
		}
	}
	return out
}

// expectedWrites sets the expected amount on the payments and invoices of a task being completed, so the
// stored row records what the deposit or invoice was compared against.
func expectedWrites(n int, f fin) []write {
	switch n {
	case 4, 5:
		kind := "pana"
		if n == 5 {
			kind = "toyota"
		}
		if exp := expectedPayment(n, f); exp != nil {
			return []write{{
				table: "sale_payments", kindCol: "payment_kind", kindVal: kind, keyCol: "sale_id",
				cols: []colVal{{col: "expected_amount", val: money.Format(*exp), cast: "numeric"}},
			}}
		}
	case 6:
		var out []write
		for _, kind := range []string{"toyota", "commission", "accessories", "release_processing"} {
			if exp := expectedInvoice(kind, f); exp != nil {
				out = append(out, write{
					table: "sale_invoices", kindCol: "invoice_kind", kindVal: kind, keyCol: "sale_id",
					cols: []colVal{{col: "expected_amount", val: money.Format(*exp), cast: "numeric"}},
				})
			}
		}
		return out
	}
	return nil
}
