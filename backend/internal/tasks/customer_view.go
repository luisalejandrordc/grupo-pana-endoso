package tasks

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// What a customer may read about their purchase.
//
// Customers see only the fields marked customer-visible, and only from COMPLETED tasks. A task that is reopened
// or restarted stops being completed and disappears from the portal at once.
//
// Visibility is decided here, by an allowlist, and is default-deny: a field exists for customers only if it is
// written in customerFields below. A new field added to a task's data is therefore hidden until someone
// deliberately shows it, and customer_view_test.go fails until the new field is classified.

// CustomerField is one value a customer may read.
type CustomerField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Kind tells the screen how to show Value: "text", "date" (YYYY-MM-DD) or "money" (a decimal string in USD).
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// CustomerSection is what a customer reads about one completed task.
type CustomerSection struct {
	TaskNumber      int             `json:"taskNumber"`
	MilestoneNumber int             `json:"milestoneNumber"`
	Title           string          `json:"title"`
	CompletedAt     time.Time       `json:"completedAt"`
	Fields          []CustomerField `json:"fields"`
}

// CustomerView returns one section for every completed task of a sale, with only its customer-visible fields. A
// section can have no fields (the task's visible content may be only documents, or nothing at all): the caller
// decides what to show.
func CustomerView(ctx context.Context, q Querier, saleID uuid.UUID) ([]CustomerSection, error) {
	var ref SaleRef
	if err := q.QueryRow(ctx, `SELECT id, vehicle_series, vin, license_plate FROM sale_registers WHERE id = $1`, saleID).
		Scan(&ref.ID, &ref.Series, &ref.VIN, &ref.Plate); err != nil {
		return nil, err
	}
	f, err := loadFin(ctx, q, saleID)
	if err != nil {
		return nil, err
	}

	type completed struct {
		number, milestone int
		title             string
		at                time.Time
	}
	rows, err := q.Query(ctx, `SELECT t.task_number, d.milestone_number, d.name_es, t.completed_at
		FROM task_instances t JOIN task_definitions d ON d.task_number = t.task_number
		WHERE t.sale_id = $1 AND t.status = 'completed' AND t.completed_at IS NOT NULL
		ORDER BY t.task_number`, saleID)
	if err != nil {
		return nil, err
	}
	var done []completed
	for rows.Next() {
		var c completed
		if err := rows.Scan(&c.number, &c.milestone, &c.title, &c.at); err != nil {
			rows.Close()
			return nil, err
		}
		done = append(done, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]CustomerSection, 0, len(done))
	for _, c := range done {
		if !showsDataToCustomers[c.number] { // nothing of this task's data is ever shown: do not even read it
			out = append(out, CustomerSection{TaskNumber: c.number, MilestoneNumber: c.milestone, Title: c.title, CompletedAt: c.at, Fields: []CustomerField{}})
			continue
		}
		data, err := loadData(ctx, q, &ref, c.number)
		if err != nil {
			return nil, err
		}
		out = append(out, CustomerSection{
			TaskNumber: c.number, MilestoneNumber: c.milestone, Title: c.title, CompletedAt: c.at,
			Fields: customerFields(c.number, data, f),
		})
	}
	return out, nil
}

// showsDataToCustomers says which tasks have any customer-visible data. The tests keep it equal to what
// customerFields can return.
var showsDataToCustomers = map[int]bool{1: true, 2: true, 3: true, 4: true, 6: true, 10: true, 11: true, 12: true, 13: true}

// fieldList collects the fields of a section, leaving out any that has no value yet.
type fieldList []CustomerField

func (l *fieldList) add(key, label, kind string, value *string) {
	if value != nil && *value != "" {
		*l = append(*l, CustomerField{Key: key, Label: label, Kind: kind, Value: *value})
	}
}

// addCalculated adds a value the system computes (a sum, a commission), already formatted as money.
func (l *fieldList) addCalculated(key, label string, value any) {
	if text, ok := value.(string); ok {
		l.add(key, label, "money", &text)
	}
}

// customerFields is the allowlist: the only place that decides what a customer sees of a task's data. Nothing
// else about a task reaches the portal. Internal-only fields (the PEP and DDC classification, Toyota's payment,
// customs, the person in charge, service orders, the transfer to Surquillo, the reasons given for mismatching
// amounts) are deliberately absent.
func customerFields(task int, data any, f fin) []CustomerField {
	var l fieldList
	switch task {
	case 1:
		d := data.(*Task1Data)
		l.add("initialToyotaAmount", "Monto inicial a Toyota", "money", d.InitialToyotaAmount)
		l.add("initialCommission", "Comisión inicial de Grupo Pana", "money", d.InitialCommission)
		l.add("accessoryAmount", "Monto de accesorios", "money", d.AccessoryAmount)
		l.add("releaseProcessingAmount", "Trámite de liberación del vehículo", "money", d.ReleaseProcessingAmount)
		l.addCalculated("quotedFinalPrice", "Precio final cotizado", str(f.quoted))
	case 2:
		d := data.(*Task2Data)
		l.add("signerDocumentType", "Tipo de documento del firmante", "text", d.SignerDocumentType)
		l.add("signerDocumentNumber", "Número de documento del firmante", "text", d.SignerDocumentNumber)
		l.add("signerFirstName", "Nombres del firmante", "text", d.SignerFirstName)
		l.add("signerLastName", "Apellidos del firmante", "text", d.SignerLastName)
	case 3:
		d := data.(*Task3Data)
		l.add("vehicleSeries", "Número de serie", "text", d.VehicleSeries)
		l.add("vin", "VIN", "text", d.VIN)
		l.add("finalToyotaAmount", "Monto final a Toyota", "money", d.FinalToyotaAmount)
		l.addCalculated("finalCommission", "Comisión final de Grupo Pana", str(f.finalCommission))
	case 4:
		d := data.(*PaymentData)
		l.add("bank", "Banco", "text", d.Bank)
		l.add("paymentDate", "Fecha de la transacción", "date", d.PaymentDate)
		l.add("transactionNumber", "Número de operación", "text", d.TransactionNumber)
		l.add("depositedAmount", "Monto depositado", "money", d.DepositedAmount)
	case 6:
		d := data.(*Task6Data)
		for _, invoice := range []struct {
			key, label string
			data       *InvoiceData
		}{
			{"toyota", "Factura de Toyota", d.Toyota},
			{"commission", "Factura de comisión", d.Commission},
			{"accessories", "Factura de accesorios", d.Accessories},
			{"releaseProcessing", "Factura de trámite de liberación", d.ReleaseProcessing},
		} {
			if invoice.data == nil {
				continue
			}
			l.add(invoice.key+".number", invoice.label+" · Número", "text", invoice.data.Number)
			l.add(invoice.key+".date", invoice.label+" · Fecha", "date", invoice.data.Date)
			l.add(invoice.key+".amount", invoice.label+" · Monto", "money", invoice.data.Amount)
		}
	case 10:
		d := data.(*Task10Data)
		l.add("licensePlate", "Placa", "text", d.LicensePlate)
		l.add("registrationDate", "Fecha de inmatriculación", "date", d.RegistrationDate)
	case 11:
		d := data.(*Task11Data)
		l.add("pdsTransferDate", "Fecha de traslado a preparación final", "date", d.PDSTransferDate)
	case 12:
		d := data.(*Task12Data)
		l.add("equipmentCompletionDate", "Fecha de término del equipamiento", "date", d.EquipmentCompletionDate)
	case 13:
		d := data.(*Task13Data)
		l.add("deliveryDate", "Fecha de entrega", "date", d.DeliveryDate)
	}
	return l
}
