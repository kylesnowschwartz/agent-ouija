package rollout

import (
	"io"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// Status is the coarse lifecycle state this package derives from a
// rollout stream. The vocabulary is owned here, not by any consumer's
// wire format -- agent-ouija is dependency-free, so this package cannot
// import a consumer's status type. Consumers map Status to their own
// enum at the boundary.
type Status int

const (
	// Idle means the stream carried no recognized lifecycle signal (a
	// rollout with only turn_context/session-meta lines, or an empty
	// stream).
	Idle Status = iota
	// Running means the last recognized signal was mid-turn activity:
	// a user message, a tool call/result, reasoning, or a commentary-phase
	// assistant message.
	Running
	// Done means the last recognized signal was a completed turn.
	Done
	// Interrupted means the last recognized signal was a user-aborted
	// turn.
	Interrupted
	// Error means the last recognized signal was an error event.
	Error
)

// String returns the lowercase status name.
func (s Status) String() string {
	switch s {
	case Idle:
		return "idle"
	case Running:
		return "running"
	case Done:
		return "done"
	case Interrupted:
		return "interrupted"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// State is the trailing snapshot TrailingState folds a rollout stream
// into.
type State struct {
	Status Status
	Cwd    string
}

// TrailingState reads every line of a rollout stream and folds it into a
// trailing {Status, Cwd} snapshot: Cwd is the first non-empty
// payload.cwd seen on a turn_context entry (the project directory never
// changes mid-session); Status is the last recognized lifecycle signal,
// later entries overriding earlier ones. Lines that fail to parse are
// skipped, not an error -- see ParseEntry. The returned error reports
// only a failure to read the underlying stream.
func TrailingState(r io.Reader) (State, error) {
	lr := jsonl.NewReader(r)
	state := State{Status: Idle}
	for {
		line, ok := lr.Next()
		if !ok {
			break
		}
		entry, ok := ParseEntry([]byte(line))
		if !ok {
			continue
		}
		if state.Cwd == "" && entry.Type == "turn_context" {
			state.Cwd = entry.Payload.Cwd
		}
		if next, ok := entryStatus(entry); ok {
			state.Status = next
		}
	}
	return state, lr.Err()
}

// entryStatus maps one entry to a lifecycle signal. The second return
// value reports whether the entry carried a recognized signal at all --
// entries that don't (turn_context, session_meta, unrecognized types)
// leave the trailing status unchanged.
func entryStatus(e Entry) (Status, bool) {
	switch e.Type {
	case "event_msg":
		switch e.Payload.Type {
		case "task_complete":
			return Done, true
		case "turn_aborted":
			return Interrupted, true
		case "user_message":
			return Running, true
		case "agent_message":
			return assistantStatus(e.Payload.Phase), true
		case "error":
			return Error, true
		}
	case "response_item":
		switch e.Payload.Type {
		case "message":
			switch e.Payload.Role {
			case "user":
				return Running, true
			case "assistant":
				return assistantStatus(e.Payload.Phase), true
			}
		case "function_call", "function_call_output", "reasoning":
			return Running, true
		}
	}
	return Idle, false
}

// assistantStatus maps an assistant message's phase to a status: still
// mid-turn ("commentary") or a completed turn (any other phase, including
// empty).
func assistantStatus(phase string) Status {
	if phase == "commentary" {
		return Running
	}
	return Done
}
