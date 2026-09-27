package loginitem

import (
	"errors"
	"testing"
)

type fake struct {
	state string
	fail  bool
	calls int
}

func (f *fake) Status() string { return f.state }
func (f *fake) SetEnabled(enabled bool) error {
	f.calls++
	if f.fail {
		return errors.New("denied")
	}
	if enabled {
		f.state = "approval"
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
