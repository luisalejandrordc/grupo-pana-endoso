package notifications

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// emailData is everything a template may use. It is read at send time, so an email always shows the sale as it is
// now (for example the advisor who holds it now), not as it was when the event happened.
type emailData struct {
	Type         string
	SaleID       string
	SaleCode     string
	GreetingName string // the recipient's name
	CustomerName string
	Vehicle      string // "Hilux · SR 4x4 · Blanco"
	AdvisorName  string
	AdvisorEmail string
	AdvisorPhone string
	AppBaseURL   string
}

// brand is the sender name shown in every email.
const brand = "EndosoFlow | Grupo Pana"

// SaleCode builds the human-friendly sale number: the year the sale was created (Lima time) and its sequence
// number. The sales module builds the same text for its own responses.
func SaleCode(createdAt time.Time, number int64) string {
	lima := time.FixedZone("America/Lima", -5*60*60)
	return fmt.Sprintf("EF-%d-%05d", createdAt.In(lima).Year(), number)
}

// saleLink is where an employee lands to open a sale in the application.
func saleLink(baseURL, saleID string) string { return baseURL + "/sales/" + saleID }

// milestoneMessage is what each customer milestone email says.
var milestoneMessage = map[string]struct{ subject, text string }{
	"milestone_task_1":  {"su cotización está lista", "La cotización de su vehículo está lista."},
	"milestone_task_3":  {"liquidación de su unidad completada", "Hemos completado la liquidación de su unidad."},
	"milestone_task_6":  {"su factura fue emitida", "La factura de su compra ha sido emitida."},
	"milestone_task_7":  {"sus documentos endosados están listos", "Los documentos endosados de su vehículo están listos."},
	"milestone_task_10": {"inmatriculación y placas completadas", "La inmatriculación de su vehículo y sus placas han sido completadas."},
	"milestone_task_11": {"su vehículo está en el punto de entrega", "Su vehículo ha sido trasladado al punto de entrega."},
	"milestone_task_13": {"su vehículo fue entregado", "¡Su vehículo ha sido entregado! Gracias por confiar en Grupo Pana."},
}

// rendered is an email ready to send.
type rendered struct {
	Subject string
	Text    string
	HTML    string
	// ReplyToAdvisor is set on customer emails so a reply reaches the advisor, not the shared sending account.
	ReplyToAdvisor bool
}

type block struct {
	paragraphs []string
	facts      [][2]string
	link       [2]string // label, address
}

// render builds the email of a notification type, or reports that the type has no template.
func render(d emailData) (rendered, bool) {
	greeting := "Estimado(a) " + d.GreetingName + ","
	vehicleFact := [2]string{"Vehículo", d.Vehicle}
	saleFact := [2]string{"Venta", d.SaleCode}
	advisorFacts := [][2]string{{"Su asesor", d.AdvisorName}, {"Correo del asesor", d.AdvisorEmail}, {"Teléfono del asesor", d.AdvisorPhone}}

	switch {
	case d.Type == SaleCreated:
		return layout(d, "Su compra "+d.SaleCode+" ha sido registrada", greeting, true, block{
			paragraphs: []string{"Hemos registrado su compra. Su asesor le acompañará en cada etapa del proceso y le avisaremos cuando se complete cada hito."},
			facts:      append([][2]string{saleFact, vehicleFact}, advisorFacts...),
		}), true
	case milestoneMessage[d.Type].text != "":
		m := milestoneMessage[d.Type]
		return layout(d, d.SaleCode+": "+m.subject, greeting, true, block{
			paragraphs: []string{m.text, "Si tiene alguna consulta, responda a este correo y le llegará directamente a su asesor."},
			facts:      [][2]string{saleFact, vehicleFact, {"Su asesor", d.AdvisorName}},
		}), true
	case d.Type == CustomerAdvisorChanged:
		return layout(d, d.SaleCode+": su asesor ha cambiado", greeting, true, block{
			paragraphs: []string{"El asesor a cargo de su compra ha cambiado. A partir de ahora puede comunicarse con:"},
			facts:      append([][2]string{saleFact, vehicleFact}, advisorFacts...),
		}), true
	case d.Type == SaleAssigned:
		return layout(d, "Se le asignó la venta "+d.SaleCode, "Hola "+d.GreetingName+",", false, block{
			paragraphs: []string{"Se le asignó una venta. Ya puede verla y avanzar sus tareas."},
			facts:      [][2]string{saleFact, {"Cliente", d.CustomerName}, vehicleFact},
			link:       [2]string{"Abrir la venta", saleLink(d.AppBaseURL, d.SaleID)},
		}), true
	case d.Type == SaleUnassigned:
		return layout(d, "La venta "+d.SaleCode+" ya no está asignada a usted", "Hola "+d.GreetingName+",", false, block{
			paragraphs: []string{"La venta fue reasignada a otro asesor y usted ya no tiene acceso a ella."},
			facts:      [][2]string{saleFact, {"Cliente", d.CustomerName}, vehicleFact, {"Nuevo asesor", d.AdvisorName}},
		}), true
	}
	return rendered{}, false
}

func layout(d emailData, subject, greeting string, toCustomer bool, b block) rendered {
	var text strings.Builder
	var page strings.Builder
	text.WriteString(greeting + "\n\n")
	fmt.Fprintf(&page, `<div style="font-family:Arial,Helvetica,sans-serif;max-width:560px;margin:0 auto;color:#1f2937">`+
		`<h2 style="margin:0 0 16px;font-size:18px">%s</h2><p>%s</p>`, html.EscapeString(brand), html.EscapeString(greeting))
	for _, p := range b.paragraphs {
		text.WriteString(p + "\n\n")
		fmt.Fprintf(&page, "<p>%s</p>", html.EscapeString(p))
	}
	var rows strings.Builder
	for _, f := range b.facts {
		if strings.TrimSpace(f[1]) == "" {
			continue
		}
		text.WriteString(f[0] + ": " + f[1] + "\n")
		fmt.Fprintf(&rows, `<tr><td style="padding:4px 12px 4px 0;color:#6b7280">%s</td><td style="padding:4px 0"><strong>%s</strong></td></tr>`,
			html.EscapeString(f[0]), html.EscapeString(f[1]))
	}
	if rows.Len() > 0 {
		text.WriteString("\n")
		page.WriteString(`<table style="border-collapse:collapse;margin:8px 0 16px">` + rows.String() + `</table>`)
	}
	if b.link[1] != "" {
		text.WriteString(b.link[0] + ": " + b.link[1] + "\n\n")
		fmt.Fprintf(&page, `<p><a href="%s" style="color:#c8102e">%s</a></p>`, html.EscapeString(b.link[1]), html.EscapeString(b.link[0]))
	}
	text.WriteString("— " + brand + "\n")
	page.WriteString(`<p style="color:#6b7280;font-size:12px;margin-top:24px">` + html.EscapeString("— "+brand) + `</p></div>`)
	return rendered{Subject: subject, Text: text.String(), HTML: page.String(), ReplyToAdvisor: toCustomer}
}
