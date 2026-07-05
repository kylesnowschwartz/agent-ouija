package transcript_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

// tail-claude depends on this rule (do not swap it for offsetstore's):
// ReadSessionIncremental KEEPS an unterminated final line when it already
// parses as complete JSON. A resident fsnotify watcher may receive no
// further event for that line, so deferring it could mean never rendering
// the session's final message.
func TestReadSessionIncremental_KeepsParseableUnterminatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	complete := `{"type":"user","uuid":"u1","message":{"role":"user","content":"first"}}` + "\n"
	// Final line is complete JSON but has NO trailing newline.
	tail := `{"type":"user","uuid":"u2","message":{"role":"user","content":"final answer"}}`
	if err := os.WriteFile(path, []byte(complete+tail), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs, offset, err := transcript.ReadSessionIncremental(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2 — the parseable unterminated tail must be KEPT", len(msgs))
	}
	if want := int64(len(complete) + len(tail)); offset != want {
		t.Errorf("offset = %d, want %d (tail bytes consumed)", offset, want)
	}
}

// The half-written counterpart: a tail that does NOT parse is an append in
// progress. It must be excluded from the offset so the next incremental
// read picks it up intact once completed.
func TestReadSessionIncremental_DefersHalfWrittenTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	complete := `{"type":"user","uuid":"u1","message":{"role":"user","content":"first"}}` + "\n"
	partial := `{"type":"user","uuid":"u2","message":{"role":"us` // mid-write
	if err := os.WriteFile(path, []byte(complete+partial), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs, offset, err := transcript.ReadSessionIncremental(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1 — half-written tail must be deferred", len(msgs))
	}
	if want := int64(len(complete)); offset != want {
		t.Errorf("offset = %d, want %d (must NOT advance past the partial line)", offset, want)
	}

	// Complete the line; the next incremental read must deliver it.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	rest := `er","content":"second"}}` + "\n"
	if _, err := f.WriteString(rest); err != nil {
		t.Fatal(err)
	}
	f.Close()

	msgs2, _, err := transcript.ReadSessionIncremental(path, offset)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs2) != 1 {
		t.Fatalf("second read: len(msgs) = %d, want 1 (the completed line)", len(msgs2))
	}
}

// tail-claude depends on the 64 MiB default line cap: transcripts carry
// 1–64 MiB lines (inline images, giant pre-2.1.19x tool results) and the
// golden --dump gate cannot catch a lowered cap (skipped lines never reach
// ParseEntry). A multi-MiB entry must survive the full read path.
func TestReadSessionIncremental_MultiMiBLineSurvives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	// ~2 MiB of payload inside a single user entry.
	payload := strings.Repeat("x", 2<<20)
	big := `{"type":"user","uuid":"u1","message":{"role":"user","content":"` + payload + `"}}` + "\n"
	small := `{"type":"user","uuid":"u2","message":{"role":"user","content":"after"}}` + "\n"
	if err := os.WriteFile(path, []byte(big+small), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs, _, err := transcript.ReadSessionIncremental(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2 — a 2 MiB line was silently dropped (line cap regression)", len(msgs))
	}
}
