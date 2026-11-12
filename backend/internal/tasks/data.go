package tasks

// The data of each task. The same struct is used to read (null where nothing is saved yet) and to write: on a
// write, a nil field is left alone, an empty string clears the field, and any other value sets it. Money is a
// decimal string, dates are "YYYY-MM-DD", and the enum fields take the values listed in the OpenAPI contract.

// Task1Data: the initial quotation amounts.
type Task1Data struct {
	InitialToyotaAmount     *string `json:"initialToyotaAmount"`
	InitialCommission       *string `json:"initialCommission"`
	AccessoryAmount         *string `json:"accessoryAmount"`
	ReleaseProcessingAmount *string `json:"releaseProcessingAmount"`
}

// Task2Data: the person who signs and the due-diligence classification.
type Task2Data struct {
	SignerDocumentType   *string `json:"signerDocumentType"`
	SignerDocumentNumber *string `json:"signerDocumentNumber"`
	SignerFirstName      *string `json:"signerFirstName"`
	SignerLastName       *string `json:"signerLastName"`
	SignerNameSource     *string `json:"signerNameSource"`
	PepStatus            *string `json:"pepStatus"`
	DDCRegime            *string `json:"ddcRegime"`
}

// Task3Data: the unit that was assigned and the final amount Toyota charges.
type Task3Data struct {
	VehicleSeries     *string `json:"vehicleSeries"`
	VIN               *string `json:"vin"`
	FinalToyotaAmount *string `json:"finalToyotaAmount"`
}

// PaymentData is one bank deposit (task 4: from the customer to Pana; task 5: from Pana to Toyota).
type PaymentData struct {
	Bank              *string `json:"bank"`
	PaymentDate       *string `json:"paymentDate"`
	TransactionNumber *string `json:"transactionNumber"`
	DepositedAmount   *string `json:"depositedAmount"`
	MismatchReason    *string `json:"mismatchReason"`
}

// InvoiceData is one invoice of task 6.
type InvoiceData struct {
	Number         *string `json:"number"`
	Date           *string `json:"date"`
	Amount         *string `json:"amount"`
	MismatchReason *string `json:"mismatchReason"`
}

// Task6Data: the four invoices.
type Task6Data struct {
	Toyota            *InvoiceData `json:"toyota"`
	Commission        *InvoiceData `json:"commission"`
	Accessories       *InvoiceData `json:"accessories"`
	ReleaseProcessing *InvoiceData `json:"releaseProcessing"`
}

// Task7Data has no fields: the task is the documents.
type Task7Data struct{}

// Task8Data: customs.
type Task8Data struct {
	DAMNumber      *string `json:"damNumber"`
	CustomsOffice  *string `json:"customsOffice"`
	ControlChannel *string `json:"controlChannel"`
}

// Task9Data: the release process.
type Task9Data struct {
	PersonInCharge *string `json:"personInCharge"`
}

// Task10Data: registration.
type Task10Data struct {
	LicensePlate     *string `json:"licensePlate"`
	RegistrationDate *string `json:"registrationDate"`
}

// Task11Data: the transfer to the PDS.
type Task11Data struct {
	PDSTransferDate *string `json:"pdsTransferDate"`
}

// Task12Data: equipment installation.
type Task12Data struct {
	ServiceOrderNumber      *string `json:"serviceOrderNumber"`
	EquipmentCompletionDate *string `json:"equipmentCompletionDate"`
}

// Task13Data: delivery.
type Task13Data struct {
	SurquilloTransferDate *string `json:"surquilloTransferDate"`
	DeliveryDate          *string `json:"deliveryDate"`
}
