package transcript_test

// Tests for the Claude Code 2.1.18x-2.1.200 session-format changes:
// compact_boundary system entries, empty (redacted) thinking blocks,
// usage.iterations, and the unknown-entry-type fallback gate.
// Background: .agent-history/INVESTIGATION-2026-07-04-format-drift.md

import (
	"encoding/json"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

func TestClassify_CompactBoundary(t *testing.T) {
	e := makeEntry("system", "s1", "2026-07-01T10:00:00.000Z",
		json.RawMessage(`"Conversation compacted"`))
	e.Subtype = "compact_boundary"
	e.CompactMetadata.Trigger = "auto"
	e.CompactMetadata.PreTokens = 165000

	msg, ok := transcript.Classify(e)
	if !ok {
		t.Fatal("expected compact_boundary system entry to classify")
	}
	c, isCompact := msg.(transcript.CompactMsg)
	if !isCompact {
		t.Fatalf("expected CompactMsg, got %T", msg)
	}
	if c.Text != "Conversation compacted (auto, 165k tokens)" {
		t.Errorf("Text = %q, want %q", c.Text, "Conversation compacted (auto, 165k tokens)")
	}
}

func TestClassify_SystemSubtypesRemainNoise(t *testing.T) {
	for _, subtype := range []string{"turn_duration", "stop_hook_summary", "away_summary", "local_command"} {
		e := makeEntry("system", "s1", "2026-07-01T10:00:00.000Z",
			json.RawMessage(`"whatever"`))
		e.Subtype = subtype
		if _, ok := transcript.Classify(e); ok {
			t.Errorf("system subtype %q should be dropped as noise", subtype)
		}
	}
}

func TestClassify_UnknownEntryTypesDropped(t *testing.T) {
	// last-prompt carries a leafUuid so it survives ParseEntry; other
	// metadata types and hypothetical future types must not become phantom
	// meta AIMsgs either.
	for _, typ := range []string{"last-prompt", "custom-title", "agent-name", "mode", "permission-mode", "some-future-type"} {
		e := makeEntry(typ, "u1", "2026-07-01T10:00:00.000Z", nil)
		if _, ok := transcript.Classify(e); ok {
			t.Errorf("entry type %q should be dropped, not classified", typ)
		}
	}
}

func TestClassify_APIRequestEntriesDropped(t *testing.T) {
	// Claude Code 2.1.300+ records each API request in the session file.
	// api-request-blob carries a message with role "user" and real text
	// content, so it must drop on its entry type, never surface as a turn.
	lines := []string{
		`{"type":"api-request-shape","sessionId":"s1","shapeHash":"3fe7","timestamp":"2026-10-16T02:43:27.020Z","version":"2.1.310","shape":{"system":[{"type":"text","text":"You are a Claude agent."}],"tools":[]}}`,
		`{"type":"api-request-blob","sessionId":"s1","hash":"77fc","message":{"role":"user","content":[{"type":"text","text":"Run echo drift-canary"}]}}`,
		`{"type":"api-request","sessionId":"s1","id":"4983","base":"4982","timestamp":"2026-10-16T02:43:27.020Z","version":"2.1.310","querySource":"sdk","shapeHash":"3fe7","params":{"model":"claude-haiku-4-5-20251001","max_tokens":32000},"keep":0,"tail":["77fc"],"cacheControl":[],"requestDigest":"47d4"}`,
	}
	for _, line := range lines {
		e, ok := transcript.ParseEntryLenient([]byte(line))
		if !ok {
			t.Errorf("line should parse leniently: %s", line)
			continue
		}
		if msg, ok := transcript.Classify(e); ok {
			t.Errorf("entry type %q should be dropped, got %T", e.Type, msg)
		}
	}
}

func TestClassify_EmptyThinkingCountedButNotEmitted(t *testing.T) {
	// Opus 4.7+/Claude 5 transcripts persist thinking as
	// {"thinking":"","signature":"..."} — the count stays truthful but no
	// dead block should reach the display pipeline.
	content := json.RawMessage(`[
		{"type":"thinking","thinking":"","signature":"EqQBCkgIBBABGAI="},
		{"type":"thinking","thinking":"","signature":"EqQBCkgIBBABGAI="},
		{"type":"text","text":"Answer."}
	]`)
	e := makeEntry("assistant", "a1", "2026-07-01T10:00:00.000Z", content,
		withModel("claude-fable-5"))

	msg, ok := transcript.Classify(e)
	if !ok {
		t.Fatal("expected assistant message to classify")
	}
	ai := msg.(transcript.AIMsg)
	if ai.ThinkingCount != 2 {
		t.Errorf("ThinkingCount = %d, want 2", ai.ThinkingCount)
	}
	for _, b := range ai.Blocks {
		if b.Type == "thinking" {
			t.Errorf("empty thinking block should not be emitted, got %+v", b)
		}
	}
	if len(ai.Blocks) != 1 || ai.Blocks[0].Type != "text" {
		t.Errorf("Blocks = %+v, want single text block", ai.Blocks)
	}
}

func TestClassify_NonEmptyThinkingStillEmitted(t *testing.T) {
	content := json.RawMessage(`[{"type":"thinking","thinking":"real thoughts","signature":"sig"}]`)
	e := makeEntry("assistant", "a1", "2026-07-01T10:00:00.000Z", content,
		withModel("claude-haiku-4-5"))

	msg, _ := transcript.Classify(e)
	ai := msg.(transcript.AIMsg)
	if ai.ThinkingCount != 1 {
		t.Errorf("ThinkingCount = %d, want 1", ai.ThinkingCount)
	}
	if len(ai.Blocks) != 1 || ai.Blocks[0].Type != "thinking" || ai.Blocks[0].Text != "real thoughts" {
		t.Errorf("Blocks = %+v, want one thinking block with text", ai.Blocks)
	}
}

func TestClassify_UsageIterationsLastWins(t *testing.T) {
	// Multi-iteration usage: the live context window is the LAST iteration's
	// snapshot; the top-level input/cache counts are a merge across cycles.
	// OutputTokens keeps the top-level total.
	e := makeEntry("assistant", "a1", "2026-07-01T10:00:00.000Z",
		json.RawMessage(`[{"type":"text","text":"hi"}]`),
		withModel("claude-fable-5"))
	e.Message.Usage = transcript.EntryUsage{
		InputTokens:              900, // merged across iterations — must not win
		OutputTokens:             70,
		CacheReadInputTokens:     5000,
		CacheCreationInputTokens: 400,
		Iterations: []transcript.EntryUsage{
			{InputTokens: 500, OutputTokens: 30, CacheReadInputTokens: 2000, CacheCreationInputTokens: 300},
			{InputTokens: 400, OutputTokens: 40, CacheReadInputTokens: 3000, CacheCreationInputTokens: 100},
		},
	}

	msg, _ := transcript.Classify(e)
	ai := msg.(transcript.AIMsg)
	if ai.Usage.InputTokens != 400 {
		t.Errorf("InputTokens = %d, want 400 (last iteration)", ai.Usage.InputTokens)
	}
	if ai.Usage.CacheReadTokens != 3000 {
		t.Errorf("CacheReadTokens = %d, want 3000 (last iteration)", ai.Usage.CacheReadTokens)
	}
	if ai.Usage.CacheCreationTokens != 100 {
		t.Errorf("CacheCreationTokens = %d, want 100 (last iteration)", ai.Usage.CacheCreationTokens)
	}
	if ai.Usage.OutputTokens != 70 {
		t.Errorf("OutputTokens = %d, want 70 (top-level total)", ai.Usage.OutputTokens)
	}
}

func TestIsOngoing_ThinkingOnlyChunk(t *testing.T) {
	// A trailing AI chunk whose only content was redacted thinking has zero
	// items but a positive ThinkingCount — Claude is mid-thought, so the
	// session is ongoing. Regression test: the empty-thinking skip in
	// Classify must not make thinking-phase sessions look idle.
	textOut := transcript.Chunk{Type: transcript.AIChunk, Items: []transcript.DisplayItem{
		{Type: transcript.ItemOutput, Text: "done with the last request"},
	}}
	userPrompt := transcript.Chunk{Type: transcript.UserChunk, UserText: "next task"}
	thinkingOnly := transcript.Chunk{Type: transcript.AIChunk, ThinkingCount: 1}

	if !transcript.IsOngoing([]transcript.Chunk{textOut, userPrompt, thinkingOnly}) {
		t.Error("thinking-only trailing chunk should read as ongoing")
	}
	if transcript.IsOngoing([]transcript.Chunk{userPrompt, textOut}) {
		t.Error("trailing text output should read as ended")
	}
}

func TestClassify_UsageWithoutIterationsUnchanged(t *testing.T) {
	e := makeEntry("assistant", "a1", "2026-07-01T10:00:00.000Z",
		json.RawMessage(`[{"type":"text","text":"hi"}]`),
		withModel("claude-fable-5"))
	e.Message.Usage = transcript.EntryUsage{
		InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 25, CacheCreationInputTokens: 10,
	}

	msg, _ := transcript.Classify(e)
	ai := msg.(transcript.AIMsg)
	if ai.Usage.TotalTokens() != 185 {
		t.Errorf("TotalTokens = %d, want 185", ai.Usage.TotalTokens())
	}
}

// Claude Code 2.1.201 stamps every entry of a forked session with
// forkedFrom lineage ({sessionId, messageUuid}) — on user, assistant,
// system, AND attachment types. Fixture sanitized from a live 2.1.201
// transcript; found by the corpus gate 2026-07-05.
func TestParseEntry_ForkedFrom(t *testing.T) {
	line := `{"type":"user","uuid":"11111111-1111-1111-1111-111111111111","timestamp":"2026-07-05T09:00:00.000Z","sessionId":"331badfb-cd6f-44c2-b04b-356654a0bba6","version":"2.1.201","isSidechain":false,"userType":"external","cwd":"/tmp/p","gitBranch":"main","forkedFrom":{"sessionId":"8a01f462-73b1-497f-a77b-b42eb41f2de2","messageUuid":"006976ab-4078-427e-855f-17ce19dc2be6"},"message":{"role":"user","content":"hi"}}`

	e, ok := transcript.ParseEntry([]byte(line))
	if !ok {
		t.Fatal("ParseEntry rejected the forked entry")
	}
	if e.ForkedFrom == nil {
		t.Fatal("ForkedFrom = nil, want lineage")
	}
	if e.ForkedFrom.SessionID != "8a01f462-73b1-497f-a77b-b42eb41f2de2" {
		t.Errorf("SessionID = %q", e.ForkedFrom.SessionID)
	}
	if e.ForkedFrom.MessageUUID != "006976ab-4078-427e-855f-17ce19dc2be6" {
		t.Errorf("MessageUUID = %q", e.ForkedFrom.MessageUUID)
	}

	unforked := `{"type":"user","uuid":"22222222-2222-2222-2222-222222222222","timestamp":"2026-07-05T09:00:01.000Z","message":{"role":"user","content":"hi"}}`
	e2, ok := transcript.ParseEntry([]byte(unforked))
	if !ok {
		t.Fatal("ParseEntry rejected the unforked entry")
	}
	if e2.ForkedFrom != nil {
		t.Errorf("ForkedFrom = %+v, want nil on an unforked entry", e2.ForkedFrom)
	}
}

// Assistant usage carries cache_creation, the cache write split by lifetime,
// at the top level and inside each iteration. Fixture sanitized from a live
// transcript.
func TestParseEntry_UsageCacheCreationByLifetime(t *testing.T) {
	line := `{"type":"assistant","uuid":"33333333-3333-3333-3333-333333333333","timestamp":"2026-10-10T09:00:00.000Z","isSidechain":false,"message":{"id":"msg_01","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":2,"cache_creation_input_tokens":37125,"cache_read_input_tokens":15575,"output_tokens":336,"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":37125,"ephemeral_5m_input_tokens":0},"iterations":[{"input_tokens":2,"output_tokens":336,"cache_read_input_tokens":15575,"cache_creation_input_tokens":37125,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":37125},"type":"message"}]}}}`

	e, ok := transcript.ParseEntry([]byte(line))
	if !ok {
		t.Fatal("ParseEntry rejected the assistant entry")
	}
	if e.Message.ID != "msg_01" {
		t.Errorf("Message.ID = %q, want %q", e.Message.ID, "msg_01")
	}
	want := transcript.CacheCreationUsage{Ephemeral5mInputTokens: 0, Ephemeral1hInputTokens: 37125}
	if got := e.Message.Usage.CacheCreation; got != want {
		t.Errorf("CacheCreation = %+v, want %+v", got, want)
	}
	if len(e.Message.Usage.Iterations) != 1 {
		t.Fatalf("Iterations = %d, want 1", len(e.Message.Usage.Iterations))
	}
	if got := e.Message.Usage.ContextUsage().CacheCreation; got != want {
		t.Errorf("iteration CacheCreation = %+v, want %+v", got, want)
	}

	fiveMin := `{"type":"assistant","uuid":"44444444-4444-4444-4444-444444444444","timestamp":"2026-10-10T09:00:01.000Z","message":{"role":"assistant","usage":{"cache_creation_input_tokens":812,"cache_creation":{"ephemeral_5m_input_tokens":812,"ephemeral_1h_input_tokens":0}}}}`
	e2, ok := transcript.ParseEntry([]byte(fiveMin))
	if !ok {
		t.Fatal("ParseEntry rejected the 5m entry")
	}
	if got := e2.Message.Usage.CacheCreation.Ephemeral5mInputTokens; got != 812 {
		t.Errorf("Ephemeral5mInputTokens = %d, want 812", got)
	}

	legacy := `{"type":"assistant","uuid":"55555555-5555-5555-5555-555555555555","timestamp":"2026-10-10T09:00:02.000Z","message":{"role":"assistant","usage":{"input_tokens":10,"cache_creation_input_tokens":5}}}`
	e3, ok := transcript.ParseEntry([]byte(legacy))
	if !ok {
		t.Fatal("ParseEntry rejected the legacy entry")
	}
	if got := e3.Message.Usage.CacheCreation; got != (transcript.CacheCreationUsage{}) {
		t.Errorf("CacheCreation = %+v, want zero value when absent", got)
	}
}
