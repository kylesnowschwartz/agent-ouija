// Package rollout parses Codex CLI rollout transcripts: the JSONL files
// Codex CLI writes at
// $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<timestamp>-<uuid>.jsonl, one
// JSON object per line.
//
// Verified live against codex-cli 0.144.1 (2026-07-10). Only the entry
// types and payload fields needed to fold a rollout stream into a
// trailing lifecycle snapshot (see TrailingState) or to read the
// session_meta header (see SessionMeta) are modeled; unknown types and
// fields decode to their zero value rather than failing the line,
// matching this repo's tolerant-decoding convention for
// claude/transcript.
package rollout

import "encoding/json"

// Entry is one line of a rollout transcript.
type Entry struct {
	Timestamp string  `json:"timestamp"`
	Type      string  `json:"type"`
	Payload   Payload `json:"payload"`
}

// Payload is the type-dependent body of an Entry.
type Payload struct {
	// Type discriminates the payload shape. On "event_msg" entries:
	// task_complete, turn_aborted, user_message, agent_message, error. On
	// "response_item" entries: message, function_call,
	// function_call_output, reasoning.
	Type string `json:"type"`

	// Role is set on response_item entries with payload.type == "message":
	// "user" or "assistant".
	Role string `json:"role"`

	// Phase is set on assistant-authored messages (event_msg
	// agent_message, response_item message role=assistant). "commentary"
	// means the assistant is still mid-turn; any other value (including
	// empty) means this is the turn's final answer.
	Phase string `json:"phase"`

	// Cwd is set on "turn_context" entries and on the "session_meta"
	// header: the project directory the session is running in.
	Cwd string `json:"cwd"`

	// Source is set on "session_meta" entries: where the session was
	// started from.
	Source Source `json:"source"`
}

// Source is session_meta's polymorphic "source" field: a plain JSON
// string for sessions started directly by a Codex frontend (observed
// live: "cli", "exec", "vscode", "mcp"), or a JSON object for derived
// sessions (e.g. {"subagent": ...}). Rollouts from older Codex versions
// omit the field entirely, leaving the zero Source.
//
// Raw is a string, not json.RawMessage as in claude/hooks and
// claude/statusline, because Entry must stay comparable -- tests and
// consumers compare entries with ==, and a []byte field would break
// that.
type Source struct {
	// Kind is the string form of the field ("cli", "vscode", ...); ""
	// when the source is an object or absent.
	Kind string

	// Raw is the source value exactly as written, preserving the object
	// form for consumers that need it. "" when the field is absent.
	Raw string
}

// UnmarshalJSON accepts both the string and object forms; it never
// fails the line (tolerant decoding -- see ParseEntry).
func (s *Source) UnmarshalJSON(b []byte) error {
	s.Raw = string(b)
	s.Kind = ""
	var kind string
	if json.Unmarshal(b, &kind) == nil {
		s.Kind = kind
	}
	return nil
}

// ParseEntry parses one JSONL line into an Entry. Returns false if the
// line is not valid JSON. Callers reading a full stream should skip a
// rejected line and continue -- rollout files can carry a partially
// written trailing line, and this is the lenient parse path for that
// (mirrors claude/transcript.ParseEntryLenient's philosophy: unknown or
// malformed input degrades silently rather than aborting the read).
func ParseEntry(line []byte) (Entry, bool) {
	var e Entry
	if err := json.Unmarshal(line, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}
