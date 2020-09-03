package main

import "github.com/gravitational/teleport-plugins/access"

// FIXME:
// Both those functions are only used to correctly marshall and unmarshall
// a JSON representation of the state - they should be implemented as
// access.State.MarshallJSON and access.State.UnmarshallJSON.
//

// stateToString converts access.State to a string.
func stateToString(state access.State) string {
	return [...]string{"", "Pending", "Approved", "Denied"}[state]
}

// stringToState converts a string to a valid State.
// if the provided string is empty or invalid, stringToState
// returns a nil access.State, which is Pending.
func stringToState(stateStr string) access.State {
	stateStrings := map[string]access.State{
		"Pending":  access.StatePending,
		"Approved": access.StateApproved,
		"Denied":   access.StateDenied,
	}
	return stateStrings[stateStr]
}
