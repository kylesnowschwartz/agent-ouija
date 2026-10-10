package transcript_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

// TestToolResultOutcome_Fixture covers each way a tool call ends, using
// tool_result entries in the shapes Claude Code writes them.
func TestToolResultOutcome_Fixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("../testdata", "tool_result_outcomes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]transcript.ToolResultOutcome{}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		blocks := transcript.ExtractContentBlocks(mustParseLenient(t, string(line)))
		for _, r := range blocks.ToolResult {
			got[r.ToolUseID] = r.Outcome
		}
	}

	want := map[string]transcript.ToolResultOutcome{
		"tu-rejected":              transcript.ToolResultRejected,
		"tu-rejected-error-prefix": transcript.ToolResultRejected,
		"tu-rejected-legacy":       transcript.ToolResultRejected,
		"tu-cancelled":             transcript.ToolResultRejected,
		"tu-interrupted-bash":      transcript.ToolResultInterrupted,
		"tu-interrupted-text":      transcript.ToolResultInterrupted,
		"tu-failed":                transcript.ToolResultError,
		"tu-auto-denied":           transcript.ToolResultError,
		"tu-ok":                    transcript.ToolResultOK,
	}
	if len(got) != len(want) {
		t.Errorf("fixture yielded %d tool results, want %d", len(got), len(want))
	}
	for id, w := range want {
		if g, ok := got[id]; !ok {
			t.Errorf("%s: missing from fixture", id)
		} else if g != w {
			t.Errorf("%s: Outcome = %d, want %d", id, g, w)
		}
	}
}

// The Bash tool's interrupted flag decides the outcome on its own, even when
// the content is the partial output and is_error is unset.
func TestToolResultOutcome_InterruptedFlagAlone(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","type":"user","message":{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"tu1","content":"partial output"}
	]},"toolUseResult":{"stdout":"partial output","stderr":"","interrupted":true,"isImage":false}}`)

	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolResult) != 1 || blocks.ToolResult[0].Outcome != transcript.ToolResultInterrupted {
		t.Fatalf("ToolResult = %+v, want one interrupted result", blocks.ToolResult)
	}
}

// A command whose output merely mentions the markers is not misread: the
// escaped flag inside stdout is not the object's own field, and marker text
// that does not open the content is ordinary output.
func TestToolResultOutcome_MarkersInsideOutput(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","type":"user","message":{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"tu1","content":"grep: [Request interrupted by user for tool use] and The user doesn't want to proceed","is_error":false}
	]},"toolUseResult":{"stdout":"{\"interrupted\":true}","stderr":"","interrupted":false}}`)

	blocks := transcript.ExtractContentBlocks(e)
	if len(blocks.ToolResult) != 1 || blocks.ToolResult[0].Outcome != transcript.ToolResultOK {
		t.Fatalf("ToolResult = %+v, want one OK result", blocks.ToolResult)
	}
}

// toolUseResult and toolDenialKind are entry-level, so with several
// tool_results in one entry they cannot be attributed to any one block; each
// block's own content and is_error decide.
func TestToolResultOutcome_EntrySignalsIgnoredForMultipleResults(t *testing.T) {
	e := mustParseLenient(t, `{"uuid":"x","type":"user","message":{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"tu1","content":"done","is_error":false},
		{"type":"tool_result","tool_use_id":"tu2","content":"Exit code 2","is_error":true},
		{"type":"tool_result","tool_use_id":"tu3","content":"[Request interrupted by user for tool use]","is_error":true}
	]},"toolUseResult":"User rejected tool use","toolDenialKind":"user-rejected"}`)

	blocks := transcript.ExtractContentBlocks(e)
	want := []transcript.ToolResultOutcome{
		transcript.ToolResultOK,
		transcript.ToolResultError,
		transcript.ToolResultInterrupted,
	}
	if len(blocks.ToolResult) != len(want) {
		t.Fatalf("got %d tool results, want %d", len(blocks.ToolResult), len(want))
	}
	for i, w := range want {
		if g := blocks.ToolResult[i].Outcome; g != w {
			t.Errorf("result %d (%s): Outcome = %d, want %d", i, blocks.ToolResult[i].ToolUseID, g, w)
		}
	}
}
