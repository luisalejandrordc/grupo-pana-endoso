package analytics

import "time"

// The response types mirror the Analytics schemas of api/openapi.yaml. Money is decimal text with two
// decimals, durations are hours, ratios run from 0 to 1 and are null when their denominator is zero.

// Meta describes the period every figure of a response covers.
type Meta struct {
	From         string    `json:"from"`
	To           string    `json:"to"`
	PreviousFrom string    `json:"previousFrom"`
	PreviousTo   string    `json:"previousTo"`
	Granularity  string    `json:"granularity"`
	Scope        string    `json:"scope"`
	GeneratedAt  time.Time `json:"generatedAt"`
}

type CountMetric struct {
	Value    int `json:"value"`
	Previous int `json:"previous"`
}

type MoneyMetric struct {
	Value    string `json:"value"`
	Previous string `json:"previous"`
}

type RateMetric struct {
	Value    *float64 `json:"value"`
	Previous *float64 `json:"previous"`
}

type Ratio struct {
	Numerator   int      `json:"numerator"`
	Denominator int      `json:"denominator"`
	Value       *float64 `json:"value"`
}

type RatioMetric struct {
	Numerator   int      `json:"numerator"`
	Denominator int      `json:"denominator"`
	Value       *float64 `json:"value"`
	Previous    *float64 `json:"previous"`
}

type ValueTotal struct {
	Amount    string `json:"amount"`
	SaleCount int    `json:"saleCount"`
}

type ValuedTotal struct {
	Amount         string `json:"amount"`
	SaleCount      int    `json:"saleCount"`
	EstimatedCount int    `json:"estimatedCount"`
}

// --- overview ---

type Overview struct {
	Meta      Meta            `json:"meta"`
	Snapshot  Snapshot        `json:"snapshot"`
	Period    OverviewPeriod  `json:"period"`
	Money     OverviewMoney   `json:"money"`
	Pipeline  []MilestoneLoad `json:"pipeline"`
	Trend     []TrendBucket   `json:"trend"`
	Attention Attention       `json:"attention"`
}

type Snapshot struct {
	ActiveSales int `json:"activeSales"`
	Overdue     int `json:"overdue"`
	Approaching int `json:"approaching"`
	Idle        int `json:"idle"`
}

type OverviewPeriod struct {
	Created   CountMetric `json:"created"`
	Completed CountMetric `json:"completed"`
	Cancelled CountMetric `json:"cancelled"`
	CloseRate RatioMetric `json:"closeRate"`
	CycleTime CycleTime   `json:"cycleTime"`
}

type CycleTime struct {
	MedianHours         *float64 `json:"medianHours"`
	P90Hours            *float64 `json:"p90Hours"`
	PreviousMedianHours *float64 `json:"previousMedianHours"`
	SampleSize          int      `json:"sampleSize"`
}

type OverviewMoney struct {
	PipelineValue     ValuedTotal    `json:"pipelineValue"`
	PendingCollection ValueTotal     `json:"pendingCollection"`
	CompletedValue    MoneyMetric    `json:"completedValue"`
	AverageTicket     NullableMoney  `json:"averageTicket"`
	Collected         MoneyMetric    `json:"collected"`
	Invoiced          MoneyMetric    `json:"invoiced"`
	LostValue         ValuedTotal    `json:"lostValue"`
	Internal          *InternalMoney `json:"internal"`
}

type NullableMoney struct {
	Value    *string `json:"value"`
	Previous *string `json:"previous"`
}

// InternalMoney is only filled for Managers and Admins.
type InternalMoney struct {
	CommissionFinal    MoneyMetric      `json:"commissionFinal"`
	CommissionRate     RateMetric       `json:"commissionRate"`
	CommissionPipeline ValueTotal       `json:"commissionPipeline"`
	CommissionDrift    string           `json:"commissionDrift"`
	PaidToToyota       MoneyMetric      `json:"paidToToyota"`
	AmountMismatches   AmountMismatches `json:"amountMismatches"`
}

type AmountMismatches struct {
	Count              int    `json:"count"`
	AbsoluteDifference string `json:"absoluteDifference"`
}

type MilestoneLoad struct {
	MilestoneNumber int    `json:"milestoneNumber"`
	Name            string `json:"name"`
	Active          int    `json:"active"`
	OnTime          int    `json:"onTime"`
	Approaching     int    `json:"approaching"`
	Overdue         int    `json:"overdue"`
	Idle            int    `json:"idle"`
	Value           string `json:"value"`
}

type TrendBucket struct {
	Bucket    string `json:"bucket"`
	Created   int    `json:"created"`
	Completed int    `json:"completed"`
	Cancelled int    `json:"cancelled"`
}

type Attention struct {
	Total int             `json:"total"`
	Items []AttentionItem `json:"items"`
}

type AttentionItem struct {
	SaleID       string     `json:"saleId"`
	Code         string     `json:"code"`
	CustomerName string     `json:"customerName"`
	Vehicle      string     `json:"vehicle"`
	AdvisorName  string     `json:"advisorName"`
	TaskNumber   int        `json:"taskNumber"`
	TaskName     string     `json:"taskName"`
	Reason       string     `json:"reason"`
	DueDate      *time.Time `json:"dueDate"`
	Hours        float64    `json:"hours"`
	Value        *string    `json:"value"`
}

// --- process ---

type Process struct {
	Meta       Meta           `json:"meta"`
	OnTimeRate RatioMetric    `json:"onTimeRate"`
	Tasks      []TaskTiming   `json:"tasks"`
	Customs    []CustomsGroup `json:"customs"`
}

