package pat

import (
	"encoding/json"
	"regexp"
)

// Tag constants matching the TypeScript messageTags.ts.
const (
	LocalCommandStdoutTag = "<local-command-stdout>"
	LocalCommandStderrTag = "<local-command-stderr>"
)

// Bash mode tags -- inline command execution via !bash in Claude Code.
const (
	BashStdoutTag       = "<bash-stdout>"
	BashStderrTag       = "<bash-stderr>"
	TaskNotificationTag = "<task-notification>"
)

// Command extraction regexes -- used by sanitize.go and session.go.
var (
	CommandNameRe = regexp.MustCompile(`<command-name>/([^<]+)</command-name>`)
	CommandArgsRe = regexp.MustCompile(`<command-args>([^<]*)</command-args>`)
	StdoutRe      = regexp.MustCompile(`(?is)<local-command-stdout>(.*?)</local-command-stdout>`)
	StderrRe      = regexp.MustCompile(`(?is)<local-command-stderr>(.*?)</local-command-stderr>`)
)

// Bash mode regexes -- used by classify.go and sanitize.go.
var (
	BashStdoutRe        = regexp.MustCompile(`(?is)<bash-stdout>(.*?)</bash-stdout>`)
	BashStderrRe        = regexp.MustCompile(`(?is)<bash-stderr>(.*?)</bash-stderr>`)
	BashInputRe         = regexp.MustCompile(`(?is)<bash-input>(.*?)</bash-input>`)
	TaskNotifySummaryRe = regexp.MustCompile(`(?is)<summary>(.*?)</summary>`)
	TaskNotifyStatusRe  = regexp.MustCompile(`(?is)<status>(.*?)</status>`)
)

// Teammate message regexes -- used by classify.go, session.go, and subagent.go.
var (
	TeammateMessageRe  = regexp.MustCompile(`^<teammate-message\s+teammate_id="[^"]+"`)
	TeammateIDRe       = regexp.MustCompile(`teammate_id="([^"]+)"`)
	TeammateContentRe  = regexp.MustCompile(`(?s)<teammate-message[^>]*>(.*?)</teammate-message>`)
	TeammateSummaryRe  = regexp.MustCompile(`<teammate-message[^>]*\bsummary="([^"]+)"`)
	TeammateColorRe    = regexp.MustCompile(`<teammate-message[^>]*\bcolor="([^"]+)"`)
	TeammateProtocolRe = regexp.MustCompile(`^\s*\{\s*"type"\s*:\s*"(idle_notification|shutdown_approved|shutdown_request|teammate_terminated|task_assignment)"`)
)

// ContentBlockJSON is the common shape for partially unmarshaling JSONL content blocks.
// Different callers use different subsets of fields; unused fields unmarshal to zero values.
type ContentBlockJSON struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// TextBlockJSON is a minimal content block for extracting text content.
// Cheaper to unmarshal when only type and text are needed.
type TextBlockJSON struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SystemOutputTags are the XML wrappers that mark a user entry as system
// output rather than genuine user input (used by classification and by the
// discovery metadata scan's turn counting).
var SystemOutputTags = []string{
	LocalCommandStderrTag,
	LocalCommandStdoutTag,
	"<local-command-caveat>",
	"<system-reminder>",
	BashStdoutTag,
	BashStderrTag,
	TaskNotificationTag,
}
