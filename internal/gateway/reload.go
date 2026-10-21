package gateway

import "fmt"

type ReloadPhase string

const (
	ReloadValidation ReloadPhase = "validation"
	ReloadStop       ReloadPhase = "stop"
	ReloadStart      ReloadPhase = "start"
)

// ReloadError reports which lifecycle phase failed. Callers must still inspect
// final runtime health: a configuration transaction may have restored the old
// gateway after this failure, or cleanup may have left a degraded runtime.
type ReloadError struct {
	Phase ReloadPhase
	Err   error
}

func (e *ReloadError) Error() string {
	label := "reload " + string(e.Phase) + " failed"
	if e.Phase == ReloadValidation {
		label = "reload candidate validation failed"
	} else if e.Phase == ReloadStart {
		label += " after gateway stop"
	}
	return fmt.Sprintf("%s: %v", label, e.Err)
}

func (e *ReloadError) Unwrap() error { return e.Err }
