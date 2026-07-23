package roundtable

import "fmt"

// validTransitions is the room state machine skeleton (design §5.1).
// failed is reachable from any non-terminal active state.
var validTransitions = map[RoomState][]RoomState{
	StateDraftingBrief: {StateWaitingR2, StateFailed},
	StateWaitingR2:     {StateSummarizingR2, StateFailed},
	StateSummarizingR2: {StateWaitingR3, StateFailed},
	StateWaitingR3:     {StateSummarizingR3, StateFailed},
	StateSummarizingR3: {StateDone, StateFailed},
	// done / failed are terminal
	StateDone:   {},
	StateFailed: {},
}

// CanTransition reports whether from → to is a legal room state edge.
func CanTransition(from, to RoomState) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// Transition applies a legal state change to room. Mutates room.State only;
// persistence is the caller's responsibility.
func Transition(room *Room, to RoomState) error {
	if room == nil {
		return fmt.Errorf("roundtable: nil room")
	}
	if !CanTransition(room.State, to) {
		return fmt.Errorf("roundtable: illegal transition %s → %s", room.State, to)
	}
	room.State = to
	return nil
}

// IsTerminal reports whether s is a terminal room state.
func IsTerminal(s RoomState) bool {
	return s == StateDone || s == StateFailed
}
