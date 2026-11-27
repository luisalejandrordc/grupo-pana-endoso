// Package dashboard answers the questions of the BI dashboard. It is read-only: it never writes, it only
// aggregates what the sales, tasks, payments and invoices modules already recorded, in the database.
//
// Two kinds of numbers live here, and the screens label them differently:
//   - Flow numbers (sales created, completed or cancelled, task timings, money moved) follow the period the
//     caller picked, using the date of the event itself.
//   - Snapshot numbers (active sales, tasks overdue now, money held in active sales) describe today and
//     ignore the period. Both kinds respect the sale filters (advisor, customer type, model, temperature).
//
// Amounts are USD strings with two decimals, like everywhere else in the API. They are recorded operational
// values: a value only counts once the task that confirms it is completed, and none of them is recognized
// revenue or an accounting figure.
package dashboard

import (
	"time"

	"github.com/google/uuid"
)

// Filter is what the caller asks for. Empty fields mean "no filter" (the period has defaults).
type Filter struct {
	// From and To are calendar dates (YYYY-MM-DD) in Lima time, both included. Empty means the last 90 days.
	From, To     string
	AdvisorID    *uuid.UUID
	CustomerType string
	Temperature  string
	ModelID      *uuid.UUID
	VersionID    *uuid.UUID
}

// Period echoes the dates that were actually used.
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// --- summary ---

