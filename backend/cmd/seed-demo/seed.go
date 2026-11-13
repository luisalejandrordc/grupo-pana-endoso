package main

import (
	"context"
	"encoding/json"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// clock is the time the services see. The seed moves it through each sale's story, so tasks have real
// start, due and completion times in the past (some on time, some late) while the data goes through the same
// services, rules and audit trail as in the application.
type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

type slotInfo struct{ label, format string }

type money struct{ toyota, commission, accessory, release int }

func (m money) quoted() int { return m.toyota + m.commission + m.accessory + m.release }

type seeder struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	clock *clock
	now   time.Time
	limit time.Time // the latest moment a story may reach
	rng   *rand.Rand

	customers *customers.Service
	catalog   *catalog.Service
	sales     *sales.Service
	tasks     *tasks.Service
	docs      *documents.Service

	password string
	admin    auth.Actor
	staff    []auth.Actor // by index in demoEmployees

	slots     map[string]slotInfo
	versionID [][]uuid.UUID
	colorID   [][]uuid.UUID
	price     [][]int
	clients   []*customers.Customer

	invoiceCounter int
	created        struct{ employees, models, customers, sales, tasks, documents int }
}

func (s *seeder) logf(format string, args ...any) { fmt.Printf("  "+format+"\n", args...) }

// move puts the clock at t, never earlier than it already is and never later than the limit.
func (s *seeder) move(t time.Time) {
	if t.After(s.limit) {
		t = s.limit
	}
	if t.After(s.clock.t) {
		s.clock.t = t
	}
}

func (s *seeder) advance(d time.Duration) { s.move(s.clock.t.Add(d)) }

func (s *seeder) hours(min, max float64) time.Duration {
	return time.Duration((min + s.rng.Float64()*(max-min)) * float64(time.Hour))
}

// --- people, catalog, customers ---

func (s *seeder) seedEmployees() error {
	hash, err := auth.HashPassword(s.password)
	if err != nil {
		return err
	}
	for _, e := range demoEmployees {
		id := ids.New()
		_, err := s.pool.Exec(s.ctx, `INSERT INTO employees
			(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
			 business_phone, business_email, role, password_hash, activated_at, created_by)
			VALUES ($1, 'DNI', $2, $3, $4, 'Peruana', $5::text::gender, DATE '1992-05-14', DATE '2024-02-01', $6, $7,
			        $8::text::employee_role, $9, now(), $10)`,
			id, e.docNumber, e.first, e.last, e.gender, e.phone, e.email, e.role, hash, s.admin.ID)
		if err != nil {
			return fmt.Errorf("employee %s: %w", e.email, err)
		}
		role := auth.RoleSalesAdvisor
		if e.role == "sales_manager" {
			role = auth.RoleSalesManager
		}
		s.staff = append(s.staff, auth.Actor{ID: id, Role: role})
		s.created.employees++
	}
	s.logf("%d employees", len(demoEmployees))
	return nil
}

func (s *seeder) seedCatalog() error {
	for _, m := range demoModels {
		first := m.versions[0]
		model, err := s.catalog.CreateModel(s.ctx, s.admin, catalog.CreateModelInput{
			Name: m.name, Description: m.description, Categories: m.categories,
			Version: catalog.VersionInput{Name: first.name, ReferencePrice: catalog.Money{Raw: first.price, Set: true}},
			Color:   catalog.ColorInput{Name: m.colors[0]},
		}, carPNG(bodyColor(m.colors[0])))
		if err != nil {
			return fmt.Errorf("model %s: %w", m.name, err)
		}
		versions := []uuid.UUID{model.Versions[0].ID}
		prices := []int{atoi(first.price)}
		for _, v := range m.versions[1:] {
			created, err := s.catalog.CreateVersion(s.ctx, s.admin, model.ID, catalog.VersionInput{
				Name: v.name, ReferencePrice: catalog.Money{Raw: v.price, Set: true},
			})
			if err != nil {
				return fmt.Errorf("version %s %s: %w", m.name, v.name, err)
			}
			versions = append(versions, created.ID)
			prices = append(prices, atoi(v.price))
		}
		colors := []uuid.UUID{model.Colors[0].ID}
		for _, name := range m.colors[1:] {
			created, err := s.catalog.CreateColor(s.ctx, s.admin, model.ID, catalog.ColorInput{Name: name}, carPNG(bodyColor(name)))
			if err != nil {
				return fmt.Errorf("color %s %s: %w", m.name, name, err)
			}
			colors = append(colors, created.ID)
		}
		s.versionID = append(s.versionID, versions)
		s.colorID = append(s.colorID, colors)
		s.price = append(s.price, prices)
		s.created.models++
	}
	s.logf("%d models with versions, colors and pictures", len(demoModels))
	return nil
}

