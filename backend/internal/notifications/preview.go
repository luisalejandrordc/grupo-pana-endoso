package notifications

// Preview is one email rendered with sample data, for looking at the templates without sending anything.
type Preview struct {
	Name    string // a file-name friendly label
	Type    string // the notification type
	Subject string
	Text    string
	HTML    string
}

// previewTypes lists every notification type that has a template, in the order they are shown.
var previewTypes = []string{
	EmployeeActivation, EmployeeActivated, PasswordReset, CustomerActivation, CustomerActivated,
	SaleCreated, "milestone_task_1", "milestone_task_3", "milestone_task_6", "milestone_task_7",
	"milestone_task_10", "milestone_task_11", "milestone_task_13",
	CustomerAdvisorChanged, SaleAssigned, SaleUnassigned,
}

// sampleData is a plausible set of values for a notification type.
func sampleData(kind, appBaseURL, portalBaseURL, logoURL string) emailData {
	return emailData{
		Type: kind, AppBaseURL: appBaseURL, PortalBaseURL: portalBaseURL, LogoURL: logoURL,
		SaleID: "7d0c3f6e-2f1a-4c55-9d3b-0a1b2c3d4e5f", SaleCode: "EF-2026-00042",
		GreetingName: "Lucía", Gender: "F", CustomerName: "Lucía Quispe Huamán",
		Vehicle:     "Hilux · SR 4x4 · Blanco Perlado",
		AdvisorName: "Carlos Mendoza", AdvisorEmail: "carlos.mendoza@grupopana.pe", AdvisorPhone: "+51 987 654 321", AdvisorGender: "M",
		Link: appBaseURL + "/activate-account?token=EJEMPLO-de-enlace-de-un-solo-uso", LinkValidFor: "72 horas",
		LoginID: "DNI 45678912", Milestones: sampleMilestones(kind),
	}
}

// sampleMilestones shows the six milestones as they stand right after the task behind each email completes.
func sampleMilestones(kind string) []milestone {
	names := []string{
		"Cotización y Documentación", "Asignación y Pago", "Facturación Oficial",
		"Trámites Aduaneros", "Liberación y Placas", "Preparación y Entrega",
	}
	completed, current := 0, 1 // milestones done, and the one in progress (0 = none)
	switch kind {
	case "milestone_task_3":
		completed, current = 1, 2
	case "milestone_task_6":
		completed, current = 3, 4
	case "milestone_task_7":
		completed, current = 3, 4
	case "milestone_task_10":
		completed, current = 5, 6
	case "milestone_task_11":
		completed, current = 5, 6
	case "milestone_task_13":
		completed, current = 6, 0
	}
	out := make([]milestone, len(names))
	for i, name := range names {
		state := "pending"
		switch {
		case i < completed:
			state = "completed"
		case i+1 == current:
			state = "in_progress"
		}
		out[i] = milestone{Number: i + 1, Name: name, State: state}
	}
	return out
}

// Previews renders every template with sample data. appBaseURL is the frontend's address and logoURL the
// address of the header logo (empty shows the brand name as text).
func Previews(appBaseURL, portalBaseURL, logoURL string) []Preview {
	var out []Preview
	for _, kind := range previewTypes {
		data := sampleData(kind, appBaseURL, portalBaseURL, logoURL)
		if kind == PasswordReset {
			data.Link = appBaseURL + "/reset-password?token=EJEMPLO-de-enlace-de-un-solo-uso"
			data.LinkValidFor = "1 hora"
		}
		if r, ok := render(data); ok {
			out = append(out, Preview{Name: kind, Type: kind, Subject: r.Subject, Text: r.Text, HTML: r.HTML})
		}
	}
	return out
}