// Summary is the first tab: how the business stands and what needs attention.
type Summary struct {
	Period      Period          `json:"period"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Kpis        SummaryKpis     `json:"kpis"`
	Trend       []TrendPoint    `json:"trend"`
	Milestones  []MilestoneLoad `json:"milestones"`
	Attention   AttentionList   `json:"attention"`
}

// SummaryKpis holds the headline numbers. The active sales and the three task counts are snapshots of today.
type SummaryKpis struct {
	ActiveSales       int `json:"activeSales"`
	CreatedInPeriod   int `json:"createdInPeriod"`
	CompletedInPeriod int `json:"completedInPeriod"`
	CancelledInPeriod int `json:"cancelledInPeriod"`
	// ClosingRate is completed / (completed + cancelled) in the period, as a percentage with one decimal.
	// It is null when nothing was completed or cancelled.
	ClosingRate      *float64 `json:"closingRate"`
	OverdueTasks     int      `json:"overdueTasks"`
	ApproachingTasks int      `json:"approachingTasks"`
	StalledTasks     int      `json:"stalledTasks"`
}

// TrendPoint is one month (Lima time) of the created, completed and cancelled sales.
type TrendPoint struct {
	Month     string `json:"month"` // YYYY-MM
	Created   int    `json:"created"`
	Completed int    `json:"completed"`
	Cancelled int    `json:"cancelled"`
}

// MilestoneLoad is how many active sales are in a milestone right now.
type MilestoneLoad struct {
	Number      int    `json:"number"`
	Name        string `json:"name"`
	ActiveSales int    `json:"activeSales"`
}

// Attention kinds.
const (
	KindOverdue     = "overdue"     // the task in progress is past its due date
	KindApproaching = "approaching" // the task in progress used 80% of its time and is not yet late
	KindStalled     = "stalled"     // the next task could start and nobody started it
)

// AttentionList is the sales that need someone today, the most urgent first. Total counts all of them; Items
// holds the first ones.
type AttentionList struct {
	Total int             `json:"total"`
	Items []AttentionItem `json:"items"`
}

// AttentionItem is one sale that needs attention.
type AttentionItem struct {
	SaleID       uuid.UUID  `json:"saleId"`
	SaleCode     string     `json:"saleCode"`
	CustomerName string     `json:"customerName"`
	Vehicle      string     `json:"vehicle"`
	AdvisorID    uuid.UUID  `json:"advisorId"`
	AdvisorName  string     `json:"advisorName"`
	TaskNumber   int        `json:"taskNumber"`
	TaskName     string     `json:"taskName"`
	Kind         string     `json:"kind"`
	DueDate      *time.Time `json:"dueDate"`
	// WaitingSince is when a stalled task became eligible to start; null for the other kinds.
	WaitingSince *time.Time `json:"waitingSince"`
}

// --- process ---

// Process is the tab about how the work flows: timings, bottlenecks, SLA compliance and cancellations.
type Process struct {
	Period        Period        `json:"period"`
	GeneratedAt   time.Time     `json:"generatedAt"`
	CycleTime     CycleTime     `json:"cycleTime"`
	ReopenedTasks int           `json:"reopenedTasks"`
	Tasks         []TaskStats   `json:"tasks"`
	Cancellations Cancellations `json:"cancellations"`
}

// CycleTime is how long a whole operation took, from creating the sale to completing it, for the sales
// completed in the period.
type CycleTime struct {
	SampleSize int      `json:"sampleSize"`
	MedianDays *float64 `json:"medianDays"`
	P90Days    *float64 `json:"p90Days"`
}

// TaskStats is one of the 13 tasks. ActiveSales is a snapshot; the rest follow the period (tasks completed in it).
type TaskStats struct {
	Number            int    `json:"number"`
	Name              string `json:"name"`
	Milestone         int    `json:"milestone"`
	RecommendedHours  *int   `json:"recommendedHours"` // null when the task has no SLA
	ActiveSales       int    `json:"activeSales"`
	CompletedInPeriod int    `json:"completedInPeriod"`
	// MedianWorkHours is started to completed; MedianWaitHours is eligible to started (time spent waiting for
	// someone to begin).
	MedianWorkHours *float64 `json:"medianWorkHours"`
	MedianWaitHours *float64 `json:"medianWaitHours"`
	// OnTime counts completions in the early or approaching phase, Late the overdue ones. Tasks without an
	// SLA have neither.
	OnTime     int      `json:"onTime"`
	Late       int      `json:"late"`
	OnTimeRate *float64 `json:"onTimeRate"`
}

// Cancellations are the sales cancelled in the period.
type Cancellations struct {
	Total      int             `json:"total"`
	ByCategory []CategoryCount `json:"byCategory"`
	ByTask     []TaskCount     `json:"byTask"`
}

// CategoryCount is cancellations of one reason.
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// TaskCount is cancellations that happened while a given task was the current one.
type TaskCount struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// --- market ---

// Market is the tab about what is being sold and to whom. It looks at the sales created in the period.
type Market struct {
	Period        Period             `json:"period"`
	GeneratedAt   time.Time          `json:"generatedAt"`
	Models        []MixItem          `json:"models"`
	Versions      []MixItem          `json:"versions"`
	Colors        []MixItem          `json:"colors"`
	CustomerTypes []MixItem          `json:"customerTypes"`
	Temperatures  []TemperatureStats `json:"temperatures"`
}

// MixItem is a group of sales (one model, one color...) with how they stand today.
type MixItem struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Total     int    `json:"total"`
	Active    int    `json:"active"`
	Completed int    `json:"completed"`
	Cancelled int    `json:"cancelled"`
}

// TemperatureStats is how sales of a temperature ended up. ClosingRate is completed / (completed + cancelled)
// among the sales created in the period that are already resolved.
type TemperatureStats struct {
	Temperature string   `json:"temperature"`
	Total       int      `json:"total"`
	Active      int      `json:"active"`
	Completed   int      `json:"completed"`
	Cancelled   int      `json:"cancelled"`
	ClosingRate *float64 `json:"closingRate"`
}

// --- finance ---

// Finance is the tab about recorded amounts.
type Finance struct {
	Period      Period         `json:"period"`
	GeneratedAt time.Time      `json:"generatedAt"`
	Pipeline    PipelineMoney  `json:"pipeline"`
	Completed   CompletedMoney `json:"completed"`
	Cash        CashFlow       `json:"cash"`
	Invoices    []InvoiceTotal `json:"invoices"`
	Monthly     []MoneyMonth   `json:"monthly"`
}

// PipelineMoney is the money of the active sales whose quotation (task 1) is completed. A snapshot of today.
// The commission is final once task 3 is completed and provisional before.
type PipelineMoney struct {
	Sales                 int    `json:"sales"`
	QuotedValue           string `json:"quotedValue"`
	FinalCommission       string `json:"finalCommission"`
	ProvisionalCommission string `json:"provisionalCommission"`
}

// CompletedMoney is the money of the sales completed in the period.
type CompletedMoney struct {
	Sales             int    `json:"sales"`
	QuotedValue       string `json:"quotedValue"`
	Commission        string `json:"commission"`
	AverageCommission string `json:"averageCommission"`
	Accessories       string `json:"accessories"`
	ReleaseProcessing string `json:"releaseProcessing"`
	ToyotaAmount      string `json:"toyotaAmount"`
}

// CashFlow is the money recorded as received from customers (task 4) and paid to Toyota (task 5). MoneyIn,
// MoneyOut, Net and PaymentMismatches follow the period (by payment date); the two "held" amounts are snapshots.
type CashFlow struct {
	MoneyIn           string `json:"moneyIn"`
	MoneyOut          string `json:"moneyOut"`
	Net               string `json:"net"`
	PaymentMismatches int    `json:"paymentMismatches"`
	HeldInActiveSales string `json:"heldInActiveSales"`
	InCancelledSales  string `json:"inCancelledSales"`
}

// InvoiceTotal is the official invoices of one kind dated in the period, once task 6 is completed.
type InvoiceTotal struct {
	Kind       string `json:"kind"`
	Count      int    `json:"count"`
	Amount     string `json:"amount"`
	Mismatches int    `json:"mismatches"` // invoices whose amount differs from the expected one
}

// MoneyMonth is one month of commission (sales completed that month) and of money in and out (by payment date).
type MoneyMonth struct {
	Month      string `json:"month"`
	Commission string `json:"commission"`
	MoneyIn    string `json:"moneyIn"`
	MoneyOut   string `json:"moneyOut"`
}

// --- advisors ---

// Advisors is the tab that compares the sales team. Only Managers and Admins can read it.
type Advisors struct {
	Period      Period       `json:"period"`
	GeneratedAt time.Time    `json:"generatedAt"`
	Items       []AdvisorRow `json:"items"`
}

// AdvisorRow is one advisor.
type AdvisorRow struct {
	AdvisorID         uuid.UUID `json:"advisorId"`
	Name              string    `json:"name"`
	IsActive          bool      `json:"isActive"`
	HasPhoto          bool      `json:"hasPhoto"`
	ActiveSales       int       `json:"activeSales"`
	CreatedInPeriod   int       `json:"createdInPeriod"`
	CompletedInPeriod int       `json:"completedInPeriod"`
	CancelledInPeriod int       `json:"cancelledInPeriod"`
	ClosingRate       *float64  `json:"closingRate"`
	MedianCycleDays   *float64  `json:"medianCycleDays"`
	OverdueTasks      int       `json:"overdueTasks"`
	PipelineValue     string    `json:"pipelineValue"`
	Commission        string    `json:"commission"`
}

// --- system ---

// System is the Admin-only tab about the platform itself. It ignores the sale filters.
type System struct {
	Period        Period            `json:"period"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	Staff         StaffStats        `json:"staff"`
	Notifications NotificationStats `json:"notifications"`
	Security      SecurityStats     `json:"security"`
}

// StaffStats counts the employees (the system actor is not one).
type StaffStats struct {
	Admins    int `json:"admins"`
	Managers  int `json:"managers"`
	Advisors  int `json:"advisors"`
	Inactive  int `json:"inactive"`
	LockedNow int `json:"lockedNow"`
}

// NotificationStats is the email outbox. PendingNow is a snapshot; the rest count notifications queued in the period.
type NotificationStats struct {
	PendingNow      int        `json:"pendingNow"`
	OldestPendingAt *time.Time `json:"oldestPendingAt"`
	Sent            int        `json:"sent"`
	Failed          int        `json:"failed"`
	Skipped         int        `json:"skipped"`
}

// SecurityStats counts employee sign-in events in the period.
type SecurityStats struct {
	SuccessfulLogins int `json:"successfulLogins"`
	FailedLogins     int `json:"failedLogins"`
	LockedAttempts   int `json:"lockedAttempts"`
	PasswordResets   int `json:"passwordResets"`
}
