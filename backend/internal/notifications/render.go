package notifications

import (
	"fmt"
	"strings"
	"time"
)

// emailData is everything a template may use. It is read at send time, so an email always shows the sale as it is
// now (for example the advisor who holds it now), not as it was when the event happened.
type emailData struct {
	Type          string
	SaleID        string
	SaleCode      string
	GreetingName  string // the recipient's name
	Gender        string // the recipient's gender (M or F)
	CustomerName  string
	Vehicle       string // "Hilux · SR 4x4 · Blanco"
	AdvisorName   string
	AdvisorEmail  string
	AdvisorPhone  string
	AdvisorGender string
	AppBaseURL    string
	PortalBaseURL string
	LogoURL       string      // public address of the header logo; empty falls back to the brand name
	Milestones    []milestone // sale's six customer milestones with their state, read at send time
	LoginID       string      // document the recipient signs in with ("DNI 12345678"), told in welcome emails
	Link          string      // single-use address minted for this send
	LinkValidFor  string      // how long the link lasts, as text ("72 horas")
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

// milestone is one of the six customer milestones of a sale, with its current state.
type milestone struct {
	Number int
	Name   string
	State  string // pending, in_progress or completed
}

// dear, welcome and asesor give the Spanish word that agrees with a person's recorded gender (M or F). Where the
// gender is not known the neutral "(a)" form is used, except for "asesor": advisors always have a gender, and the
// masculine is the generic noun of the role.
func dear(gender string) string {
	switch gender {
	case "M":
		return "Estimado"
	case "F":
		return "Estimada"
	}
	return "Estimado(a)"
}

func welcome(gender string) string {
	switch gender {
	case "M":
		return "Bienvenido"
	case "F":
		return "Bienvenida"
	}
	return "Bienvenido(a)"
}

func asesor(gender string) string {
	if gender == "F" {
		return "asesora"
	}
	return "asesor"
}

func newAdvisor(gender string) string {
	if gender == "F" {
		return "nueva asesora"
	}
	return "nuevo asesor"
}

// portalSale is where a customer opens one purchase in the customer portal.
func (d emailData) portalSale() string { return d.PortalBaseURL + "/sales/" + d.SaleID }

// portalDocuments is where a customer finds the documents of one purchase.
func (d emailData) portalDocuments() string { return d.portalSale() + "/documents" }

// stage describes the customer email sent when a task completes.
type stage struct {
	milestone int // the milestone the task belongs to
	subject   string
	title     string
	text      string
	button    string // the label of the portal button
	documents bool   // the button opens the documents of the purchase instead of the purchase
}

// stages are the seven customer emails sent when a task completes, by notification type.
// The task that triggers each one is in MilestoneTaskType.
var stages = map[string]stage{
	"milestone_task_1": {
		1, "Su cotización oficial está lista para revisión", "Su cotización oficial está lista",
		"La cotización oficial de su vehículo ya está disponible. Ingrese a su portal para revisarla y descargarla en PDF.",
		"Ver mi cotización", true,
	},
	"milestone_task_3": {
		2, "Siguiente paso: su vehículo ha sido asignado", "Su vehículo ya fue asignado",
		"Hemos asignado su vehículo. El siguiente paso es realizar el pago a Grupo Pana: encontrará el detalle, junto con los datos de su vehículo, en su portal.",
		"Ver el siguiente paso", false,
	},
	"milestone_task_6": {
		3, "Sus comprobantes de pago han sido emitidos", "Sus comprobantes ya fueron emitidos",
		"Hemos registrado la emisión de los comprobantes de su compra. Puede consultarlos y descargarlos desde su portal.",
		"Ver mis comprobantes", true,
	},
	"milestone_task_7": {
		4, "Iniciamos los trámites aduaneros de su vehículo", "Iniciamos los trámites aduaneros",
		"Los documentos endosados de su vehículo ya fueron procesados y comenzó su trámite aduanero de nacionalización. Le mantendremos informado de cada avance.",
		"Ver el avance", false,
	},
	"milestone_task_10": {
		5, "¡Su vehículo ya cuenta con placa oficial!", "¡Su vehículo ya tiene placa oficial!",
		"La inscripción de su vehículo fue completada y ya cuenta con su placa definitiva. En su portal encontrará la placa y podrá descargar su TIVe.",
		"Ver mi TIVe", true,
	},
	"milestone_task_11": {
		6, "Su vehículo está en fase de preparación final", "Su vehículo está en preparación final",
		"Su vehículo ya se encuentra en la etapa previa a la entrega, en la que se realiza la inspección y se instalan los accesorios. Ante cualquier consulta, su {asesor} está a su disposición.",
		"Ver el avance", false,
	},
	"milestone_task_13": {
		6, "¡Felicidades por su nuevo vehículo! Gracias por confiar en Grupo Pana", "¡Felicidades por su nuevo vehículo!",
		"Su vehículo ha sido entregado. Gracias por confiar en Grupo Pana. En su portal encontrará los datos de la entrega y el acta firmada.",
		"Ver mi acta de entrega", true,
	},
}

// stageEyebrow names the milestone an email belongs to ("Etapa 3 de 6 · Facturación Oficial").
func stageEyebrow(number int, milestones []milestone) string {
	for _, m := range milestones {
		if m.Number == number {
			return fmt.Sprintf("Etapa %d de %d · %s", number, len(milestones), m.Name)
		}
	}
	return "Novedades de su compra"
}

// rendered is an email ready to send.
type rendered struct {
	Subject        string
	Text           string
	HTML           string
	ReplyToAdvisor bool // set on customer emails so a reply reaches the advisor
}

// render builds the email of a notification type, or reports that the type has no template.
func render(d emailData) (rendered, bool) {
	greeting := dear(d.Gender) + " " + d.GreetingName + ","
	vehicleFact := fact{"Vehículo", d.Vehicle}
	saleFact := fact{"Compra", d.SaleCode}
	advisor := &person{name: d.AdvisorName, email: d.AdvisorEmail, phone: d.AdvisorPhone, gender: d.AdvisorGender}
	userFact := fact{"Su usuario de ingreso", d.LoginID}

	switch {
	case d.Type == EmployeeActivation:
		return content{
			preheader:  "Defina su contraseña para empezar a usar EndosoFlow.",
			eyebrow:    welcome(d.Gender) + " al equipo",
			title:      "Active su cuenta",
			greeting:   "Hola " + d.GreetingName + ",",
			paragraphs: []string{"Se creó su cuenta en EndosoFlow, la plataforma de gestión comercial de Grupo Pana. Para empezar, defina su contraseña con el botón de abajo. Para ingresar usará su documento de identidad y la contraseña que cree."},
			facts:      []fact{userFact},
			button:     &button{"Definir mi contraseña", d.Link},
			showLink:   true,
			notes: []string{
				"Este enlace es de un solo uso y vence en " + d.LinkValidFor + ".",
				"Si no esperaba este mensaje, puede ignorarlo.",
			},
		}.withLogo(d.LogoURL).build("Active su cuenta de EndosoFlow"), true

	case d.Type == EmployeeActivated:
		return content{
			preheader:  "Su cuenta de EndosoFlow ya está activa.",
			eyebrow:    "Cuenta activada",
			title:      "Ya puede ingresar a EndosoFlow",
			greeting:   "Hola " + d.GreetingName + ",",
			paragraphs: []string{"Definió su contraseña y su cuenta quedó activa. A partir de ahora puede ingresar con su documento de identidad y dicha contraseña."},
			facts:      []fact{userFact},
			button:     &button{"Iniciar sesión", d.AppBaseURL + "/"},
			notes:      []string{"Si usted no realizó esta activación, comuníquese de inmediato con el administrador del sistema."},
		}.withLogo(d.LogoURL).build("Su cuenta de EndosoFlow está activa"), true

	case d.Type == EmployeePasswordReset:
		return content{
			preheader:  "Cree una nueva contraseña para su cuenta de EndosoFlow.",
			eyebrow:    "Seguridad de la cuenta",
			title:      "Restablezca su contraseña",
			greeting:   "Hola " + d.GreetingName + ",",
			paragraphs: []string{"Recibimos una solicitud para restablecer la contraseña de su cuenta. Al cambiarla se cerrarán todas sus sesiones abiertas."},
			button:     &button{"Crear nueva contraseña", d.Link},
			showLink:   true,
			notes: []string{
				"Este enlace es de un solo uso y vence en " + d.LinkValidFor + ".",
				"Si usted no lo solicitó, ignore este mensaje: su contraseña no cambiará.",
			},
		}.withLogo(d.LogoURL).build("Restablezca su contraseña de EndosoFlow"), true

	case d.Type == CustomerActivation:
		return content{
			preheader:  "Active su cuenta para seguir el avance de su compra.",
			eyebrow:    welcome(d.Gender) + " a Grupo Pana",
			title:      "Active su cuenta de cliente",
			greeting:   greeting,
			paragraphs: []string{"Hemos registrado sus datos en Grupo Pana. Desde su portal de seguimiento podrá ver el avance de cada una de sus compras. Para ingresar, active su cuenta definiendo su contraseña; su usuario es el documento con el que fue registrado."},
			facts:      []fact{userFact},
			button:     &button{"Activar mi cuenta", d.Link},
			showLink:   true,
			notes: []string{
				"Este enlace es de un solo uso y vence en " + d.LinkValidFor + ".",
				"Si no esperaba este mensaje, puede ignorarlo.",
			},
			toCustomer: true,
		}.withLogo(d.LogoURL).build(welcome(d.Gender) + " a Grupo Pana: acceso a su portal de seguimiento"), true

	case d.Type == CustomerActivated:
		return content{
			preheader:  "Su cuenta de Grupo Pana ya está activa.",
			eyebrow:    "Cuenta activada",
			title:      "Su cuenta está activa",
			greeting:   greeting,
			paragraphs: []string{"Definió su contraseña y su cuenta quedó activa. Desde su portal de seguimiento podrá ver el avance de cada una de sus compras."},
			facts:      []fact{userFact},
			button:     &button{"Ir a mi portal", d.PortalBaseURL + "/"},
			notes:      []string{"Si usted no realizó esta activación, comuníquese de inmediato con Grupo Pana."},
			toCustomer: true,
		}.withLogo(d.LogoURL).build("Su cuenta de Grupo Pana está activa"), true

	case d.Type == CustomerPasswordReset:
		return content{
			preheader:  "Cree una nueva contraseña para su cuenta de Grupo Pana.",
			eyebrow:    "Seguridad de la cuenta",
			title:      "Restablezca su contraseña",
			greeting:   greeting,
			paragraphs: []string{"Recibimos una solicitud para restablecer la contraseña de su cuenta del portal de seguimiento. Al cambiarla se cerrarán todas sus sesiones abiertas."},
			button:     &button{"Crear nueva contraseña", d.Link},
			showLink:   true,
			notes: []string{
				"Este enlace es de un solo uso y vence en " + d.LinkValidFor + ".",
				"Si usted no lo solicitó, ignore este mensaje: su contraseña no cambiará.",
			},
			toCustomer: true,
		}.withLogo(d.LogoURL).build("Restablezca su contraseña de Grupo Pana"), true

	case d.Type == SaleCreated:
		return content{
			preheader:      "Su proceso de compra ha comenzado.",
			eyebrow:        stageEyebrow(1, d.Milestones),
			title:          "Hemos registrado su solicitud",
			greeting:       greeting,
			paragraphs:     []string{"Su proceso de compra ha comenzado. Estos son los datos de su solicitud y de su " + asesor(d.AdvisorGender) + ", quien le acompañará en cada etapa. Puede seguir el avance desde su portal."},
			facts:          []fact{saleFact, vehicleFact},
			contact:        advisor,
			progress:       d.Milestones,
			button:         &button{"Ver mi compra", d.portalSale()},
			toCustomer:     true,
			replyToAdvisor: true,
		}.withLogo(d.LogoURL).build("Hemos registrado su nueva solicitud de vehículo"), true

	case stages[d.Type].title != "":
		st := stages[d.Type]
		st.text = strings.ReplaceAll(st.text, "{asesor}", asesor(d.AdvisorGender))
		url := d.portalSale()
		if st.documents {
			url = d.portalDocuments()
		}
		return content{
			preheader:      st.text,
			eyebrow:        stageEyebrow(st.milestone, d.Milestones),
			title:          st.title,
			greeting:       greeting,
			paragraphs:     []string{st.text},
			facts:          []fact{saleFact, vehicleFact},
			contact:        advisor,
			progress:       d.Milestones,
			button:         &button{st.button, url},
			toCustomer:     true,
			replyToAdvisor: true,
		}.withLogo(d.LogoURL).build(st.subject), true

	case d.Type == AdvisorChanged:
		return content{
			preheader:      "A partir de ahora puede comunicarse con su " + newAdvisor(d.AdvisorGender) + ".",
			eyebrow:        "Cambio de asesor",
			title:          "Su asesor ha cambiado",
			greeting:       greeting,
			paragraphs:     []string{"El asesor a cargo de su compra ha cambiado. A partir de ahora puede comunicarse con:"},
			facts:          []fact{saleFact, vehicleFact},
			contact:        advisor,
			button:         &button{"Ver mi compra", d.portalSale()},
			toCustomer:     true,
			replyToAdvisor: true,
		}.withLogo(d.LogoURL).build("Ha cambiado el asesor de su compra"), true

	case d.Type == SaleAssigned:
		return content{
			preheader:  "Ya puede ver la venta y avanzar sus tareas.",
			eyebrow:    "Nueva venta asignada",
			title:      "Se le asignó la venta " + d.SaleCode,
			greeting:   "Hola " + d.GreetingName + ",",
			paragraphs: []string{"Se le asignó una venta. Ya puede verla y avanzar sus tareas."},
			facts:      []fact{{"Venta", d.SaleCode}, {"Cliente", d.CustomerName}, vehicleFact},
			button:     &button{"Abrir la venta", saleLink(d.AppBaseURL, d.SaleID)},
		}.withLogo(d.LogoURL).build("Se le asignó la venta " + d.SaleCode), true

	case d.Type == SaleUnassigned:
		return content{
			preheader:  "La venta fue reasignada a otro asesor.",
			eyebrow:    "Venta reasignada",
			title:      "La venta " + d.SaleCode + " ya no está asignada a usted",
			greeting:   "Hola " + d.GreetingName + ",",
			paragraphs: []string{"La venta fue reasignada a otro asesor y usted ya no tiene acceso a ella."},
			facts:      []fact{{"Venta", d.SaleCode}, {"Cliente", d.CustomerName}, vehicleFact, {strings.ToUpper(newAdvisor(d.AdvisorGender)[:1]) + newAdvisor(d.AdvisorGender)[1:], d.AdvisorName}},
		}.withLogo(d.LogoURL).build("La venta " + d.SaleCode + " ya no está asignada a usted"), true
	}
	return rendered{}, false
}
