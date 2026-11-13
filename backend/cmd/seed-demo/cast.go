package main

import (
	"endosoflow/internal/customers"
	"time"
)

// The demo cast. Everything here is invented; the document numbers use ranges that real people do not have in
// these fields (a 9000... DNI, a 2060... RUC) so a demo database never collides with real data by accident.

const demoEmailDomain = "demo.endosoflow.test"

type demoEmployee struct {
	role, first, last, gender, docNumber, email, phone string
}

var demoEmployees = []demoEmployee{
	{"sales_manager", "Carolina", "Vega Torres", "F", "90000001", "gerencia@" + demoEmailDomain, "999100001"},
	{"sales_advisor", "Marco", "Salazar Ruiz", "M", "90000002", "marco@" + demoEmailDomain, "999100002"},
	{"sales_advisor", "Daniela", "Ríos Campos", "F", "90000003", "daniela@" + demoEmailDomain, "999100003"},
	{"sales_advisor", "Luis", "Paredes Soto", "M", "90000004", "luis@" + demoEmailDomain, "999100004"},
}

// The index of the manager and of the three advisors in demoEmployees.
const managerIndex = 0

type demoModel struct {
	name        string
	description string
	categories  []string
	versions    []demoVersion
	colors      []string
}

type demoVersion struct {
	name  string
	price string
}

var demoModels = []demoModel{
	{
		name: "Hilux Demo", description: "Pickup de trabajo y de uso diario.",
		categories: []string{"camioneta"},
		versions:   []demoVersion{{"SR 4x2", "35000"}, {"SRV 4x4", "42500"}},
		colors:     []string{"Blanco", "Gris Plata", "Negro"},
	},
	{
		name: "Prado Demo", description: "SUV de lujo con tracción integral.",
		categories: []string{"familiar"},
		versions:   []demoVersion{{"TX", "58900"}, {"VX", "72300"}},
		colors:     []string{"Blanco Perla", "Negro"},
	},
	{
		name: "Hiace Demo", description: "Van para transporte de personal.",
		categories: []string{"comercial"},
		versions:   []demoVersion{{"Wagon 15", "49800"}},
		colors:     []string{"Blanco", "Plata"},
	},
}

type demoCustomer struct {
	in customers.CreateInput
}

func person(docType, docNumber, first, last, gender, nationality, phone string) demoCustomer {
	return demoCustomer{customers.CreateInput{
		CustomerType: customers.TypeDiplomat, DocumentType: docType, DocumentNumber: docNumber,
		FirstName: first, LastName: last, Gender: gender, Nationality: nationality, Phone: phone,
		Email: "cliente." + docNumber + "@" + demoEmailDomain,
	}}
}

func organization(kind, ruc, legalName, nationality, phone, contactFirst, contactLast, jobTitle string) demoCustomer {
	return demoCustomer{customers.CreateInput{
		CustomerType: kind, DocumentType: "RUC", DocumentNumber: ruc, LegalName: legalName, Nationality: nationality,
		Phone: phone, Email: "contacto." + ruc + "@" + demoEmailDomain,
		ContactFirstName: contactFirst, ContactLastName: contactLast, ContactJobTitle: jobTitle,
	}}
}

var demoCustomers = []demoCustomer{
	organization(customers.TypeEmbassy, "20601000011", "Embajada del Japón (demo)", "Japonesa", "014440011", "Hiro", "Tanaka", "Agregado administrativo"),
	person("CE", "900000012", "Anne", "Dubois", "F", "Francesa", "999200012"),
	organization(customers.TypeCatholicEntity, "20601000013", "Congregación Santa Rosa (demo)", "Peruana", "014440013", "Padre Julián", "Mendoza", "Ecónomo"),
	person("CDI", "CDI9000014", "Ahmed", "Al Mansour", "M", "Emiratí", "999200014"),
	organization(customers.TypeInternationalOrg, "20601000015", "Programa Regional de Salud (demo)", "Suiza", "014440015", "Karin", "Meier", "Jefa de logística"),
	organization(customers.TypeEmbassy, "20601000016", "Embajada de Canadá (demo)", "Canadiense", "014440016", "Michael", "Roberts", "Oficial de administración"),
	person("DNI", "90000017", "Rosa", "Huamán Cruz", "F", "Peruana", "999200017"),
	organization(customers.TypeOtherReligious, "20601000018", "Iglesia Evangélica Luz del Sur (demo)", "Peruana", "014440018", "Pastor Elías", "Quiroga", "Administrador"),
	person("PAS", "PAS9000019", "James", "Whitaker", "M", "Británica", "999200019"),
	organization(customers.TypeInternationalOrg, "20601000020", "Fundación Andina de Cooperación (demo)", "Española", "014440020", "Lucía", "Ferrer", "Coordinadora"),
	organization(customers.TypeEmbassy, "20601000021", "Embajada de Corea (demo)", "Coreana", "014440021", "Min-jun", "Park", "Segundo secretario"),
	person("CE", "900000022", "Sofía", "Almeida Reyes", "F", "Brasileña", "999200022"),
}

