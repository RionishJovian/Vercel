package broadband

// Spec is persisted encrypted before router I/O, so retries reuse the exact
// operation even after a process restart. Never serialize it to an HTTP response.
type Spec struct {
	AccountID         string `json:"account_id"`
	OperationID       string `json:"operation_id"`
	ExpectedOperation string `json:"expected_operation"`
	Action            string `json:"action"`
	Username          string `json:"username"`
	Password          string `json:"password,omitempty"`
	Profile           string `json:"profile"`
	PeriodSeconds     int64  `json:"period_seconds"`
}
type Observation struct {
	State          State         `json:"state"`
	OperationID    string        `json:"operation_id"`
	ObservedAt     int64         `json:"observed_at"`
	Connected      bool          `json:"connected"`
	Session        *SessionUsage `json:"session,omitempty"`
	UsageAvailable bool          `json:"usage_available"`
}

// These are observed session counters, never a claimed complete billing meter.
// Sessions that begin and end between polls cannot be reconstructed from PPP active.
type SessionUsage struct {
	ID            string `json:"id"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	DownloadBytes int64  `json:"download_bytes"`
	UploadBytes   int64  `json:"upload_bytes"`
}
