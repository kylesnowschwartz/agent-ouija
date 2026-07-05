package transcript_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

func writeSession(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Contract pinned to gearshifter's transcriptModel: last assistant entry
// wins, "<synthetic>" is skipped and the scan continues to older entries.
func TestLastAssistantModel(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","message":{"model":"claude-sonnet-5"}}`,
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"model":"claude-opus-4-8"}}`,
		`{"type":"assistant","message":{"model":"<synthetic>"}}`,
	)

	model, mtime := transcript.LastAssistantModel(path)
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8 (last real assistant entry)", model)
	}
	if mtime.IsZero() {
		t.Error("mtime should be the transcript's ModTime, got zero")
	}

	if m, _ := transcript.LastAssistantModel(filepath.Join(t.TempDir(), "missing.jsonl")); m != "" {
		t.Errorf("missing transcript must yield empty, got %q", m)
	}
}

// The scan decodes a minimal {type, message.model} struct on purpose:
// format drift in an unrelated modeled field (here isMeta as a string and
// stop_reason as an object, both wrong-typed for Entry) must never reject
// the line and silently drop the model — gearshifter's gear would fall
// back to the global settings model with no error.
func TestLastAssistantModel_ToleratesDriftInUnrelatedFields(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","isMeta":"yes","stop_reason":{"weird":true},"message":{"model":"claude-opus-4-8"}}`,
	)
	if m, _ := transcript.LastAssistantModel(path); m != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8 despite wrong-typed unrelated fields", m)
	}
}

func TestLastAssistantModel_NoAssistantEntries(t *testing.T) {
	path := writeSession(t, `{"type":"user","message":{"content":"hi"}}`)
	if m, _ := transcript.LastAssistantModel(path); m != "" {
		t.Errorf("want empty model, got %q", m)
	}
}

func TestScanTailEntries_NewestFirstAndLenient(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","uuid":"u1","message":{"content":"one"}}`,
		`not json at all`,
		`{"type":"custom-title","customTitle":"Named"}`,
		`{"type":"assistant","uuid":"u2","message":{"model":"claude-opus-4-8"}}`,
	)

	var types []string
	err := transcript.ScanTailEntries(path, 0, func(e transcript.Entry) bool {
		types = append(types, e.Type)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"assistant", "custom-title", "user"}
	if len(types) != len(want) {
		t.Fatalf("types = %v, want %v (newest first, garbage skipped, uuid-less included)", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("types = %v, want %v", types, want)
		}
	}
}

func TestScanTailEntries_EarlyStop(t *testing.T) {
	path := writeSession(t,
		`{"type":"user","uuid":"u1"}`,
		`{"type":"assistant","uuid":"u2"}`,
	)
	count := 0
	if err := transcript.ScanTailEntries(path, 0, func(transcript.Entry) bool {
		count++
		return false
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("fn called %d times after returning false, want 1", count)
	}
}

// LastAssistantModelAt returns the ENTRY's timestamp — the fix for the
// live-session arbitration bug (gearshifter 2026-07-05): a /model
// change appends a user entry, bumping the FILE mtime past the fresh
// settings.json while the model fact itself is old. Entries without a
// timestamp fall back to the file mtime (old fixtures keep the
// file-mtime semantics).
func TestLastAssistantModelAt(t *testing.T) {
	path := writeSession(t,
		`{"type":"assistant","timestamp":"2026-07-04T10:00:00.000Z","message":{"model":"claude-fable-5"}}`,
		`{"type":"user","timestamp":"2026-07-05T09:00:00.000Z","message":{}}`,
	)
	model, at := transcript.LastAssistantModelAt(path)
	if model != "claude-fable-5" {
		t.Errorf("model = %q, want claude-fable-5", model)
	}
	want := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	if !at.Equal(want) {
		t.Errorf("at = %v, want the assistant ENTRY's timestamp %v (not the file mtime)", at, want)
	}

	// No timestamp on the matched entry → file mtime fallback.
	path = writeSession(t, `{"type":"assistant","message":{"model":"claude-opus-4-8"}}`)
	model, at = transcript.LastAssistantModelAt(path)
	if model != "claude-opus-4-8" || at.IsZero() {
		t.Errorf("timestampless entry must fall back to file mtime, got %q %v", model, at)
	}

	if m, _ := transcript.LastAssistantModelAt(filepath.Join(t.TempDir(), "missing.jsonl")); m != "" {
		t.Errorf("missing transcript must yield empty, got %q", m)
	}
}