// cancelPlan cancels a sale after the last thing it does.
type cancelPlan struct {
	by       string // "advisor", "manager" or "admin"
	category string
	text     string
}

// scenario is one sale and how far its story goes. "Through" tasks are completed in order, with realistic
// waits; "current" is a task left in progress.
type scenario struct {
	customer, model, version, color int
	advisor                         int // index in demoEmployees (1 to 3)
	temperature                     string
	startedDaysAgo                  float64
	through                         int
	current                         int           // 0 for none
	currentStartedAgo               time.Duration // how long the current task has been open
	partial                         bool          // the current task has some of its data and files
	restart                         int           // a task that was opened by mistake and restarted once
	correctFirst                    bool          // a manager corrects the quotation after Task 1
	cancel                          *cancelPlan
	notes                           string
}

var scenarios = []scenario{
	{customer: 0, model: 0, version: 1, color: 0, advisor: 1, temperature: "hot", startedDaysAgo: 1, through: 0, notes: "Primera cotización pedida por la agregaduría."},
	{customer: 1, model: 0, version: 0, color: 1, advisor: 2, temperature: "warm", startedDaysAgo: 2, through: 0, current: 1, currentStartedAgo: 20 * time.Hour, partial: true},
	{customer: 2, model: 2, version: 0, color: 0, advisor: 3, temperature: "warm", startedDaysAgo: 4, through: 1, current: 2, currentStartedAgo: 30 * time.Hour, partial: true},
	{customer: 3, model: 1, version: 1, color: 1, advisor: 1, temperature: "hot", startedDaysAgo: 9, through: 1, current: 2, currentStartedAgo: 6 * 24 * time.Hour, partial: true, notes: "Falta la copia del pasaporte diplomático; el cliente viaja esta semana."},
	{customer: 4, model: 1, version: 0, color: 0, advisor: 2, temperature: "cold", startedDaysAgo: 12, through: 3, correctFirst: true},
	{customer: 5, model: 0, version: 1, color: 2, advisor: 3, temperature: "hot", startedDaysAgo: 16, through: 5, current: 6, currentStartedAgo: 8 * time.Hour, partial: true},
	{customer: 6, model: 0, version: 0, color: 0, advisor: 1, temperature: "warm", startedDaysAgo: 22, through: 6, restart: 2},
	{customer: 7, model: 2, version: 0, color: 1, advisor: 2, temperature: "warm", startedDaysAgo: 28, through: 9, current: 10, currentStartedAgo: 14 * time.Hour, partial: true},
	{customer: 8, model: 1, version: 0, color: 1, advisor: 3, temperature: "hot", startedDaysAgo: 36, through: 12},
	{customer: 9, model: 0, version: 0, color: 0, advisor: 1, temperature: "warm", startedDaysAgo: 48, through: 13},
	{customer: 10, model: 1, version: 1, color: 0, advisor: 2, temperature: "hot", startedDaysAgo: 40, through: 13, correctFirst: true},
	{
		customer: 11, model: 0, version: 0, color: 2, advisor: 3, temperature: "cold", startedDaysAgo: 14, through: 1, current: 2, currentStartedAgo: 5 * time.Hour, partial: true,
		cancel: &cancelPlan{by: "advisor", category: "customer_withdrew", text: "El cliente decidió esperar al próximo trimestre."},
	},
	{
		customer: 4, model: 1, version: 0, color: 1, advisor: 1, temperature: "warm", startedDaysAgo: 26, through: 5,
		cancel: &cancelPlan{by: "manager", category: "payment_or_financing_failed", text: "La transferencia internacional fue rechazada por el banco emisor."},
	},
}
