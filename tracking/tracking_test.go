package tracking

import (
	"testing"
	"time"
)

func TestGroupState(t *testing.T) {
	base := time.Now()
	later := base.Add(time.Minute)
	state := NewGroupState(base)
	if changed := state.Update(later); !changed {
		t.Errorf("Expected state to change.")
	}
	if !state.timestamp.Equal(later) {
		t.Errorf("Expected timestamp: %v, got: %v", later, state.timestamp)
	}
	if state.timeout == 0 {
		t.Errorf("Expected non-zero timeout.")
	}
	if !state.IsUp() {
		t.Errorf("Expected up state to be true.")
	}
}

func TestGroupStateDown(t *testing.T) {
	base := time.Now().Add(-time.Hour)
	state := NewGroupState(base)
	state.Update(base.Add(time.Minute))
	if state.IsUp() {
		t.Errorf("Expected up state to be false.")
	}
}
