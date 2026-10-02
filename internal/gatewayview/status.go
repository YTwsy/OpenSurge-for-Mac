// Package gatewayview defines the shared, additive presentation contract for
// gateway status. It never controls services or changes raw runtime evidence.
package gatewayview

type Status struct {
	State            string `json:"state"`
	Reason           string `json:"reason,omitempty"`
	Phase            string `json:"phase,omitempty"`
	OperationID      string `json:"operation_id,omitempty"`
	Busy             bool   `json:"busy"`
	ConfigPending    bool   `json:"config_pending"`
	DiagnosisWarning bool   `json:"diagnosis_warning"`
}
