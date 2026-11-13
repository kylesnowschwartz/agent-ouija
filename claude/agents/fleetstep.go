package agents

import (
	"encoding/json"
	"os"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// FleetStepToolName is the MCP tool the fleet plugin records each worker step as.
const FleetStepToolName = "mcp__plugin_fleet_step__step"

// FleetStep is one call to FleetStepToolName in a subagent transcript.
type FleetStep struct {
	ID        string    // tool_use id
	Timestamp time.Time // the assistant entry's timestamp; zero when unparseable
	Kind      string    // input "kind": "execute", "read", "search", "edit", ...
	Title     string    // input "title": the command line or a label
	Path      string    // input "path"; "" when absent
}

// ReadFleetSteps returns the FleetStepToolName calls in the transcript lines
// from offset on, in file order, plus the offset to pass on the next call.
// Calls to other tools are skipped. Input is decoded tolerantly: a step whose
// input is malformed or has a field of the wrong type still yields a
// FleetStep carrying the fields that decode. Kind, Title and Path are the
// raw input strings, unsanitized, matching how the library hands tool input
// to consumers elsewhere (ContentBlock.ToolInput, ToolUseBlock.Input).
//
// Partial-last-line rule: an unterminated last line is always deferred — it
// is neither returned nor counted in next, even when it parses — so the next
// call reads it once its newline lands. This is the offsetstore rule, not
// transcript.ReadSessionIncremental's keep-a-parseable-tail rule; a step is
// reported at most one poll late and never twice. A resident poller (such as
// a sidebar server ticking each second) passes next back unchanged.
//
// transcript.ReadSessionIncremental is not used because Classify drops
// sidechain entries, and every subagent transcript entry is a sidechain entry.
func ReadFleetSteps(path string, offset int64) (steps []FleetStep, next int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset, err
	}

	lr := jsonl.NewReader(f)
	for {
		line, ok := lr.Next()
		if !ok {
			break
		}
		if !lr.LastLineTerminated() {
			break
		}
		steps = append(steps, decodeFleetSteps([]byte(line))...)
	}
	return steps, offset + lr.TerminatedBytesRead(), lr.Err()
}

// decodeFleetSteps returns the fleet step calls in one assistant line.
func decodeFleetSteps(line []byte) []FleetStep {
	var entry struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" {
		return nil
	}
	var blocks []struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if json.Unmarshal(entry.Message.Content, &blocks) != nil {
		return nil
	}
	ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
	var steps []FleetStep
	for _, b := range blocks {
		if b.Type != "tool_use" || b.Name != FleetStepToolName {
			continue
		}
		step := FleetStep{ID: b.ID, Timestamp: ts}
		var fields map[string]json.RawMessage
		if json.Unmarshal(b.Input, &fields) == nil {
			step.Kind = rawString(fields["kind"])
			step.Title = rawString(fields["title"])
			step.Path = rawString(fields["path"])
		}
		steps = append(steps, step)
	}
	return steps
}

// rawString decodes a JSON string value, returning "" for anything else.
func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}