func (s *seeder) seedCustomers() error {
	creator := s.staff[1]
	for _, c := range demoCustomers {
		created, err := s.customers.Create(s.ctx, creator, c.in)
		if err != nil {
			return fmt.Errorf("customer %s: %w", c.in.DocumentNumber, err)
		}
		s.clients = append(s.clients, created)
		s.created.customers++
	}
	s.logf("%d customers", len(demoCustomers))
	return nil
}

func (s *seeder) loadSlots() error {
	rows, err := s.pool.Query(s.ctx, `SELECT slot_code, label_es, allowed_format::text FROM document_slots`)
	if err != nil {
		return err
	}
	defer rows.Close()
	s.slots = map[string]slotInfo{}
	for rows.Next() {
		var code string
		var info slotInfo
		if err := rows.Scan(&code, &info.label, &info.format); err != nil {
			return err
		}
		s.slots[code] = info
	}
	return rows.Err()
}

func atoi(text string) int {
	var n int
	_, _ = fmt.Sscanf(text, "%d", &n)
	return n
}

// --- one sale ---

func (s *seeder) amounts(price int) money {
	round := func(v float64) int { return int(math.Round(v/50) * 50) }
	m := money{toyota: round(float64(price) * 0.86), commission: round(float64(price) * 0.09), accessory: round(float64(price) * 0.03)}
	m.release = price - m.toyota - m.commission - m.accessory
	if m.release < 300 {
		m.release = 300
	}
	return m
}

func (s *seeder) runSale(index int, sc scenario) error {
	advisor := s.staff[sc.advisor]
	manager := s.staff[managerIndex]
	customer := s.clients[sc.customer]
	start := s.now.Add(-time.Duration(sc.startedDaysAgo * 24 * float64(time.Hour)))
	s.clock.t = start

	in := sales.CreateInput{
		CustomerID: customer.ID, VersionID: s.versionID[sc.model][sc.version], ColorID: s.colorID[sc.model][sc.color],
		Temperature: sc.temperature,
	}
	sale, err := s.sales.Create(s.ctx, advisor, in)
	if err != nil {
		return fmt.Errorf("create sale: %w", err)
	}
	s.created.sales++
	if sc.notes != "" {
		s.advance(s.hours(0.5, 2))
		if _, err := s.sales.UpdateNotes(s.ctx, advisor, sale.ID, sales.UpdateInput{Version: &sale.Version, Notes: &sc.notes}); err != nil {
			return fmt.Errorf("sale notes: %w", err)
		}
	}

	fin := s.amounts(s.price[sc.model][sc.version])
	c := &saleContext{sale: sale, customerName: customer.DisplayName, advisor: advisor, fin: &fin, index: index}

	for n := 1; n <= sc.through; n++ {
		s.advance(s.hours(1, 6))
		if sc.restart == n {
			if _, err := s.tasks.Start(s.ctx, advisor, sale.ID, n); err != nil {
				return fmt.Errorf("task %d start: %w", n, err)
			}
			s.advance(s.hours(0.3, 1))
			if _, err := s.tasks.Restart(s.ctx, manager, sale.ID, n, "La tarea se abrió por error mientras se esperaba la respuesta del cliente."); err != nil {
				return fmt.Errorf("task %d restart: %w", n, err)
			}
			s.created.tasks++
			s.advance(s.hours(2, 5))
		}
		if err := s.completeTask(c, n); err != nil {
			return err
		}
		if n == 1 && sc.correctFirst {
			s.advance(s.hours(1, 3))
			fin.accessory += 200
			if _, err := s.save(manager, sale.ID, 1, map[string]any{"accessoryAmount": fmt.Sprintf("%d", fin.accessory)},
				"El cliente agregó un kit de accesorios después de enviar la cotización."); err != nil {
				return fmt.Errorf("task 1 correction: %w", err)
			}
		}
	}

	if sc.current > 0 {
		s.move(s.now.Add(-sc.currentStartedAgo))
		if err := s.startPartial(c, sc.current, sc.partial); err != nil {
			return err
		}
	}

	if sc.cancel != nil {
		s.advance(s.hours(2, 5))
		actor := advisor
		switch sc.cancel.by {
		case "manager":
			actor = manager
		case "admin":
			actor = s.admin
		}
		if _, err := s.sales.Cancel(s.ctx, actor, sale.ID, sales.CancelInput{Category: sc.cancel.category, Text: sc.cancel.text}); err != nil {
			return fmt.Errorf("cancel: %w", err)
		}
	}
	return nil
}