type TaskTiming struct {
	TaskNumber       int        `json:"taskNumber"`
	Name             string     `json:"name"`
	MilestoneNumber  int        `json:"milestoneNumber"`
	RecommendedHours *int       `json:"recommendedHours"`
	CompletedCount   int        `json:"completedCount"`
	WorkMedianHours  *float64   `json:"workMedianHours"`
	WorkP90Hours     *float64   `json:"workP90Hours"`
	WaitMedianHours  *float64   `json:"waitMedianHours"`
	Deadlines        *Deadlines `json:"deadlines"`
	CurrentCount     int        `json:"currentCount"`
	OverdueCount     int        `json:"overdueCount"`
	ReworkCount      int        `json:"reworkCount"`
}

type Deadlines struct {
	Early            int `json:"early"`
	InApproaching    int `json:"inApproaching"`
	Overdue          int `json:"overdue"`
	RecurringOverdue int `json:"recurringOverdue"`
}

type CustomsGroup struct {
	Channel         string   `json:"channel"`
	Count           int      `json:"count"`
	WorkMedianHours *float64 `json:"workMedianHours"`
}

// --- commercial ---

type Commercial struct {
	Meta             Meta                `json:"meta"`
	Vehicles         []VehicleGroup      `json:"vehicles"`
	CustomerTypes    []CustomerTypeGroup `json:"customerTypes"`
	Temperatures     []TemperatureGroup  `json:"temperatures"`
	Cancellations    Cancellations       `json:"cancellations"`
	ValueTrend       []ValueBucket       `json:"valueTrend"`
	PriceComposition *PriceComposition   `json:"priceComposition"`
	InvoicedByKind   []InvoicedKind      `json:"invoicedByKind"`
}

type VehicleGroup struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	Created       int     `json:"created"`
	Completed     int     `json:"completed"`
	Cancelled     int     `json:"cancelled"`
	Active        int     `json:"active"`
	QuotedValue   string  `json:"quotedValue"`
	AverageTicket *string `json:"averageTicket"`
	Commission    *string `json:"commission"`
}

type CustomerTypeGroup struct {
	CustomerType string  `json:"customerType"`
	Created      int     `json:"created"`
	Completed    int     `json:"completed"`
	Cancelled    int     `json:"cancelled"`
	Active       int     `json:"active"`
	CloseRate    Ratio   `json:"closeRate"`
	QuotedValue  string  `json:"quotedValue"`
	Commission   *string `json:"commission"`
}

type TemperatureGroup struct {
	Temperature string `json:"temperature"`
	Created     int    `json:"created"`
	Completed   int    `json:"completed"`
	Cancelled   int    `json:"cancelled"`
	InProgress  int    `json:"inProgress"`
	CloseRate   Ratio  `json:"closeRate"`
}

type Cancellations struct {
	Total      ValuedTotal          `json:"total"`
	ByCategory []CancellationReason `json:"byCategory"`
	ByTask     []CancellationTask   `json:"byTask"`
}

type CancellationReason struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Value    string `json:"value"`
}

type CancellationTask struct {
	TaskNumber int    `json:"taskNumber"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
}

type ValueBucket struct {
	Bucket         string  `json:"bucket"`
	CreatedValue   string  `json:"createdValue"`
	CompletedValue string  `json:"completedValue"`
	Commission     *string `json:"commission"`
}

type PriceComposition struct {
	Toyota            string `json:"toyota"`
	Commission        string `json:"commission"`
	Accessories       string `json:"accessories"`
	ReleaseProcessing string `json:"releaseProcessing"`
	SaleCount         int    `json:"saleCount"`
}

type InvoicedKind struct {
	Kind   string `json:"kind"`
	Count  int    `json:"count"`
	Amount string `json:"amount"`
}

// --- team ---

type Team struct {
	Meta     Meta                 `json:"meta"`
	Advisors []AdvisorPerformance `json:"advisors"`
}

type AdvisorPerformance struct {
	AdvisorID        string   `json:"advisorId"`
	Name             string   `json:"name"`
	IsActive         bool     `json:"isActive"`
	Active           int      `json:"active"`
	Overdue          int      `json:"overdue"`
	Idle             int      `json:"idle"`
	Created          int      `json:"created"`
	Completed        int      `json:"completed"`
	Cancelled        int      `json:"cancelled"`
	CloseRate        Ratio    `json:"closeRate"`
	CycleMedianHours *float64 `json:"cycleMedianHours"`
	OnTimeRate       Ratio    `json:"onTimeRate"`
	PipelineValue    string   `json:"pipelineValue"`
	CompletedValue   string   `json:"completedValue"`
	Commission       string   `json:"commission"`
}

// --- system ---

type System struct {
	Meta          Meta                `json:"meta"`
	Notifications SystemNotifications `json:"notifications"`
	Security      SystemSecurity      `json:"security"`
	Accounts      SystemAccounts      `json:"accounts"`
	Storage       SystemStorage       `json:"storage"`
}

type SystemNotifications struct {
	Pending      int            `json:"pending"`
	Sent         int            `json:"sent"`
	Failed       int            `json:"failed"`
	Skipped      int            `json:"skipped"`
	FailedByType []FailedByType `json:"failedByType"`
}

type FailedByType struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type SystemSecurity struct {
	SuccessfulSignIns int `json:"successfulSignIns"`
	FailedSignIns     int `json:"failedSignIns"`
	LockedAccounts    int `json:"lockedAccounts"`
}

type SystemAccounts struct {
	ActiveEmployees       int `json:"activeEmployees"`
	NotActivatedEmployees int `json:"notActivatedEmployees"`
}

type SystemStorage struct {
	PendingDeletions int `json:"pendingDeletions"`
}
