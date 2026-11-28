// Command seed fills a development database with realistic data: employees of every role, customers, a vehicle
// catalog with images, and sales at every stage (just created, in progress, late, completed, cancelled). It drives
// the real services, so what it creates is exactly what the application would have created.
//
// It refuses to run outside local or test environments, and when the data is already there.
//
//	ENV=local DATABASE_URL=... go run ./cmd/seed
//
// With SEED_HISTORY=1 it also creates about fifty sales spread over the last eleven months, with varied prices
// and outcomes, so the analytics trends have something to show.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/documents"
	"endosoflow/internal/notifications"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const password = "Demo-12345!"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	env := os.Getenv("ENV")
	if env != "local" && env != "test" {
		return fmt.Errorf("refusing to seed: ENV is %q, it must be local or test", env)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM employees WHERE business_email LIKE '%@seed.grupopana.pe'`).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return fmt.Errorf("the seed data is already there (%d employees); reset the database to seed again", existing)
	}

	mediaDir := os.Getenv("LOCAL_MEDIA_DIR")
	if mediaDir == "" {
		mediaDir = "./media"
	}
	mediaURL := os.Getenv("MEDIA_BASE_URL")
	if mediaURL == "" {
		mediaURL = "http://localhost:8080/media"
	}
	store := storage.NewLocal(mediaDir, mediaURL)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := &seeder{ctx: ctx, pool: pool, store: store}
	docs := documents.NewService(documents.Deps{DB: pool, Store: store, Sales: sales.Gate{}, Logger: logger})
	s.docs = docs
	ntsWr := notifications.Writer{}
	s.sales = sales.NewService(sales.Deps{DB: pool, Images: store, Documents: docs, DocumentStore: store, Notifier: ntsWr, Logger: logger})
	s.tasks = tasks.NewService(tasks.Deps{DB: pool, Sales: sales.Gate{}, Documents: docs, Notifier: ntsWr, Logger: logger})

	if err := s.people(); err != nil {
		return fmt.Errorf("people: %w", err)
	}
	if err := s.catalog(); err != nil {
		return fmt.Errorf("catalog: %w", err)
	}
	if err := s.operations(); err != nil {
		return fmt.Errorf("sales: %w", err)
	}
	if os.Getenv("SEED_HISTORY") != "" {
		if err := s.history(); err != nil {
			return fmt.Errorf("history: %w", err)
		}
	}
	fmt.Printf("Seed ready. Sign in with a document number and the password %q:\n", password)
	for _, p := range s.staff {
		fmt.Printf("  %-26s DNI %s  (%s)\n", p.name, p.doc, p.role)
	}
	return nil
}

type person struct {
	id         uuid.UUID
	doc, name  string
	role       auth.Role
	first, end string
}

type vehicle struct{ model, version, color uuid.UUID }

type seeder struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	store    storage.Store
	sales    *sales.Service
	tasks    *tasks.Service
	docs     *documents.Service
	staff    []person
	admin    person
	manager  person
	advisors []person
	cars     []vehicle
	clients  []uuid.UUID

	// History mode (see history.go): a backdated clock, a hook that moves it between steps, and the prices
	// of the sale being built. All are nil in the normal seed.
	clk   *fakeClock
	tick  func(kind string, task int)
	price *pricing
}

func (s *seeder) exec(sql string, args ...any) error {
	_, err := s.pool.Exec(s.ctx, sql, args...)
	return err
}

func (s *seeder) people() error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	specs := []struct {
		first, last, doc string
		role             auth.Role
	}{
		{"Carla", "Mendoza Rivas", "40123001", auth.RoleSystemAdmin},
		{"Jorge", "Salas Prieto", "40123002", auth.RoleSalesManager},
		{"Diego", "Ramírez Soto", "40123003", auth.RoleSalesAdvisor},
		{"María Fernanda", "Torres Salazar", "40123004", auth.RoleSalesAdvisor},
		{"Valeria", "Cáceres Núñez", "40123005", auth.RoleSalesAdvisor},
	}
	for i, sp := range specs {
		id := ids.New()
		email := strings.ToLower(strings.Split(sp.first, " ")[0]) + "." + strings.ToLower(strings.Split(sp.last, " ")[0]) + "@seed.grupopana.pe"
		email = strings.NewReplacer("í", "i", "é", "e", "á", "a", "ó", "o", "ú", "u").Replace(email)
		if err := s.exec(`INSERT INTO employees (id, document_type, document_number, first_name, last_name, nationality, gender, birthdate,
				hire_date, business_phone, business_email, role, password_hash, activated_at, created_by)
			VALUES ($1, 'DNI', $2, $3, $4, 'Peruana', $5, DATE '1990-01-01', DATE '2021-03-01', $6, $7, $8::text::employee_role, $9, now(), $10)`,
			id, sp.doc, sp.first, sp.last, map[bool]string{true: "F", false: "M"}[sp.first == "Carla" || sp.first == "María Fernanda" || sp.first == "Valeria"],
			fmt.Sprintf("+51 99%07d", 1000100+i), email, string(sp.role), hash, auth.SystemActorID); err != nil {
			return err
		}
		p := person{id: id, doc: sp.doc, name: sp.first + " " + sp.last, role: sp.role}
		s.staff = append(s.staff, p)
		switch sp.role {
		case auth.RoleSystemAdmin:
			s.admin = p
		case auth.RoleSalesManager:
			s.manager = p
		default:
			s.advisors = append(s.advisors, p)
		}
	}
	// One advisor has a profile picture, so the interface can be seen both ways.
	key := "employees/" + s.advisors[0].id.String() + "/seed.png"
	data := avatar(color.RGBA{0xc9, 0x0e, 0x16, 255})
	if err := s.store.Put(s.ctx, key, "image/png", data); err != nil {
		return err
	}
	if err := s.exec(`UPDATE employees SET profile_picture_key = $2, profile_picture_mime = 'image/png', profile_picture_size = $3 WHERE id = $1`,
		s.advisors[0].id, key, len(data)); err != nil {
		return err
	}

	customers := []struct {
		kind, docType, doc, first, last, legal, nationality string
	}{
		{"diplomatico", "DNI", "07123401", "Carlos Alberto", "Mendoza Ruiz", "", "Peruana"},
		{"diplomatico", "CE", "001234567", "Lucía Fernanda", "Rojas Díaz", "", "Colombiana"},
		{"diplomatico", "PAS", "P7654321", "Hiroshi", "Tanaka Mori", "", "Japonesa"},
		{"embajada", "RUC", "20549876123", "", "", "Embajada de Canadá", "Canadiense"},
		{"organismo_internacional", "RUC", "20100123456", "", "", "Organización Internacional del Café", "Peruana"},
	}
	for i, c := range customers {
		id := ids.New()
		email := fmt.Sprintf("cliente%d@seed.example.org", i+1)
		if c.legal == "" {
			err = s.exec(`INSERT INTO customers (id, customer_type, document_type, document_number, first_name, last_name, gender, nationality, phone, email, created_by)
				VALUES ($1, $2::text::customer_type, $3::text::customer_document_type, $4, $5, $6, 'M', $7, '+51 987 654 3' || $8, $9, $10)`,
				id, c.kind, c.docType, c.doc, c.first, c.last, c.nationality, fmt.Sprint(10+i), email, s.manager.id)
		} else {
			err = s.exec(`INSERT INTO customers (id, customer_type, document_type, document_number, legal_name, nationality, phone, email,
					contact_first_name, contact_last_name, contact_job_title, created_by)
				VALUES ($1, $2::text::customer_type, 'RUC', $3, $4, $5, '+51 1 444 00' || $6, $7, 'Marta', 'Ibáñez', 'Agregada administrativa', $8)`,
				id, c.kind, c.doc, c.legal, c.nationality, fmt.Sprint(10+i), email, s.manager.id)
		}
		if err != nil {
			return err
		}
		s.clients = append(s.clients, id)
	}
	return nil
}

// avatar draws a simple round-cornered picture, enough to see how pictures are shown.
func avatar(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			dx, dy := float64(x-100), float64(y-78)
			head := dx*dx+dy*dy < 38*38
			bx, by := float64(x-100), float64(y-190)
			body := bx*bx/(80*80)+by*by/(78*78) < 1
			if head || body {
				img.Set(x, y, color.RGBA{255, 255, 255, 235})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// carImage draws a flat side view of a car in the given color: a stand-in for a catalog photograph.
func carImage(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{0xee, 0xef, 0xf1, 255}}, image.Point{}, draw.Src)
	fill := func(x0, y0, x1, y1 int, col color.RGBA) {
		draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{col}, image.Point{}, draw.Src)
	}
	dark := color.RGBA{0x24, 0x26, 0x2b, 255}
	fill(70, 230, 570, 300, c)  // body
	fill(190, 160, 450, 232, c) // cabin
	fill(210, 175, 310, 228, color.RGBA{0xcf, 0xe3, 0xf0, 255})
	fill(322, 175, 432, 228, color.RGBA{0xcf, 0xe3, 0xf0, 255})
	fill(60, 292, 580, 304, dark) // sill
	for _, cx := range []int{170, 470} {
		for y := 270; y < 350; y++ {
			for x := cx - 40; x < cx+40; x++ {
				dx, dy := float64(x-cx), float64(y-310)
				switch {
				case dx*dx+dy*dy < 26*26:
					img.Set(x, y, color.RGBA{0xc9, 0xcc, 0xd2, 255})
				case dx*dx+dy*dy < 40*40:
					img.Set(x, y, dark)
				}
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func (s *seeder) catalog() error {
	type model struct {
		name, category string
		versions       map[string]int
		colors         map[string]color.RGBA
	}
	models := []model{
		{"Yaris", "sedan", map[string]int{"1.5 Full CVT": 21990, "1.3 GLi MT": 17990}, map[string]color.RGBA{"Blanco Súper": {0xf4, 0xf4, 0xf2, 255}, "Gris Metálico": {0x8a, 0x8f, 0x98, 255}}},
		{"Corolla Cross", "familiar", map[string]int{"2.0 XEI CVT": 31990, "2.0 SEG CVT": 35990}, map[string]color.RGBA{"Blanco Perlado": {0xe9, 0xe7, 0xe0, 255}, "Rojo Intenso": {0xb5, 0x12, 0x1b, 255}}},
		{"Hilux", "camioneta", map[string]int{"2.8 SRV 4×4 AT": 49990, "2.4 SR 4×2 MT": 36990}, map[string]color.RGBA{"Negro Mica": {0x1f, 0x21, 0x25, 255}, "Plata": {0xb7, 0xbb, 0xc2, 255}}},
		{"Raize", "hatchback", map[string]int{"1.0T CVT": 19990}, map[string]color.RGBA{"Azul Eléctrico": {0x23, 0x5b, 0xc8, 255}}},
	}
	for _, m := range models {
		tx, err := s.pool.Begin(s.ctx) // a model needs its category the moment it exists
		if err != nil {
			return err
		}
		defer tx.Rollback(s.ctx) // a no-op once committed; on an error it frees the connection
		exec := func(sql string, args ...any) error { _, err := tx.Exec(s.ctx, sql, args...); return err }
		modelID := ids.New()
		if err := exec(`INSERT INTO vehicle_models (id, model_name, created_by) VALUES ($1, $2, $3)`, modelID, m.name, s.admin.id); err != nil {
			return err
		}
		if err := exec(`INSERT INTO vehicle_model_categories (model_id, category) VALUES ($1, $2::text::vehicle_category)`, modelID, m.category); err != nil {
			return err
		}
		versionIDs, colorIDs := []uuid.UUID{}, []uuid.UUID{}
		for name, price := range m.versions {
			id := ids.New()
			if err := exec(`INSERT INTO vehicle_versions (id, model_id, version_name, reference_price, created_by) VALUES ($1, $2, $3, $4, $5)`, id, modelID, name, price, s.admin.id); err != nil {
				return err
			}
			versionIDs = append(versionIDs, id)
		}
		for name, rgba := range m.colors {
			id := ids.New()
			if err := exec(`INSERT INTO vehicle_colors (id, model_id, color_name, created_by) VALUES ($1, $2, $3, $4)`, id, modelID, name, s.admin.id); err != nil {
				return err
			}
			key := "catalog/seed/" + id.String() + ".png"
			data := carImage(rgba)
			if err := s.store.Put(s.ctx, key, "image/png", data); err != nil {
				return err
			}
			if err := exec(`INSERT INTO catalog_images (id, vehicle_color_id, viewpoint, mime_type, size_bytes, storage_key, uploaded_by)
				VALUES ($1, $2, 'main_view', 'image/png', $3, $4, $5)`, ids.New(), id, len(data), key, s.admin.id); err != nil {
				return err
			}
			colorIDs = append(colorIDs, id)
		}
		for _, v := range versionIDs {
			s.cars = append(s.cars, vehicle{model: modelID, version: v, color: colorIDs[rand.IntN(len(colorIDs))]})
		}
		if err := tx.Commit(s.ctx); err != nil {
			return err
		}
	}
	return nil
}

func digits(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(byte('0' + rand.IntN(10)))
	}
	return b.String()
}

func (s *seeder) today() string {
	now := time.Now()
	if s.clk != nil {
		now = s.clk.Now()
	}
	return now.In(time.FixedZone("Lima", -5*3600)).Format("2006-01-02")
}

// pause lets history mode move its clock; it does nothing in the normal seed.
func (s *seeder) pause(kind string, task int) {
	if s.tick != nil {
		s.tick(kind, task)
	}
}

func (s *seeder) data(n int) string {
	d := s.today()
	switch n {
	case 1:
		if p := s.price; p != nil {
			return fmt.Sprintf(`{"initialToyotaAmount":"%s","initialCommission":"%s","accessoryAmount":"%s","releaseProcessingAmount":"%s"}`,
				cents(p.toyota), cents(p.commission), cents(p.accessories), cents(p.release))
		}
		return `{"initialToyotaAmount":"30000","initialCommission":"2500","accessoryAmount":"1200.50","releaseProcessingAmount":"800"}`
	case 2:
		return fmt.Sprintf(`{"signerDocumentType":"DNI","signerDocumentNumber":"%s","signerFirstName":"Ana","signerLastName":"Rojas","pepStatus":"no_pep","ddcRegime":"general"}`, digits(8))
	case 3:
		final := "31000"
		if p := s.price; p != nil {
			final = cents(p.finalToyota)
		}
		return fmt.Sprintf(`{"vehicleSeries":"S%s","vin":"JT4RN81A0M5%s","finalToyotaAmount":"%s"}`, digits(8), digits(6), final)
	case 4:
		deposit := "34500.50"
		if p := s.price; p != nil {
			deposit = cents(p.quoted())
		}
		return fmt.Sprintf(`{"bank":"bcp","paymentDate":"%s","transactionNumber":"T%s","depositedAmount":"%s"}`, d, digits(8), deposit)
	case 5:
		deposit := "31000"
		if p := s.price; p != nil {
			deposit = cents(p.finalToyota)
		}
		return fmt.Sprintf(`{"bank":"bbva","paymentDate":"%s","transactionNumber":"T%s","depositedAmount":"%s"}`, d, digits(8), deposit)
	case 6:
		amounts := [4]string{"31000", "1500", "1200.50", "800"} // Toyota, commission, accessories, release
		if p := s.price; p != nil {
			amounts = [4]string{cents(p.finalToyota), cents(p.finalCommission()), cents(p.accessories), cents(p.release)}
		}
		return fmt.Sprintf(`{"toyota":{"number":"F001-%s","date":"%s","amount":"%s"},"commission":{"number":"F002-%s","date":"%s","amount":"%s"},"accessories":{"number":"F003-%s","date":"%s","amount":"%s"},"releaseProcessing":{"number":"F004-%s","date":"%s","amount":"%s"}}`,
			digits(6), d, amounts[0], digits(6), d, amounts[1], digits(6), d, amounts[2], digits(6), d, amounts[3])
	case 8:
		return fmt.Sprintf(`{"damNumber":"118-2026-%s","customsOffice":"callao_maritima","controlChannel":"green"}`, digits(8))
	case 9:
		return `{"personInCharge":"Juan Pérez"}`
	case 10:
		return fmt.Sprintf(`{"licensePlate":"A%s","registrationDate":"%s"}`, digits(5), d)
	case 11:
		return fmt.Sprintf(`{"pdsTransferDate":"%s"}`, d)
	case 12:
		return fmt.Sprintf(`{"serviceOrderNumber":"OS-%s","equipmentCompletionDate":"%s"}`, digits(5), d)
	case 13:
		return fmt.Sprintf(`{"deliveryDate":"%s"}`, d)
	}
	return `{}`
}

var pdf = []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")

func xlsx() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml"} {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte("<x/>"))
	}
	_ = w.Close()
	return buf.Bytes()
}

// work does tasks 1..n of a sale as its advisor, uploading real files; with partial it leaves the task after the last one
// started and half filled.
func (s *seeder) work(sale uuid.UUID, advisor person, upTo int, startNext bool) error {
	actor := auth.Actor{ID: advisor.id, Role: advisor.role}
	do := func(n int, finish bool) error {
		s.pause("wait", n)
		if _, err := s.tasks.Start(s.ctx, actor, sale, n); err != nil {
			return fmt.Errorf("start %d: %w", n, err)
		}
		if n == 7 && !finish {
			return nil
		}
		if n != 7 {
			detail, err := s.tasks.Get(s.ctx, actor, sale, n)
			if err != nil {
				return err
			}
			v := detail.Version
			if _, err := s.tasks.SaveData(s.ctx, actor, sale, n, tasks.SaveInput{Version: &v, Data: []byte(s.data(n))}); err != nil {
				return fmt.Errorf("save %d: %w", n, err)
			}
		}
		if !finish {
			return nil
		}
		s.pause("work", n)
		detail, err := s.tasks.Get(s.ctx, actor, sale, n)
		if err != nil {
			return err
		}
		for _, slot := range detail.Documents {
			if !slot.Required || slot.File != nil {
				continue
			}
			bytes := pdf
			if slot.AllowedFormat == "excel" {
				bytes = xlsx()
			}
			if _, _, err := s.docs.Upload(s.ctx, actor, documents.UploadInput{SaleID: sale, Number: n, Slot: slot.SlotCode, FileName: slot.SlotCode + ".pdf", Data: bytes}); err != nil {
				return fmt.Errorf("upload %s: %w", slot.SlotCode, err)
			}
		}
		if _, err := s.tasks.Complete(s.ctx, actor, sale, n); err != nil {
			return fmt.Errorf("complete %d: %w", n, err)
		}
		return nil
	}
	for n := 1; n <= upTo; n++ {
		if err := do(n, true); err != nil {
			return err
		}
	}
	if startNext && upTo < 13 {
		return do(upTo+1, false)
	}
	return nil
}

func (s *seeder) operations() error {
	type plan struct {
		client, car, advisor int
		temperature          string
		done                 int
		inProgress           bool
		late                 bool
		cancel               bool
		note                 string
	}
	plans := []plan{
		{0, 1, 0, "warm", 0, false, false, false, ""},
		{3, 2, 1, "hot", 4, true, false, false, "La embajada pidió acelerar el trámite aduanero."},
		{4, 4, 2, "hot", 7, true, true, false, ""},
		{1, 0, 0, "cold", 2, true, false, false, ""},
		{2, 5, 1, "warm", 10, true, false, false, ""},
		{0, 3, 2, "warm", 13, false, false, false, "Entregada sin observaciones."},
		{3, 6, 0, "cold", 3, false, false, true, ""},
		{1, 1, 1, "warm", 1, true, false, false, ""},
		{4, 2, 0, "hot", 12, true, false, false, ""},
	}
	for _, p := range plans {
		advisor := s.advisors[p.advisor]
		car := s.cars[p.car%len(s.cars)]
		sale, err := s.sales.Create(s.ctx, auth.Actor{ID: s.manager.id, Role: s.manager.role}, sales.CreateInput{
			CustomerID: s.clients[p.client], VersionID: car.version, ColorID: car.color, AdvisorID: &advisor.id,
			Temperature: p.temperature, Notes: p.note,
		})
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		if err := s.work(sale.ID, advisor, p.done, p.inProgress); err != nil {
			return err
		}
		if p.late {
			if err := s.exec(`UPDATE task_instances SET started_at = started_at - interval '9 days', due_date = now() - interval '2 days'
				WHERE sale_id = $1 AND status = 'in_progress'`, sale.ID); err != nil {
				return err
			}
		}
		if p.cancel {
			if _, err := s.sales.Cancel(s.ctx, auth.Actor{ID: s.manager.id, Role: s.manager.role}, sale.ID, "customer_withdrew", "El cliente desistió por un cambio de destino."); err != nil {
				return err
			}
		}
	}
	return nil
}