type saleContext struct {
	sale         *sales.Detail
	customerName string
	advisor      auth.Actor
	fin          *money
	index        int
}

func (s *seeder) save(actor auth.Actor, saleID uuid.UUID, n int, data map[string]any, reason string) (*tasks.SaveResult, error) {
	task, err := s.tasks.Get(s.ctx, actor, saleID, n)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return s.tasks.SaveData(s.ctx, actor, saleID, n, tasks.SaveInput{Version: &task.Version, Reason: reason, Data: raw})
}

// completeTask runs a whole task: start, data, documents, a wait that is early, on time or late, and completion.
func (s *seeder) completeTask(c *saleContext, n int) error {
	started, err := s.tasks.Start(s.ctx, c.advisor, c.sale.ID, n)
	if err != nil {
		return fmt.Errorf("task %d start: %w", n, err)
	}
	if _, err := s.save(c.advisor, c.sale.ID, n, s.dataFor(c, n), ""); err != nil {
		return fmt.Errorf("task %d data: %w", n, err)
	}
	if err := s.uploadRequired(c, n); err != nil {
		return err
	}
	// Mostly early or inside the time limit; now and then late, so the phases and the reminders have something to show.
	if started.DurationHours == nil {
		s.advance(s.hours(1, 4))
	} else {
		factors := []float64{0.3, 0.6, 0.9, 0.5, 1.4, 0.4, 0.75}
		factor := factors[(c.index*3+n)%len(factors)]
		s.advance(time.Duration(float64(*started.DurationHours) * factor * float64(time.Hour)))
	}
	if _, err := s.tasks.Complete(s.ctx, c.advisor, c.sale.ID, n); err != nil {
		return fmt.Errorf("task %d complete: %w", n, err)
	}
	s.created.tasks++
	return nil
}

