package loginitem

import (
	"errors"
	"testing"
)

type fake struct {
	state        string
	enabledState string
	fail         bool
	calls        int
}

func (f *fake) Status() string { return f.state }
func (f *fake) SetEnabled(enabled bool) error {
	f.calls++
	if f.fail {
		return errors.New("denied")
	}
	if enabled {
		f.state = "approval"
		if f.enabledState != "" {
			f.state = f.enabledState
		}
	} else {
		f.state = "disabled"
	}
	return nil
}
func TestUsesAuthoritativeApprovalAndFailure(t *testing.T) {
	f := &fake{state: "disabled"}
	m := New(f)
	initial := m.Snapshot()
	pending := m.SetEnabled(true)
	if pending.State != "approval" || pending.Failed || pending.Sequence <= initial.Sequence {
		t.Fatal(pending)
	}
	m.SetEnabled(true)
	if f.calls != 1 {
		t.Fatal("registered an already pending login item")
	}
	f.fail = true
	failed := m.SetEnabled(false)
	if !failed.Failed || failed.State != "approval" {
		t.Fatal(failed)
	}
	f.fail = false
	disabled := m.SetEnabled(false)
	if disabled.Failed || disabled.State != "disabled" || disabled.Sequence <= failed.Sequence {
		t.Fatal(disabled)
	}
}
func TestUnavailableCannotRegister(t *testing.T) {
	if state := New(nil).SetEnabled(true); state.State != "unavailable" || !state.Failed {
		t.Fatal(state)
	}
}

func TestMissingRegistrationRecoversOnlyOnExplicitRequest(t *testing.T) {
	for _, state := range []string{"enabled", "approval"} {
		t.Run(state, func(t *testing.T) {
			f := &fake{state: "not_found", enabledState: state}
			m := New(f)
			initial := m.Snapshot()
			if initial.State != "not_found" || initial.Failed || f.calls != 0 {
				t.Fatal("reading a missing registration changed login items", initial, f.calls)
			}
			recovered := m.SetEnabled(true)
			if recovered.State != state || recovered.Failed || f.calls != 1 || recovered.Sequence <= initial.Sequence {
				t.Fatal("missing registration could not be recovered", recovered, f.calls)
			}
			m.SetEnabled(true)
			if f.calls != 1 {
				t.Fatal("registered an already enabled or pending item")
			}
		})
	}
}

func TestMissingRegistrationFailureCanBeRetried(t *testing.T) {
	f := &fake{state: "not_found", fail: true}
	m := New(f)
	failed := m.SetEnabled(true)
	if !failed.Failed || failed.State != "not_found" || failed.Error != "denied" || f.calls != 1 {
		t.Fatal(failed, f.calls)
	}
	if polled := m.Snapshot(); !polled.Failed || polled.Error != "denied" || f.calls != 1 {
		t.Fatal("polling lost the native error or retried registration", polled, f.calls)
	}
	f.fail = false
	if retried := m.SetEnabled(true); retried.Failed || retried.State != "approval" || retried.Error != "" || f.calls != 2 {
		t.Fatal(retried, f.calls)
	}
}

func TestUnsupportedProviderAndUnknownStatusCannotRegister(t *testing.T) {
	for _, state := range []string{"unavailable", "unknown"} {
		f := &fake{state: state}
		if result := New(f).SetEnabled(true); !result.Failed || result.State != "unavailable" || f.calls != 0 {
			t.Fatal(result, f.calls)
		}
	}
}
