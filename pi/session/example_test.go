package session_test

import (
	"fmt"
	"path/filepath"

	"github.com/kylesnowschwartz/agent-ouija/pi/session"
)

func ExampleTrailingState() {
	state, err := session.TrailingState(filepath.Join("testdata", "two-turns.jsonl"))
	if err != nil {
		return
	}
	fmt.Println(state.Status, state.StopReason, len(state.PendingToolCalls))
	// Output:
	// done stop 0
}

func ExampleReadToolCalls() {
	path := filepath.Join("testdata", "two-turns.jsonl")
	var offset int64 // kept by the poller between ticks
	calls, next, err := session.ReadToolCalls(path, offset)
	if err != nil {
		return
	}
	for _, c := range calls {
		fmt.Println(c.ID, c.Name)
	}
	offset = next
	_ = offset
	// Output:
	// call-1 bash
	// call-2 bash
}