// startPartial opens a task and, when partial, leaves it with some of its data and its first file.
func (s *seeder) startPartial(c *saleContext, n int, partial bool) error {
	if _, err := s.tasks.Start(s.ctx, c.advisor, c.sale.ID, n); err != nil {
		return fmt.Errorf("task %d start: %w", n, err)
	}
	s.created.tasks++
	if !partial {
		return nil
	}
	data := s.dataFor(c, n)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	if n == 2 {
		keys = []string{"signerDocumentType", "signerDocumentNumber"}
	} else if n == 6 {
		keys = []string{"toyota"}
	} else {
		sortStrings(keys)
		keys = keys[:(len(keys)+1)/2]
	}
	subset := map[string]any{}
	for _, k := range keys {
		subset[k] = data[k]
	}
	if _, err := s.save(c.advisor, c.sale.ID, n, subset, ""); err != nil {
		fmt.Println(subset)
		return fmt.Errorf("task %d partial data: %w", n, err)
	}
	if slots := tasks.RequiredSlots(n, c.sale.CustomerType); len(slots) > 0 {
		if err := s.upload(c, n, slots[0]); err != nil {
			return err
		}
	}
	return nil
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func (s *seeder) uploadRequired(c *saleContext, n int) error {
	for _, slot := range tasks.RequiredSlots(n, c.sale.CustomerType) {
		if err := s.upload(c, n, slot); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) upload(c *saleContext, n int, slot string) error {
	info := s.slots[slot]
	name := slot
	var data []byte
	if info.format == "excel" {
		name += ".xlsx"
		fin := c.fin
		data = simpleXLSX("Cotización", [][]any{
			{"Concepto", "Monto (USD)"},
			{"Unidad (Toyota)", fin.toyota},
			{"Comisión Grupo Pana", fin.commission},
			{"Accesorios", fin.accessory},
			{"Trámite de liberación", fin.release},
			{"Precio final", fin.quoted()},
		})
	} else {
		name += ".pdf"
		data = simplePDF(info.label, []string{
			"Cliente: " + c.customerName,
			"Operación: venta " + strings.ToUpper(c.sale.ID.String()[:8]),
			"Fecha: " + s.clock.Now().Format("2006-01-02"),
			"",
			"Documento de demostración generado por seed-demo.",
		})
	}
	if _, _, err := s.docs.Upload(s.ctx, c.advisor, documents.UploadInput{SaleID: c.sale.ID, Number: n, Slot: slot, FileName: name, Data: data}); err != nil {
		return fmt.Errorf("upload %s: %w", slot, err)
	}
	s.created.documents++
	return nil
}

// --- realistic values ---

const letters = "ABCDEFGHJKLMNPRSTUVWXYZ"

func (s *seeder) pick(chars string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = chars[s.rng.IntN(len(chars))]
	}
	return string(out)
}

func (s *seeder) digits(n int) string { return s.pick("0123456789", n) }

func (s *seeder) invoice(series string) string {
	s.invoiceCounter++
	return fmt.Sprintf("%s-%06d", series, 1000+s.invoiceCounter)
}

func (s *seeder) dataFor(c *saleContext, n int) map[string]any {
	today := s.clock.Now().Format("2006-01-02")
	fin := c.fin
	banks := []string{"bcp", "bbva", "scotiabank", "interbank"}
	switch n {
	case 1:
		return map[string]any{
			"initialToyotaAmount": fmt.Sprintf("%d", fin.toyota), "initialCommission": fmt.Sprintf("%d", fin.commission),
			"accessoryAmount": fmt.Sprintf("%d", fin.accessory), "releaseProcessingAmount": fmt.Sprintf("%d", fin.release),
		}
	case 2:
		first := []string{"Andrea", "Jorge", "Paula", "Ricardo"}[c.index%4]
		last := []string{"Castillo Vera", "Mendoza Rojas", "Núñez Pardo", "Salas Gómez"}[c.index%4]
		return map[string]any{
			"signerDocumentType": "DNI", "signerDocumentNumber": "4" + s.digits(7), "signerFirstName": first,
			"signerLastName": last, "pepStatus": "no_pep", "ddcRegime": "general",
		}
	case 3:
		return map[string]any{
			"vehicleSeries": "MR" + s.pick(letters, 2) + s.digits(7), "vin": "JTF" + s.pick("ABCDEFGHJKLMNPRSTUVWXYZ0123456789", 14),
			"finalToyotaAmount": fmt.Sprintf("%d", fin.toyota),
		}
	case 4:
		return map[string]any{
			"bank": banks[c.index%4], "paymentDate": today, "transactionNumber": "OP" + s.digits(9),
			"depositedAmount": fmt.Sprintf("%d", fin.quoted()),
		}
	case 5:
		return map[string]any{
			"bank": banks[(c.index+1)%4], "paymentDate": today, "transactionNumber": "TR" + s.digits(9),
			"depositedAmount": fmt.Sprintf("%d", fin.toyota),
		}
	case 6:
		inv := func(series string, amount int) map[string]any {
			return map[string]any{"number": s.invoice(series), "date": today, "amount": fmt.Sprintf("%d", amount)}
		}
		return map[string]any{
			"toyota": inv("F001", fin.toyota), "commission": inv("F002", fin.commission),
			"accessories": inv("F003", fin.accessory), "releaseProcessing": inv("F004", fin.release),
		}
	case 8:
		offices := []string{"callao_maritima", "chancay"}
		channels := []string{"green", "orange", "green", "red"}
		return map[string]any{"damNumber": "118-2026-10-" + s.digits(6), "customsOffice": offices[c.index%2], "controlChannel": channels[c.index%4]}
	case 9:
		return map[string]any{"personInCharge": []string{"Jorge Linares", "Patricia Ugarte", "Miguel Cárdenas"}[c.index%3]}
	case 10:
		return map[string]any{"licensePlate": s.pick(letters, 3) + s.digits(3), "registrationDate": today}
	case 11:
		return map[string]any{"transferDate": today}
	case 12:
		return map[string]any{"serviceOrderNumber": "OS-" + s.digits(6), "completionDate": today}
	case 13:
		return map[string]any{"deliveryDate": today}
	}
	return map[string]any{}
}
