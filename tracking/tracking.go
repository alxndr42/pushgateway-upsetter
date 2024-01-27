// Package tracking helps managing the local state of metrics groups.
package tracking

import (
	"time"
)

// GroupState contains the up state and timing data of a metrics group.
type GroupState struct {
	up        bool
	timestamp time.Time
	timeout   time.Duration
}

func NewGroupState(timestamp time.Time) *GroupState {
	return &GroupState{timestamp: timestamp}
}

// Update returns true if the up state changed during the update.
func (gs *GroupState) Update(timestamp time.Time) bool {
	wasUp := gs.up
	if timestamp.After(gs.timestamp) && !gs.timestamp.IsZero() {
		delta := timestamp.Sub(gs.timestamp)
		gs.timeout = delta + delta/2
	}
	gs.timestamp = timestamp
	gs.up = gs.IsUp()
	return wasUp != gs.up
}

// IsUp returns the up state at the current time.
func (gs *GroupState) IsUp() bool {
	if gs.timestamp.IsZero() || gs.timeout == 0 {
		return false
	}
	expiration := gs.timestamp.Add(gs.timeout)
	return time.Now().Before(expiration)
}
