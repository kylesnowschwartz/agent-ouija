package transcript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolUseBlock is a raw tool_use content block from an assistant message.
// Input is preserved verbatim.
type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResultBlock is a raw tool_result content block from a user message.
// Content is preserved verbatim.
type ToolResultBlock struct {
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`

	// Outcome tells a declined or interrupted call apart from a failed one.
	// Claude Code sets is_error on all three, so IsError alone cannot.
	// ExtractContentBlocks fills it; see ToolResultOutcome for the signals.
	Outcome ToolResultOutcome `json:"-"`
}

// ToolResultOutcome is how a tool call ended, as recorded in its
// tool_result block and the entry carrying it.
type ToolResultOutcome int

const (
	// ToolResultOK: the tool ran and is_error is false.
	ToolResultOK ToolResultOutcome = iota
	// ToolResultError: the tool ran and failed, or a permission rule, hook,
	// or the auto-mode classifier refused it. is_error is true.
	ToolResultError
	// ToolResultRejected: the user declined the call at the approval
	// dialog or called it off, so it never ran.
	ToolResultRejected
	// ToolResultInterrupted: the user interrupted the call while it ran.
	ToolResultInterrupted
)

// Signals Claude Code writes for a declined or interrupted tool call.
const (
	// rejectedToolUseResult is the whole toolUseResult string of a call
	// declined at the approval dialog.
	rejectedToolUseResult = "User rejected tool use"
	// rejectedContentPrefix opens the tool_result content of a declined
	// ("doesn't want to proceed with this tool use") or cancelled ("doesn't
	// want to take this action right now") call. toolUseResult repeats it,
	// sometimes behind an "Error: " prefix.
	rejectedContentPrefix = "The user doesn't want to "
	// interruptedContentPrefix opens the tool_result content of a call the
	// user interrupted ("[Request interrupted by user for tool use]").
	interruptedContentPrefix = "[Request interrupted by user"

	denialKindUserRejected = "user-rejected"
	denialKindCancelled    = "cancelled"
)

// ThinkingBlock marks the presence of a thinking content block. The thinking
// text is intentionally omitted — only presence matters to known consumers.
type ThinkingBlock struct{}

// ContentBlocks holds the content blocks extracted from a single message by
// ExtractContentBlocks. Blocks are classified during parsing; callers access
// only the types they need.
type ContentBlocks struct {
	ToolUse    []ToolUseBlock
	ToolResult []ToolResultBlock
	Thinking   []ThinkingBlock
	HasText    bool // true when at least one "text" block is present
}

// ExtractContentBlocks walks message.content and classifies each block by
// type, preserving raw inputs and results verbatim. Unrecognised block types
// are silently ignored. Returns zero-value blocks (not an error) when
// content is absent or not a JSON array.
//
// This is the LOSSLESS extraction path: unlike Classify — which filters
// noise, strips XML, and drops sidechain entries — this function applies no
// filtering at all. Consumers that count tool calls or track agent launches
// across every entry (e.g. tail-claude-hud's extractor) depend on that:
// routing them through Classify would silently drop data.
func ExtractContentBlocks(e Entry) ContentBlocks {
	if len(e.Message.Content) == 0 {
		return ContentBlocks{}
	}

	// Content can be a JSON string (plain text) or an array of typed blocks.
	// Only arrays contain tool_use / tool_result / thinking blocks.
	if e.Message.Content[0] != '[' {
		return ContentBlocks{}
	}

	// rawBlock lets us inspect only the "type" field before full
	// unmarshalling, avoiding allocations for blocks we won't use.
	type rawBlock struct {
		Type string `json:"type"`
	}
	var raw []rawBlock
	if err := json.Unmarshal(e.Message.Content, &raw); err != nil {
		return ContentBlocks{}
	}

	var result ContentBlocks
	toolResults := 0
	for _, rb := range raw {
		if rb.Type == "tool_result" {
			toolResults++
		}
	}
	// toolUseResult and toolDenialKind describe the entry, not one block, so
	// they decide a block's outcome only when the entry carries exactly one
	// tool_result.
	entrySignals := toolResults == 1

	for i, rb := range raw {
		switch rb.Type {
		case "tool_use":
			var block ToolUseBlock
			if err := unmarshalNthBlock(e.Message.Content, i, &block); err == nil {
				result.ToolUse = append(result.ToolUse, block)
			}
		case "tool_result":
			var block ToolResultBlock
			if err := unmarshalNthBlock(e.Message.Content, i, &block); err == nil {
				block.Outcome = toolResultOutcome(e, block, entrySignals)
				result.ToolResult = append(result.ToolResult, block)
			}
		case "thinking":
			result.Thinking = append(result.Thinking, ThinkingBlock{})
		case "text":
			result.HasText = true
		}
	}
	return result
}

// toolResultOutcome classifies one tool_result. A rejection wins over an
// interruption: Claude Code follows a declined call with a separate
// "[Request interrupted by user for tool use]" text entry, so the decline
// is the more specific record. is_error is the fallback.
func toolResultOutcome(e Entry, block ToolResultBlock, entrySignals bool) ToolResultOutcome {
	text := markerText(block.Content)
	var useResult toolUseResultSignals
	if entrySignals {
		useResult = decodeToolUseResultSignals(e.ToolUseResult)
	}

	switch {
	case entrySignals && (e.ToolDenialKind == denialKindUserRejected || e.ToolDenialKind == denialKindCancelled),
		useResult.text == rejectedToolUseResult,
		strings.HasPrefix(strings.TrimPrefix(useResult.text, "Error: "), rejectedContentPrefix),
		strings.HasPrefix(text, rejectedContentPrefix):
		return ToolResultRejected
	case useResult.interrupted, strings.HasPrefix(text, interruptedContentPrefix):
		return ToolResultInterrupted
	case block.IsError:
		return ToolResultError
	default:
		return ToolResultOK
	}
}

// interruptedFlag is how the Bash tool's object-shaped toolUseResult marks
// an interrupted command. Claude Code writes compact JSON, and the same
// bytes inside an escaped string value read `\"interrupted\":true`, so a
// substring match is a sound prefilter.
var interruptedFlag = []byte(`"interrupted":true`)

// toolUseResultSignals is what toolResultOutcome reads from an entry's
// toolUseResult: the whole value when it is a string that may carry a
// rejection, or the interrupted flag when it is an object.
type toolUseResultSignals struct {
	text        string
	interrupted bool
}

// decodeToolUseResultSignals decodes toolUseResult only when a cheap byte
// scan finds a signal in it, because the value can hold a whole file or
// command output and this runs on every tool_result.
func decodeToolUseResultSignals(raw json.RawMessage) toolUseResultSignals {
	if len(raw) == 0 {
		return toolUseResultSignals{}
	}
	switch raw[0] {
	case '"':
		if !bytes.Contains(raw, []byte(rejectedToolUseResult)) && !bytes.Contains(raw, []byte(rejectedContentPrefix)) {
			return toolUseResultSignals{}
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return toolUseResultSignals{text: s}
		}
	case '{':
		if !bytes.Contains(raw, interruptedFlag) {
			return toolUseResultSignals{}
		}
		var obj struct {
			Interrupted bool `json:"interrupted"`
		}
		if json.Unmarshal(raw, &obj) == nil {
			return toolUseResultSignals{interrupted: obj.Interrupted}
		}
	}
	return toolUseResultSignals{}
}

// markerText returns a tool_result's content when it is a string, or its
// first text block when it is an array, but only when the content contains
// a rejection or interruption marker; otherwise it skips decoding and
// returns "".
func markerText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if !bytes.Contains(raw, []byte(rejectedContentPrefix)) && !bytes.Contains(raw, []byte(interruptedContentPrefix)) {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	case '[':
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(raw, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "text" {
					return b.Text
				}
			}
		}
	}
	return ""
}

// unmarshalNthBlock unmarshals the nth element of a JSON array into dest.
// Avoids re-parsing the whole array when only one element is needed.
func unmarshalNthBlock(raw json.RawMessage, n int, dest any) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	if n >= len(items) {
		return fmt.Errorf("transcript: block index %d out of range (len %d)", n, len(items))
	}
	return json.Unmarshal(items[n], dest)
}
