package sns

type ConceptChanges struct {
	ChangedRecords []Event  `json:"events"`
	UpdatedIds     []string `json:"updatedIDs"`
}

type Event struct {
	ConceptType   string      `json:"type"`
	ConceptUUID   string      `json:"uuid"`
	AggregateHash string      `json:"aggregateHash"`
	TransactionID string      `json:"transactionID"`
	EventDetails  interface{} `json:"eventDetails"`
}
