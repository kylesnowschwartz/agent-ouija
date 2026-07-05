package transcript

import (
	"encoding/json"
	"fmt"
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
}

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
