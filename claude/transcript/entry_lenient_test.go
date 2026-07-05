package transcript_test

import (
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

// Ported from tail-claude-hud's transcript tests: uuid-less typed entries
// (custom-title and friends) must be reachable through the lenient path.
func TestParseEntryLenient_UUIDLessCustomTitle(t *testing.T) {
	line := []byte(`{"type":"custom-title","customTitle":"My Session","slug":"my-session"}`)

	if _, ok := transcript.ParseEntry(line); ok {
		t.Fatal("strict ParseEntry must reject uuid-less entries")
	}

	e, ok := transcript.ParseEntryLenient(line)
	if !ok {
		t.Fatal("ParseEntryLenient rejected a valid uuid-less entry")
	}
	if e.Type != "custom-title" {
		t.Errorf("Type = %q, want %q", e.Type, "custom-title")
	}
	if e.CustomTitle != "My Session" {
		t.Errorf("CustomTitle = %q, want %q", e.CustomTitle, "My Session")
	}
	if e.Slug != "my-session" {
		t.Errorf("Slug = %q, want %q", e.Slug, "my-session")
	}
}

func TestParseEntryLenient_InvalidJSON(t *testing.T) {
	if _, ok := transcript.ParseEntryLenient([]byte(`{"type":`)); ok {
		t.Error("ParseEntryLenient accepted invalid JSON")
	}
}

func TestParsedTimestamp(t *testing.T) {
	tests := []struct {
		name string
		line string
		zero bool
	}{
		{"nanoseconds", `{"uuid":"x","timestamp":"2024-03-15T14:22:33.123456789Z"}`, false},
		{"rfc3339", `{"uuid":"x","timestamp":"2024-03-15T14:22:33Z"}`, false},
		{"no timezone", `{"uuid":"x","timestamp":"2024-03-15T14:22:33.123456789"}`, false},
		{"offset zone", `{"uuid":"x","timestamp":"2024-03-15T14:22:33+12:00"}`, false},
		{"missing", `{"uuid":"x"}`, true},
		{"garbage", `{"uuid":"x","timestamp":"not-a-timestamp"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := transcript.ParseEntry([]byte(tt.line))
			if !ok {
				t.Fatal("ParseEntry failed")
			}
			got := e.ParsedTimestamp()
			if got.IsZero() != tt.zero {
				t.Errorf("ParsedTimestamp().IsZero() = %v, want %v", got.IsZero(), tt.zero)
			}
		})
	}
}
