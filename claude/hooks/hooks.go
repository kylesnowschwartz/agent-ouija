// Package hooks defines the stdin payload and stdout output shapes for
// Claude Code hook events.
//
// Claude Code pipes a JSON document to a hook command's stdin. The exact
// field set varies by event; Payload models the common superset with
// snake_case JSON tags (hook payloads are snake_case, unlike the camelCase
// native transcript fields — never mix the two conventions in one struct).
// The raw document is preserved so callers can reach unmodeled fields
// without a library release.
package hooks

import (
	"encoding/json"
	"io"
	"os"
)

// Payload is the common superset of fields Claude Code sends to hook
// commands on stdin. Fields not applicable to a given event are zero.
type Payload struct {
	// HookEventName is the canonical event discriminator, e.g.
	// "PreToolUse", "PostToolUse", "PermissionRequest", "SessionStart",
	// "Stop", "Notification", "UserPromptSubmit".
	HookEventName string `json:"hook_event_name"`

	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`

	// Tool events.
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`

	// SessionStart: "startup", "resume", "clear", or "compact".
	Source string `json:"source"`

	// UserPromptSubmit.
	Prompt string `json:"prompt"`

	// Notification.
	Message string `json:"message"`

	// Stop / SubagentStop.
	StopHookActive bool `json:"stop_hook_active"`

	// Raw is the complete JSON document as received, for fields this
	// struct does not model. Populated by Decode; ignored by Marshal.
	Raw json.RawMessage `json:"-"`
}

// Decode reads a hook payload from r (conventionally os.Stdin), preserving
// the raw bytes in Payload.Raw.
func Decode(r io.Reader) (Payload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Payload{}, err
	}
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return Payload{}, err
	}
	p.Raw = data
	return p, nil
}

// EffectiveSessionID returns the payload's session_id, falling back to the
// CLAUDE_CODE_SESSION_ID environment variable when the payload omits it.
// Some hook invocations arrive with an empty session_id; without the
// fallback, session-keyed side effects (breadcrumbs, registries) silently
// no-op.
func (p Payload) EffectiveSessionID() string {
	if p.SessionID != "" {
		return p.SessionID
	}
	return os.Getenv("CLAUDE_CODE_SESSION_ID")
}

// TerminalSequenceOutput is a hook-output document carrying a raw terminal
// escape sequence. terminalSequence is a top-level hook-output field
// (Claude Code 2.1.141+) written directly to the terminal — used for
// bells and OSC desktop notifications.
type TerminalSequenceOutput struct {
	TerminalSequence string `json:"terminalSequence"`
}

// SessionStartOutput is the SessionStart hook-output document. sessionTitle
// (Claude Code 2.1.152+) sets the terminal session title; Claude Code
// honors it only when the session source is "startup" or "resume".
type SessionStartOutput struct {
	HookSpecificOutput struct {
		HookEventName string `json:"hookEventName"`
		SessionTitle  string `json:"sessionTitle"`
	} `json:"hookSpecificOutput"`
}

// NewSessionStartOutput builds a SessionStartOutput with the event name
// pre-filled.
func NewSessionStartOutput(title string) SessionStartOutput {
	var out SessionStartOutput
	out.HookSpecificOutput.HookEventName = "SessionStart"
	out.HookSpecificOutput.SessionTitle = title
	return out
}
