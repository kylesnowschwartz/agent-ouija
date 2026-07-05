package agents_test

import (
	"path/filepath"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/agents"
)

// The test-session fixture contains agent-abc1234 and agent-def5678 (real),
// plus agent-warmup99 (warmup), agent-acompact-xyz (compaction artifact),
// and agent-empty000 (zero bytes) — all three must be filtered.
func TestScanSubagentMeta_FiltersLikeDiscoverSubagents(t *testing.T) {
	sessionPath := filepath.Join("../testdata", "test-session.jsonl")

	metas := agents.ScanSubagentMeta(sessionPath)
	if len(metas) != 2 {
		t.Fatalf("len = %d, want 2 (warmup/compact/empty filtered): %+v", len(metas), metas)
	}
	ids := map[string]bool{}
	for _, m := range metas {
		ids[m.ID] = true
		if m.Size == 0 {
			t.Errorf("agent %s has Size 0", m.ID)
		}
		if m.ModTime.IsZero() {
			t.Errorf("agent %s has zero ModTime", m.ID)
		}
		if m.FirstTime.IsZero() {
			t.Errorf("agent %s has zero FirstTime (fixture first lines carry timestamps)", m.ID)
		}
	}
	if !ids["abc1234"] || !ids["def5678"] {
		t.Errorf("ids = %v, want abc1234 and def5678", ids)
	}
}

func TestScanSubagentMeta_MissingDir(t *testing.T) {
	if got := agents.ScanSubagentMeta(filepath.Join(t.TempDir(), "nope.jsonl")); got != nil {
		t.Errorf("missing subagents dir: got %v, want nil", got)
	}
}
