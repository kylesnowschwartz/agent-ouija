package transcript_test

import (
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

func mustParseLenient(t *testing.T, line string) transcript.Entry {
	t.Helper()
	e, ok := transcript.ParseEntryLenient([]byte(line))
	if !ok {
		t.Fatalf("ParseEntryLenient(%q) failed", line)
	}
	return e
}

func TestExtractContentBlocks_AllTypes(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","message":{"role":"assistant","content":[
		{"type":"thinking","thinking":"hmm"},
		{"type":"text","text":"hello"},
		{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"/a.go"}},
		{"type":"tool_result","tool_use_id":"tu1","content":"ok","is_error":false}
	]}}`)

	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolUse) != 1 || blocks.ToolUse[0].Name != "Read" || blocks.ToolUse[0].ID != "tu1" {
		t.Errorf("ToolUse = %+v, want one Read block with ID tu1", blocks.ToolUse)
	}
	if string(blocks.ToolUse[0].Input) != `{"file_path":"/a.go"}` {
		t.Errorf("ToolUse Input not preserved verbatim: %s", blocks.ToolUse[0].Input)
	}
	if len(blocks.ToolResult) != 1 || blocks.ToolResult[0].ToolUseID != "tu1" {
		t.Errorf("ToolResult = %+v, want one block for tu1", blocks.ToolResult)
	}
	if len(blocks.Thinking) != 1 {
		t.Errorf("Thinking = %d blocks, want 1", len(blocks.Thinking))
	}
	if !blocks.HasText {
		t.Error("HasText = false, want true")
	}
}

// ExtractContentBlocks is the lossless path: sidechain entries must NOT be
// filtered (Classify drops them; consumers like tail-claude-hud's extractor
// count tool calls across every entry and rely on this difference).
func TestExtractContentBlocks_NoSidechainFiltering(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","isSidechain":true,"message":{"role":"assistant","content":[
		{"type":"tool_use","id":"tu1","name":"Bash","input":{}}
	]}}`)

	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolUse) != 1 {
		t.Fatalf("sidechain entry's tool_use dropped; ExtractContentBlocks must not filter")
	}
}

func TestExtractContentBlocks_StringContent(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","message":{"role":"user","content":"plain text"}}`)
	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolUse) != 0 || len(blocks.ToolResult) != 0 || blocks.HasText {
		t.Errorf("string content should yield zero blocks, got %+v", blocks)
	}
}

func TestExtractContentBlocks_ErrorResult(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","message":{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"tu9","content":"boom","is_error":true}
	]}}`)
	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolResult) != 1 || !blocks.ToolResult[0].IsError {
		t.Errorf("IsError not preserved: %+v", blocks.ToolResult)
	}
}

func TestExtractContentBlocks_UnknownBlockTypesIgnored(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","message":{"role":"assistant","content":[
		{"type":"future_block","data":123},
		{"type":"text","text":"hi"}
	]}}`)
	blocks := transcript.ExtractContentBlocks(e)
	if !blocks.HasText {
		t.Error("known blocks must survive unknown siblings")
	}
}
