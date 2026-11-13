package session

import (
	"os"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// Status is the coarse state TrailingState derives from the last
// conversational message of a session. Consumers map it to their own
// vocabulary.
type Status int

const (
	// None means the session holds no user, assistant or toolResult
	// message yet.
	None Status = iota
	// Working means the last conversational message is a user message, a
	// toolResult, or an assistant message with stop reason "toolUse": the
	// model or a tool produces more output next.
	Working
	// Done means the last conversational message is an assistant message
	// that ended the run: stop reason "stop", "length" or "deferred", or a
	// reason this package does not know. A "deferred" response is a
	// provider handle for a reply fetched later; Pi's agent loop ends the
	// run on it and appends nothing more for that request.
	Done
	// Interrupted means the last assistant message has stop reason
	// "aborted": the user stopped the run.
	Interrupted
	// Error means the last assistant message has stop reason "error".
	Error
)

// String returns the lowercase status name.
func (s Status) String() string {
	switch s {
	case None:
		return "none"
	case Working:
		return "working"
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

// ToolCall is one toolCall block of an assistant message.
type ToolCall struct {
	ID   string
	Name string
	// Arguments is the arguments object exactly as written; "" when
	// absent.
	Arguments string
	// Time is the assistant entry's timestamp; zero when unparseable.
	Time time.Time
}

// State is the trailing snapshot of a session file.
type State struct {
	Status Status
	// StopReason is the last conversational message's stop reason when
	// that message is an assistant message, otherwise "".
	StopReason string
	// PendingToolCalls are the tool calls of the last assistant message
	// that have no toolResult yet, in call order. Set only when Status is
	// Working.
	PendingToolCalls []ToolCall
	// LastMessageAt is the timestamp of the last conversational message
	// entry; zero when there is none or it is unparseable.
	LastMessageAt time.Time
}

// tailStartBytes and tailMaxBytes bound TrailingState's reverse read.
const (
	tailStartBytes = 64 << 10
	tailMaxBytes   = 16 << 20
)

// TrailingState reads the end of the session file at path and returns
// its trailing State. Conversational messages are message entries with
// role user, assistant or toolResult; every other entry type and role
// (system, bashExecution, custom, session_info, ...) is skipped. A
// bashExecution message records a shell command the user ran directly;
// it starts no model turn, so it leaves the status unchanged. An
// unparseable line, such as a partly written last line, is skipped.
//
// Cost: it reads the last 64 KiB and doubles the window, up to 16 MiB,
// only while the window holds neither the last conversational message
// nor, when that message is a toolResult, the assistant message the
// result answers. A session whose last messages are larger than 16 MiB
// reports what the window holds. The per-call cost does not grow with
// the file, so a poller can call it every tick.
func TrailingState(path string) (State, error) {
	info, err := os.Stat(path)
	if err != nil {
		return State{}, err
	}
	for window := int64(tailStartBytes); ; window *= 2 {
		lines, err := jsonl.TailLines(path, window)
		if err != nil {
			return State{}, err
		}
		state, complete := foldTail(lines)
		if complete || window >= info.Size() || window >= tailMaxBytes {
			return state, nil
		}
	}
}

// foldTail scans lines newest first. complete reports whether the
// window held everything the State needs.
func foldTail(lines [][]byte) (state State, complete bool) {
	var (
		found    bool
		answered = map[string]bool{}
	)
	for i := len(lines) - 1; i >= 0; i-- {
		e, ok := ParseEntry(lines[i])
		if !ok || e.Type != "message" {
			continue
		}
		m := e.Message
		switch m.Role {
		case "user":
			if !found {
				ts, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
				return State{Status: Working, LastMessageAt: ts}, true
			}
		case "toolResult":
			if !found {
				found = true
				state.Status = Working
				state.LastMessageAt, _ = time.Parse(time.RFC3339Nano, e.Timestamp)
			}
			answered[m.ToolCallID] = true
		case "assistant":
			ts, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
			if !found {
				state.Status = assistantStatus(m.StopReason)
				state.StopReason = m.StopReason
				state.LastMessageAt = ts
			}
			if state.Status == Working {
				for _, c := range toolCalls(m.Content) {
					if !answered[c.ID] {
						c.Time = ts
						state.PendingToolCalls = append(state.PendingToolCalls, c)
					}
				}
			}
			return state, true
		}
	}
	return state, false
}

func assistantStatus(stopReason string) Status {
	switch stopReason {
	case "toolUse":
		return Working
	case "aborted":
		return Interrupted
	case "error":
		return Error
	default:
		return Done
	}
}
