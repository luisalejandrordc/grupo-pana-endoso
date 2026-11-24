package notifications

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/ids"
	"endosoflow/internal/testkit"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- templates (no database needed) -------------------------------------------------------------

func TestEveryTemplateRendersBothVersions(t *testing.T) {
	previews := Previews("https://app.test", "https://portal.test", "https://cdn.test/logo.png")
	if len(previews) != len(previewTypes) {
		t.Fatalf("%d of %d notification types have a template", len(previews), len(previewTypes))
	}
	for _, p := range previews {
		if p.Subject == "" || !strings.HasPrefix(p.HTML, "<!DOCTYPE html>") || !strings.Contains(p.HTML, `src="https://cdn.test/logo.png"`) {
			t.Errorf("%s: incomplete HTML or subject", p.Type)
		}
		if !strings.Contains(p.Text, "— "+brand) || strings.Contains(p.Text, "<") {
			t.Errorf("%s: the text version must be plain text with a signature, got %q", p.Type, p.Text)
		}
	}
}

func TestAccountEmailsCarryTheLinkInBothVersions(t *testing.T) {
	for _, kind := range []string{EmployeeActivation, EmployeePasswordReset, CustomerActivation} {
		d := sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png")
		d.Link = "https://app.test/activate-account?token=abc_DEF-123"
		r, ok := render(d)
		if !ok {
			t.Fatalf("%s has no template", kind)
		}
		if !strings.Contains(r.Text, d.Link) || !strings.Contains(r.HTML, `href="`+d.Link+`"`) {
			t.Errorf("%s: the link must be in the text and as a button in the HTML", kind)
		}
		if !strings.Contains(r.Text, "72 horas") {
			t.Errorf("%s: the email must say how long the link lasts", kind)
		}
	}
}

func TestEmailsEscapeWhatPeopleTyped(t *testing.T) {
	d := sampleData(SaleCreated, "https://app.test", "https://portal.test", "https://cdn.test/logo.png")
	d.GreetingName = `<script>alert("x")</script>`
	d.AdvisorName = `Ana <b>"Pro"</b> & Co`
	r, _ := render(d)
	if strings.Contains(r.HTML, "<script>") || strings.Contains(r.HTML, "<b>") {
		t.Fatalf("names must be escaped in the HTML:\n%s", r.HTML)
	}
}

// The nine customer notifications of the specification (the seven task emails, creation and welcome)
var specSubjects = map[string]string{
	CustomerActivation:  "Bienvenida a Grupo Pana: acceso a su portal de seguimiento", // the sample customer is a woman
	CustomerActivated:   "Su cuenta de Grupo Pana está activa",
	SaleCreated:         "Hemos registrado su nueva solicitud de vehículo",
	"milestone_task_1":  "Su cotización oficial está lista para revisión",
	"milestone_task_3":  "Siguiente paso: su vehículo ha sido asignado",
	"milestone_task_6":  "Sus comprobantes de pago han sido emitidos",
	"milestone_task_7":  "Iniciamos los trámites aduaneros de su vehículo",
	"milestone_task_10": "¡Su vehículo ya cuenta con placa oficial!",
	"milestone_task_11": "Su vehículo está en fase de preparación final",
	"milestone_task_13": "¡Felicidades por su nuevo vehículo! Gracias por confiar en Grupo Pana",
	AdvisorChanged:      "Ha cambiado el asesor de su compra",
}

func TestCustomerEmailsHaveTheSpecifiedSubjects(t *testing.T) {
	for kind, want := range specSubjects {
		if r, ok := render(sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png")); !ok || r.Subject != want {
			t.Errorf("%s: subject %q, want %q", kind, r.Subject, want)
		}
	}
}

// informal matches the "tú" forms; every email must address the reader as "usted".
var informal = regexp.MustCompile(`(?i)\b(tu|tus|tú|ti|contigo|tienes|puedes|eres|estás|revisa|ignora|elige|recuerda)\b`)

func TestEveryEmailAddressesTheReaderAsUsted(t *testing.T) {
	for _, p := range Previews("https://app.test", "https://portal.test", "https://cdn.test/logo.png") {
		if m := informal.FindString(p.Subject + "\n" + p.Text); m != "" {
			t.Errorf("%s uses the informal %q:\n%s", p.Type, m, p.Text)
		}
	}
}

