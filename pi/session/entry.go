// Package session parses Pi coding agent session files: the JSONL files
// Pi writes at <agent dir>/sessions/<cwd folder>/<timestamp>_<id>.jsonl,
// one JSON object per line.
//
// Line 1 is the header ({"type":"session","version":3,"id","timestamp",
// "cwd","parentSession"?}). Every later line is an entry carrying "type",
// "id", "parentId" and "timestamp": message, model_change,
// thinking_level_change, usage, compaction, branch_summary, custom,
// custom_message, context_edit, label or session_info. Pi writes the file
// once the session holds its first user or assistant message, then
// appends one complete line per entry and never rewrites a line.
//
// Only the fields needed for TrailingState, ReadToolCalls and ReadSummary
// are modeled; unknown types and fields decode to their zero value rather
// than failing the line, matching codex/rollout's tolerant decoding.
package session

import "encoding/json"

// Entry is one line of a session file: the header or an entry.
type Entry struct {
	// Type is "session" on the header, otherwise the entry type.
	Type string `json:"type"`
	// ID is the session id on the header, otherwise the entry id.
	ID string `json:"id"`
	// ParentID is the id of the entry this one follows; "" on the first
	// entry and on the header.
	ParentID string `json:"parentId"`
	// Timestamp is the ISO-8601 time the line was written.
	Timestamp string `json:"timestamp"`

	// Version, Cwd and ParentSession are set on the header.
	Version       int    `json:"version"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession"`

	// Name is set on session_info entries: the session's display name.
	Name string `json:"name"`

	// Message is set on message entries.
	Message Message `json:"message"`
}

// Message is the body of a message entry.
type Message struct {
	// Role is "system", "user", "assistant", "toolResult" or
	// "bashExecution" (a shell command the user ran directly, not a model
	// tool call). Extensions can add roles.
	Role string `json:"role"`

	// StopReason is set on assistant messages: "stop", "length",
	// "toolUse", "error", "aborted" or "deferred". Pi never writes the
	// streaming-only "pending".
	StopReason string `json:"stopReason"`

	// ToolCallID, ToolName and IsError are set on toolResult messages.
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	IsError    bool   `json:"isError"`

	// Content is the message content: a JSON string or an array of
	// blocks (text, image, thinking, toolCall).
	Content Raw `json:"content"`
}

// Raw preserves a JSON value exactly as written while keeping Entry
// comparable; a json.RawMessage field would make it a slice.
type Raw struct {
	Raw string
}

// UnmarshalJSON records the value exactly as written.
func (r *Raw) UnmarshalJSON(b []byte) error {
	r.Raw = string(b)
	return nil
}

// ParseEntry parses one JSONL line into an Entry. It returns false when
// the line is not valid JSON or a modeled field has the wrong JSON type;
// callers reading a stream skip such a line and continue.
func ParseEntry(line []byte) (Entry, bool) {
	var e Entry
	if err := json.Unmarshal(line, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// toolCalls returns the toolCall blocks of an assistant message's
// content. Malformed content yields none.
func toolCalls(content Raw) []ToolCall {
	var blocks []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(content.Raw), &blocks) != nil {
		return nil
	}
	var out []ToolCall
	for _, b := range blocks {
		if b.Type != "toolCall" {
			continue
		}
		out = append(out, ToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Arguments)})
	}
	return out
}