func TestEmailsToCustomersLeadToTheCustomerPortal(t *testing.T) {
	for kind := range specSubjects {
		d := sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png")
		d.Link = "https://portal.test/activate-account?token=abc"
		r, _ := render(d)
		if strings.Contains(r.HTML, "app.test") {
			t.Errorf("%s must not send a customer to the employee portal", kind)
		}
		if !strings.Contains(r.HTML, `href="https://portal.test/`) {
			t.Errorf("%s has no button into the customer portal", kind)
		}
	}
	r, _ := render(sampleData(SaleAssigned, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(r.HTML, `href="https://app.test/sales/`) {
		t.Error("an advisor's email opens the sale in the employee application")
	}
}

func TestTaskEmailsOpenTheRightPlaceInThePortal(t *testing.T) {
	d := sampleData("milestone_task_1", "https://app.test", "https://portal.test", "https://cdn.test/logo.png")
	id := d.SaleID
	for kind, want := range map[string]string{
		"milestone_task_1":  "/sales/" + id + "/documents", // the quotation PDF
		"milestone_task_3":  "/sales/" + id,
		"milestone_task_6":  "/sales/" + id + "/documents", // the invoices
		"milestone_task_7":  "/sales/" + id,
		"milestone_task_10": "/sales/" + id + "/documents", // the TIVe
		"milestone_task_11": "/sales/" + id,
		"milestone_task_13": "/sales/" + id + "/documents", // the signed delivery record
		SaleCreated:         "/sales/" + id,
	} {
		r, _ := render(sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
		if !strings.Contains(r.HTML, `href="https://portal.test`+want+`"`) {
			t.Errorf("%s: expected a button to %s", kind, want)
		}
	}
}

func TestProgressEmailsCarryNoRecordedDataBecauseThePortalIsTheSourceOfTruth(t *testing.T) {
	// A manager can correct the VIN, the plate or the delivery date after an email went out; the email cannot be
	// corrected. So these emails announce the event and point to the portal, and print none of it.
	for _, kind := range []string{"milestone_task_3", "milestone_task_10", "milestone_task_13"} {
		r, _ := render(sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
		for _, banned := range []string{"VIN", "Número de serie", "Placa:", "Fecha de entrega", "serie:"} {
			if strings.Contains(r.Text, banned) || strings.Contains(r.HTML, banned+"</td>") {
				t.Errorf("%s must not print %q:\n%s", kind, banned, r.Text)
			}
		}
		if !strings.Contains(r.HTML, `href="https://portal.test/sales/`) {
			t.Errorf("%s must lead to the purchase in the portal", kind)
		}
	}
	// What stays identifies the purchase and does not change.
	r, _ := render(sampleData("milestone_task_3", "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(r.Text, "Compra: EF-2026-00042") || !strings.Contains(r.Text, "Vehículo: ") {
		t.Fatal("the sale code and the vehicle identify the purchase")
	}
	// Logging in is by document, which never changes, and the welcome emails must say which one.
	w, _ := render(sampleData(CustomerActivation, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(w.Text, "Su usuario de ingreso: DNI 45678912") {
		t.Fatal("the welcome email names the login document")
	}
}

func TestEveryPurchaseEmailShowsTheAdvisorAndTheWelcomeDoesNot(t *testing.T) {
	// The advisor can change, but then a new email tells the customer (Notification 10), so the card stays right.
	for kind, shows := range map[string]bool{
		SaleCreated: true, AdvisorChanged: true,
		"milestone_task_1": true, "milestone_task_6": true, "milestone_task_13": true,
		CustomerActivation: false, // no sale yet, so no advisor
	} {
		r, _ := render(sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
		has := strings.Contains(r.Text, "carlos.mendoza@grupopana.pe") && strings.Contains(r.HTML, "wa.me/")
		if has != shows {
			t.Errorf("%s: advisor card shown = %v, want %v", kind, has, shows)
		}
	}
}

func TestTheProgressBarShowsTheSixMilestonesAndTheirState(t *testing.T) {
	r, _ := render(sampleData("milestone_task_6", "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	for _, name := range []string{
		"Cotización y Documentación", "Asignación y Pago", "Facturación Oficial",
		"Trámites Aduaneros", "Liberación y Placas", "Preparación y Entrega",
	} {
		if !strings.Contains(r.HTML, name) || !strings.Contains(r.Text, name) {
			t.Errorf("the milestone %q is missing", name)
		}
	}
	if !strings.Contains(r.HTML, "3 de 6 etapas completadas") || strings.Contains(r.HTML, "de 7") {
		t.Fatal("after the invoices, three of six milestones are completed")
	}
	if !strings.Contains(r.Text, "✓ 3. Facturación Oficial") || !strings.Contains(r.Text, "▶ 4. Trámites Aduaneros") || !strings.Contains(r.Text, "○ 5. Liberación y Placas") {
		t.Fatalf("the text version marks completed, current and pending milestones:\n%s", r.Text)
	}
	done, _ := render(sampleData("milestone_task_13", "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(done.HTML, "compra completada") {
		t.Fatal("the last email says the purchase is complete")
	}
	if !strings.Contains(r.HTML, "Etapa 3 de 6 · Facturación Oficial") {
		t.Fatal("the headline names the milestone the email belongs to")
	}
	advisorChange, _ := render(sampleData(AdvisorChanged, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if strings.Contains(advisorChange.HTML, "etapas completadas") {
		t.Fatal("an advisor change is not about progress")
	}
}

func TestEveryTaskThatNotifiesHasAnEmail(t *testing.T) {
	for task := 1; task <= 13; task++ {
		kind := MilestoneTaskType(task)
		if kind == "" {
			continue
		}
		if _, ok := stages[kind]; !ok {
			t.Errorf("task %d queues %s but there is no template for it", task, kind)
		}
	}
}

func TestTheAdvisorCardOffersWhatsApp(t *testing.T) {
	for phone, want := range map[string]string{
		"987654321":         "https://wa.me/51987654321",
		"987 654 321":       "https://wa.me/51987654321",
		"+51 987 654 321":   "https://wa.me/51987654321",
		"+1 (415) 555-0199": "https://wa.me/14155550199",
		"(01) 234-5678":     "", // a landline: no chat address
		"":                  "",
	} {
		if got := whatsappURL(phone); got != want {
			t.Errorf("%q: got %q, want %q", phone, got, want)
		}
	}
	r, _ := render(sampleData(SaleCreated, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(r.HTML, `href="https://wa.me/51987654321"`) || !strings.Contains(r.Text, "WhatsApp del asesor: https://wa.me/51987654321") {
		t.Fatal("the advisor card must offer a WhatsApp chat")
	}
}

func TestOnlyEmailsFromAnAdvisorAreRepliedToTheAdvisor(t *testing.T) {
	for kind, want := range map[string]bool{
		SaleCreated: true, "milestone_task_3": true, AdvisorChanged: true,
		CustomerActivation: false, EmployeeActivation: false, SaleAssigned: false,
	} {
		if r, _ := render(sampleData(kind, "https://app.test", "https://portal.test", "https://cdn.test/logo.png")); r.ReplyToAdvisor != want {
			t.Errorf("%s: ReplyToAdvisor = %v, want %v", kind, r.ReplyToAdvisor, want)
		}
	}
}

func TestTheHeaderShowsTheLogoWhenThereIsAnAddressAndTheNameWhenNot(t *testing.T) {
	with, _ := render(sampleData(SaleCreated, "https://app.test", "https://portal.test", "https://cdn.test/logo.png"))
	if !strings.Contains(with.HTML, `<img src="https://cdn.test/logo.png"`) || !strings.Contains(with.HTML, `alt="EndosoFlow"`) {
		t.Fatalf("expected the logo image with an alt text and an escaped address:\n%s", with.HTML)
	}
	without, _ := render(sampleData(SaleCreated, "https://app.test", "https://portal.test", ""))
	if strings.Contains(without.HTML, "<img") || !strings.Contains(without.HTML, "Endoso<span") {
		t.Fatal("without a logo address the header must show the brand name as text")
	}
}

func TestGreetingsAndWelcomesAgreeWithTheRecipientsGender(t *testing.T) {
	for gender, want := range map[string]struct{ dear, welcome string }{
		"M": {"Estimado Carlos,", "Bienvenido"},
		"F": {"Estimada Lucía,", "Bienvenida"},
		"":  {"Estimado(a) Lucía,", "Bienvenido(a)"}, // an organization has no gender recorded
	} {
		name := "Lucía"
		if gender == "M" {
			name = "Carlos"
		}
		for _, kind := range []string{CustomerActivation, CustomerActivated, SaleCreated, "milestone_task_6", AdvisorChanged} {
			d := sampleData(kind, "https://app.test", "https://portal.test", "")
			d.Gender, d.GreetingName = gender, name
			r, _ := render(d)
			if !strings.Contains(r.Text, want.dear) || !strings.Contains(r.HTML, want.dear) {
				t.Errorf("gender %q, %s: expected the greeting %q:\n%s", gender, kind, want.dear, r.Text)
			}
		}
		for kind, label := range map[string]string{CustomerActivation: " a Grupo Pana", EmployeeActivation: " al equipo"} {
			d := sampleData(kind, "https://app.test", "https://portal.test", "")
			d.Gender = gender
			r, _ := render(d)
			if !strings.Contains(r.HTML, want.welcome+label) {
				t.Errorf("gender %q, %s: expected %q", gender, kind, want.welcome+label)
			}
			if gender != "" && strings.Contains(r.HTML+r.Subject, "(a)") {
				t.Errorf("gender %q, %s: a known gender must not leave an \"(a)\"", gender, kind)
			}
		}
	}
	d := sampleData(CustomerActivation, "https://app.test", "https://portal.test", "")
	d.Gender = "M"
	if r, _ := render(d); r.Subject != "Bienvenido a Grupo Pana: acceso a su portal de seguimiento" {
		t.Fatalf("the subject follows the gender too: %q", r.Subject)
	}
}

func TestAdvisorWordingAgreesWithTheAdvisorsGender(t *testing.T) {
	woman := func(kind string) rendered {
		d := sampleData(kind, "https://app.test", "https://portal.test", "")
		d.AdvisorGender, d.AdvisorName = "F", "Carla Mendoza"
		r, _ := render(d)
		return r
	}
	man := func(kind string) rendered {
		d := sampleData(kind, "https://app.test", "https://portal.test", "")
		d.AdvisorGender = "M"
		r, _ := render(d)
		return r
	}
	for _, kind := range []string{SaleCreated, "milestone_task_6", "milestone_task_11", AdvisorChanged} {
		w, m := woman(kind), man(kind)
		for _, want := range []string{"Su asesora: Carla Mendoza", "Correo de la asesora:", "Teléfono de la asesora:", "WhatsApp de la asesora:", "directamente a su asesora"} {
			if !strings.Contains(w.Text, want) && !strings.Contains(w.HTML, want) {
				t.Errorf("%s with a woman as advisor: expected %q:\n%s", kind, want, w.Text)
			}
		}
		if !strings.Contains(w.HTML, ">Su asesora</div>") {
			t.Errorf("%s: the card label must say \"Su asesora\"", kind)
		}
		if strings.Contains(m.Text, "asesora") || strings.Contains(m.HTML, "asesora") {
			t.Errorf("%s with a man as advisor must not say \"asesora\"", kind)
		}
	}
	if !strings.Contains(woman("milestone_task_11").Text, "su asesora está a su disposición") {
		t.Fatal("the advisor in a sentence agrees too")
	}
	if !strings.Contains(woman(AdvisorChanged).HTML, "su nueva asesora") { // in the preview line
		t.Fatal("\"nuevo\" becomes \"nueva\" with the advisor")
	}
	if !strings.Contains(woman(SaleUnassigned).Text, "Nueva asesora: Carla Mendoza") {
		t.Fatal("the label of the employee email agrees as well")
	}
}

func TestSpanishDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{72 * time.Hour: "72 horas", time.Hour: "1 hora", 2 * time.Hour: "2 horas", 20 * time.Minute: "20 minutos"} {
		if got := spanishDuration(d); got != want {
			t.Errorf("%v: got %q, want %q", d, got, want)
		}
	}
}

// ---- the queue, the wake-up and the links (database) --------------------------------------------

type captureSender struct {
	mu   sync.Mutex
	msgs []mail.Message
	err  error // returned instead of sending, when set
}

func (c *captureSender) Send(_ context.Context, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.msgs = append(c.msgs, m)
	return nil
}

func (c *captureSender) to(address string) []mail.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []mail.Message
	for _, m := range c.msgs {
		if m.To == address {
			out = append(out, m)
		}
	}
	return out
}

type accountFixture struct {
	auth   *auth.Service // the one that activates accounts, so it queues the confirmations
	env    *testkit.Env
	sender *captureSender
	waker  *Waker
	writer Writer
	disp   *Dispatcher
}

func newAccountFixture(t *testing.T, tweak func(*DispatcherDeps)) *accountFixture {
	t.Helper()
	env := testkit.New(t)
	f := &accountFixture{env: env, sender: &captureSender{}, waker: NewWaker()}
	f.writer = Writer{Waker: f.waker}
	// Its own auth service, so customer links can use an address different from the employees'.
	links := auth.NewService(auth.Deps{
		DB: env.Pool, JWTSecret: "0123456789abcdef0123456789abcdef",
		AppBaseURL: "http://app.test", PortalBaseURL: "https://portal.test", Notifier: f.writer, Logger: env.Logger,
	})
	f.auth = links
	deps := DispatcherDeps{
		DB: env.Pool, Sender: f.sender, Links: links, Wake: f.waker, AppBaseURL: "http://app.test",
		PortalBaseURL: "https://portal.test", LogoURL: "https://cdn.test/logo.png", Logger: env.Logger,
	}
	if tweak != nil {
		tweak(&deps)
	}
	f.disp = NewDispatcher(deps)
	return f
}

// pendingEmployee inserts an employee who has not activated yet.
func (f *accountFixture) pendingEmployee(t *testing.T) (id uuid.UUID, email string) {
	t.Helper()
	id = ids.New()
	email = fmt.Sprintf("nuevo-%s@test.pe", id)
	_, err := f.env.Pool.Exec(f.env.Ctx, `INSERT INTO employees
		(id, document_type, document_number, first_name, last_name, nationality, gender, birthdate, hire_date,
		 business_phone, business_email, role, created_by)
		VALUES ($1, 'DNI', $2, 'Marta', 'Nueva', 'Peruana', 'F', DATE '1992-02-02', DATE '2026-01-01',
				'999000111', $3, 'sales_advisor', $4)`,
		id, fmt.Sprintf("%08d", rand.IntN(100_000_000)), email, auth.SystemActorID)
	if err != nil {
		t.Fatalf("insert employee: %v", err)
	}
	t.Cleanup(func() {
		f.env.Replica(func(c *pgxpool.Conn) {
			for _, stmt := range []string{
				`DELETE FROM account_tokens WHERE employee_id = $1`,
				`DELETE FROM notification_logs WHERE recipient_employee_id = $1`,
				`DELETE FROM access_logs WHERE employee_id = $1`,
				`DELETE FROM employees WHERE id = $1`,
			} {
				_, _ = c.Exec(context.Background(), stmt, id)
			}
		})
	})
	return id, email
}

func (f *accountFixture) queueActivation(t *testing.T, id uuid.UUID, email string) {
	t.Helper()
	if err := database.WithTx(f.env.Ctx, f.env.Pool, func(tx pgx.Tx) error {
		return f.writer.EmployeeActivation(f.env.Ctx, tx, id, email)
	}); err != nil {
		t.Fatalf("queue: %v", err)
	}
}

var tokenIn = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

func (f *accountFixture) status(t *testing.T, email string) (status, lastError string) {
	t.Helper()
	var detail *string
	err := f.env.Pool.QueryRow(f.env.Ctx, `SELECT status::text, last_error FROM notification_logs WHERE recipient_email = $1 ORDER BY created_at DESC LIMIT 1`, email).Scan(&status, &detail)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if detail != nil {
		lastError = *detail
	}
	return
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTheActivationEmailCarriesALinkThatWorks(t *testing.T) {
	f := newAccountFixture(t, nil)
	id, email := f.pendingEmployee(t)
	f.queueActivation(t, id, email)

	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	sent := f.sender.to(email)
	if len(sent) != 1 || !strings.Contains(sent[0].Subject, "Active su cuenta") || sent[0].HTMLBody == "" {
		t.Fatalf("expected one HTML activation email, got %+v", sent)
	}
	if !strings.Contains(sent[0].HTMLBody, `src="https://cdn.test/logo.png"`) {
		t.Fatal("the configured logo address must reach the sent HTML")
	}
	if !strings.Contains(sent[0].Body, "Hola Marta") || sent[0].ReplyTo != "" {
		t.Fatalf("unexpected text version %q", sent[0].Body)
	}
	if !strings.Contains(sent[0].Body, "http://app.test/activate-account?token=") || !strings.Contains(sent[0].Body, "Su usuario de ingreso: DNI ") {
		t.Fatalf("an employee's link opens in the employee portal and names the login document:\n%s", sent[0].Body)
	}
	if !strings.Contains(sent[0].HTMLBody, "Bienvenida al equipo") {
		t.Fatal("the new employee is a woman: the welcome must agree with her gender")
	}
	match := tokenIn.FindStringSubmatch(sent[0].Body)
	if match == nil {
		t.Fatalf("no link in %q", sent[0].Body)
	}
	if st, _ := f.status(t, email); st != "sent" {
		t.Fatalf("status = %s", st)
	}
	account, err := f.auth.Activate(f.env.Ctx, match[1], testkit.Password, auth.ClientInfo{})
	if err != nil || account != auth.AccountEmployee {
		t.Fatalf("activate: %v (%q)", err, account)
	}
	// Setting the first password queues a confirmation, which the dispatcher sends: no link, a way in, and a
	// warning for whoever did not do it.
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	sent = f.sender.to(email)
	if len(sent) != 2 {
		t.Fatalf("the activation email and then its confirmation, got %d", len(sent))
	}
	done := sent[1]
	if done.Subject != "Su cuenta de EndosoFlow está activa" || strings.Contains(done.Body, "token=") ||
		!strings.Contains(done.Body, "Su usuario de ingreso: DNI ") || !strings.Contains(done.Body, "no realizó esta activación") ||
		!strings.Contains(done.HTMLBody, `href="http://app.test/"`) {
		t.Fatalf("unexpected confirmation:\n%s %s", done.Subject, done.Body)
	}
}

func TestAnAccountThatActivatedMeanwhileGetsNoEmail(t *testing.T) {
	f := newAccountFixture(t, nil)
	id, email := f.pendingEmployee(t)
	f.queueActivation(t, id, email)
	if _, err := f.env.Pool.Exec(f.env.Ctx, `UPDATE employees SET password_hash = 'x', activated_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	if st, why := f.status(t, email); st != "skipped" || !strings.Contains(why, "activada") || len(f.sender.to(email)) != 0 {
		t.Fatalf("status %s (%s), sent %d", st, why, len(f.sender.to(email)))
	}
}

func TestARejectedEmailLeavesNoValidLink(t *testing.T) {
	f := newAccountFixture(t, nil)
	f.sender.err = fmt.Errorf("%w: INVALID_RECIPIENT", mail.ErrRejected)
	id, email := f.pendingEmployee(t)
	f.queueActivation(t, id, email)
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := f.status(t, email); st != "failed" {
		t.Fatalf("status = %s", st)
	}
	var open int
	if err := f.env.Pool.QueryRow(f.env.Ctx, `SELECT count(*) FROM account_tokens WHERE employee_id = $1 AND revoked_at IS NULL AND used_at IS NULL`, id).Scan(&open); err != nil || open != 0 {
		t.Fatalf("a link nobody received must not stay valid (open: %d, %v)", open, err)
	}
}

func TestATransientFailureIsRetriedWithAFreshLink(t *testing.T) {
	f := newAccountFixture(t, func(d *DispatcherDeps) { d.Backoff = []time.Duration{300 * time.Millisecond} })
	f.sender.err = errors.New("network down")
	id, email := f.pendingEmployee(t)
	f.queueActivation(t, id, email)
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := f.status(t, email); st != "pending" {
		t.Fatalf("a transient failure must stay pending, got %s (%s)", st, func() string { _, w := f.status(t, email); return w }())
	}
	f.sender.mu.Lock()
	f.sender.err = nil
	f.sender.mu.Unlock()
	time.Sleep(400 * time.Millisecond) // past the backoff
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	sent := f.sender.to(email)
	if len(sent) != 1 || tokenIn.FindStringSubmatch(sent[0].Body) == nil {
		t.Fatalf("expected the retry to send one email with a link, got %+v", sent)
	}
	if _, err := f.env.Auth.Activate(f.env.Ctx, tokenIn.FindStringSubmatch(sent[0].Body)[1], testkit.Password, auth.ClientInfo{}); err != nil {
		t.Fatalf("the link of the retry must work: %v", err)
	}
}

func TestCustomerWelcomeEmailActivatesTheCustomer(t *testing.T) {
	f := newAccountFixture(t, nil)
	admin := f.env.Employee(auth.RoleSystemAdmin, false)
	for _, organization := range []bool{false, true} {
		customerID := f.env.Customer(admin.ID, organization)
		var email string
		if err := f.env.Pool.QueryRow(f.env.Ctx, `SELECT email FROM customers WHERE id = $1`, customerID).Scan(&email); err != nil {
			t.Fatal(err)
		}
		queue := func() error {
			return database.WithTx(f.env.Ctx, f.env.Pool, func(tx pgx.Tx) error {
				return f.writer.CustomerActivation(f.env.Ctx, tx, customerID, email)
			})
		}
		if err := queue(); err != nil { // the welcome when the customer is created
			t.Fatal(err)
		}
		if err := queue(); err != nil { // a resend
			t.Fatal(err)
		}
		if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
			t.Fatal(err)
		}
		sent := f.sender.to(email)
		if len(sent) != 2 {
			t.Fatalf("the welcome and the resend must both be sent, got %d", len(sent))
		}
		greeting := map[bool]string{
			false: "Estimada Lucía Quispe,",
			true:  "Estimado(a) Hiro Tanaka,",
		}[organization] // an organization's contact has no recorded gender
		if !strings.Contains(sent[1].Body, greeting) || sent[1].ReplyTo != "" {
			t.Fatalf("expected %q in %q", greeting, sent[1].Body)
		}
		if !strings.Contains(sent[1].Body, "https://portal.test/activate-account?token=") ||
			!strings.Contains(sent[1].Body, "Su usuario de ingreso: "+map[bool]string{false: "DNI", true: "RUC"}[organization]+" ") {
			t.Fatalf("a customer's link opens in the customer portal and names the login document:\n%s", sent[1].Body)
		}
		oldLink, newLink := tokenIn.FindStringSubmatch(sent[0].Body), tokenIn.FindStringSubmatch(sent[1].Body)
		if oldLink == nil || newLink == nil {
			t.Fatalf("both emails need a link:\n%s\n%s", sent[0].Body, sent[1].Body)
		}
		if _, err := f.env.Auth.Activate(f.env.Ctx, oldLink[1], testkit.Password, auth.ClientInfo{}); err == nil {
			t.Fatal("the link of the first email must stop working once the resend went out")
		}
		account, err := f.auth.Activate(f.env.Ctx, newLink[1], testkit.Password, auth.ClientInfo{})
		if err != nil || account != auth.AccountCustomer {
			t.Fatalf("activate: %v (%q)", err, account)
		}
		if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
			t.Fatal(err)
		}
		sent = f.sender.to(email)
		if len(sent) != 3 {
			t.Fatalf("the welcome, the resend and the confirmation, got %d", len(sent))
		}
		if c := sent[2]; c.Subject != "Su cuenta de Grupo Pana está activa" || !strings.Contains(c.Body, greeting) ||
			!strings.Contains(c.HTMLBody, `href="https://portal.test/"`) || strings.Contains(c.Body, "token=") {
			t.Fatalf("a customer's confirmation greets them and opens the customer portal:\n%s\n%s", c.Subject, c.Body)
		}
		var activated bool
		if err := f.env.Pool.QueryRow(f.env.Ctx, `SELECT password_hash IS NOT NULL AND activated_at IS NOT NULL FROM customers WHERE id = $1`, customerID).Scan(&activated); err != nil || !activated {
			t.Fatalf("activated=%v (%v)", activated, err)
		}
		if _, err := f.auth.Activate(f.env.Ctx, newLink[1], testkit.Password, auth.ClientInfo{}); err == nil {
			t.Fatal("the link is single use")
		}
	}
}

func TestTheDispatcherSendsAsSoonAsAQueuedNotificationCommits(t *testing.T) {
	// The periodic look is an hour away, so only the wake-up can explain a prompt delivery.
	f := newAccountFixture(t, nil)
	id, email := f.pendingEmployee(t)

	ctx, cancel := context.WithCancel(f.env.Ctx)
	done := make(chan struct{})
	go func() { f.disp.Run(ctx, time.Hour); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	time.Sleep(200 * time.Millisecond) // let the first run finish

	err := database.WithTx(f.env.Ctx, f.env.Pool, func(tx pgx.Tx) error {
		if err := f.writer.EmployeeActivation(f.env.Ctx, tx, id, email); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond) // still inside the transaction: nothing may be sent yet
		if n := len(f.sender.to(email)); n != 0 {
			t.Errorf("an uncommitted notification was sent (%d)", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the email after the commit", func() bool { return len(f.sender.to(email)) == 1 })
}

func TestARolledBackTransactionNeverWakesOrSends(t *testing.T) {
	f := newAccountFixture(t, nil)
	id, email := f.pendingEmployee(t)
	ctx, cancel := context.WithCancel(f.env.Ctx)
	done := make(chan struct{})
	go func() { f.disp.Run(ctx, time.Hour); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	time.Sleep(200 * time.Millisecond)

	_ = database.WithTx(f.env.Ctx, f.env.Pool, func(tx pgx.Tx) error {
		if err := f.writer.EmployeeActivation(f.env.Ctx, tx, id, email); err != nil {
			return err
		}
		return errors.New("the business action failed")
	})
	time.Sleep(400 * time.Millisecond)
	if n := len(f.sender.to(email)); n != 0 {
		t.Fatalf("nothing was committed, yet %d email(s) went out", n)
	}
}

func TestABurstLargerThanOneBatchDrainsWithoutWaitingForTheNextTick(t *testing.T) {
	f := newAccountFixture(t, func(d *DispatcherDeps) { d.BatchSize = 2 })
	var emails []string
	for i := 0; i < 5; i++ {
		id, email := f.pendingEmployee(t)
		f.queueActivation(t, id, email)
		emails = append(emails, email)
	}
	ctx, cancel := context.WithCancel(f.env.Ctx)
	done := make(chan struct{})
	go func() { f.disp.Run(ctx, time.Hour); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitFor(t, "all five emails", func() bool {
		for _, e := range emails {
			if len(f.sender.to(e)) != 1 {
				return false
			}
		}
		return true
	})
}

func TestAWakerCollapsesManyWakesAndIsSafeWhenNil(t *testing.T) {
	var none *Waker
	none.Wake() // must not panic
	w := NewWaker()
	for i := 0; i < 10; i++ {
		w.Wake() // must not block
	}
	got := 0
	for done := false; !done; {
		select {
		case <-w.signal():
			got++
		default:
			done = true
		}
	}
	if got != 1 {
		t.Fatalf("ten wakes must collapse into one pending signal, got %d", got)
	}
}

func TestACustomerWhoForgotTheirPasswordGetsALinkToTheCustomerPortalThatWorks(t *testing.T) {
	f := newAccountFixture(t, nil)
	admin := f.env.Employee(auth.RoleSystemAdmin, false)
	customerID := f.env.Customer(admin.ID, false)
	hash, err := auth.HashPassword(testkit.Password)
	if err != nil {
		t.Fatal(err)
	}
	var email, docNumber string
	if err := f.env.Pool.QueryRow(f.env.Ctx, `UPDATE customers SET password_hash = $2, activated_at = now() WHERE id = $1
		RETURNING email, document_number`, customerID, hash).Scan(&email, &docNumber); err != nil {
		t.Fatal(err)
	}

	if err := f.auth.CustomerForgotPassword(f.env.Ctx, "DNI", docNumber, auth.ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.disp.RunOnce(f.env.Ctx); err != nil {
		t.Fatal(err)
	}
	sent := f.sender.to(email)
	if len(sent) != 1 {
		t.Fatalf("one reset email, got %d", len(sent))
	}
	mail := sent[0]
	if mail.Subject != "Restablezca su contraseña de Grupo Pana" || !strings.Contains(mail.Body, "Estimada Lucía Quispe,") ||
		!strings.Contains(mail.Body, "https://portal.test/reset-password?token=") || mail.ReplyTo != "" {
		t.Fatalf("a customer's reset email, in usted, with their gender, to the customer portal:\n%s\n%s", mail.Subject, mail.Body)
	}
	if !strings.Contains(mail.HTMLBody, "Crear nueva contraseña") {
		t.Fatal("the HTML version has the button")
	}

	token := tokenIn.FindStringSubmatch(mail.Body)
	if token == nil {
		t.Fatal("no link in the email")
	}
	if err := f.auth.ResetPassword(f.env.Ctx, token[1], "Another-Strong-Pass-7", auth.ClientInfo{}); err != nil {
		t.Fatalf("the emailed link must reset the password: %v", err)
	}
	if _, err := f.auth.CustomerLogin(f.env.Ctx, auth.LoginRequest{DocumentType: "DNI", DocumentNumber: docNumber, Password: "Another-Strong-Pass-7"}, auth.ClientInfo{}); err != nil {
		t.Fatalf("sign in with the new password: %v", err)
	}
}
