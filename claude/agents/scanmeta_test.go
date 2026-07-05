package agents_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// The tests below were moved from tail-claude-hud's internal/gather white-box
// suite (parseFirstEntry/readAgentMeta) when that logic became
// ScanSubagentMeta; they exercise the same behaviors through the public API.

// writeScanFixture creates {tmp}/{session}.jsonl plus a subagents dir holding
// one agent transcript with the given first-line content and timestamp, and
// returns the session path and subagents dir.
func writeScanFixture(t *testing.T, content string, ts time.Time) (sessionPath, subagentsDir string) {
	t.Helper()
	tmp := t.TempDir()
	sessionPath = filepath.Join(tmp, "session.jsonl")
	if err := os.WriteFile(sessionPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	subagentsDir = filepath.Join(tmp, "session", "subagents")
	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatalf("mkdir subagents: %v", err)
	}
	entry := map[string]any{
		"type":        "user",
		"uuid":        "test-uuid",
		"timestamp":   ts.UTC().Format(time.RFC3339),
		"isSidechain": true,
		"message":     map[string]any{"role": "user", "content": content},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subagentsDir, "agent-abc123.jsonl"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}
	return sessionPath, subagentsDir
}

func TestScanSubagentMeta_FirstTimeFromFirstEntry(t *testing.T) {
	wantTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	sessionPath, _ := writeScanFixture(t, "Implement the feature", wantTime)

	metas := agents.ScanSubagentMeta(sessionPath)
	if len(metas) != 1 {
		t.Fatalf("len = %d, want 1", len(metas))
	}
	if !metas[0].FirstTime.Equal(wantTime) {
		t.Errorf("FirstTime = %v, want %v", metas[0].FirstTime, wantTime)
	}
}

func TestScanSubagentMeta_FiltersWarmupContent(t *testing.T) {
	sessionPath, _ := writeScanFixture(t, "Warmup", time.Now())

	if metas := agents.ScanSubagentMeta(sessionPath); len(metas) != 0 {
		t.Errorf("warmup agent not filtered: %+v", metas)
	}
}

func TestScanSubagentMeta_SidecarBothFields(t *testing.T) {
	sessionPath, subagentsDir := writeScanFixture(t, "Build the feature", time.Now())
	meta := []byte(`{"agentType":"rb-worker","description":"Build the feature"}`)
	if err := os.WriteFile(filepath.Join(subagentsDir, "agent-abc123.meta.json"), meta, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	metas := agents.ScanSubagentMeta(sessionPath)
	if len(metas) != 1 {
		t.Fatalf("len = %d, want 1", len(metas))
	}
	if metas[0].AgentType != "rb-worker" {
		t.Errorf("AgentType = %q, want rb-worker", metas[0].AgentType)
	}
	if metas[0].Description != "Build the feature" {
		t.Errorf("Description = %q, want 'Build the feature'", metas[0].Description)
	}
}

func TestScanSubagentMeta_SidecarAgentTypeOnly(t *testing.T) {
	sessionPath, subagentsDir := writeScanFixture(t, "Plan the work", time.Now())
	if err := os.WriteFile(filepath.Join(subagentsDir, "agent-abc123.meta.json"), []byte(`{"agentType":"Plan"}`), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	metas := agents.ScanSubagentMeta(sessionPath)
	if len(metas) != 1 {
		t.Fatalf("len = %d, want 1", len(metas))
	}
	if metas[0].AgentType != "Plan" || metas[0].Description != "" {
		t.Errorf("got AgentType=%q Description=%q, want Plan and empty", metas[0].AgentType, metas[0].Description)
	}
}

func TestScanSubagentMeta_MissingSidecarYieldsZeroMeta(t *testing.T) {
	sessionPath, _ := writeScanFixture(t, "No sidecar here", time.Now())

	metas := agents.ScanSubagentMeta(sessionPath)
	if len(metas) != 1 {
		t.Fatalf("len = %d, want 1", len(metas))
	}
	if metas[0].AgentType != "" || metas[0].Description != "" {
		t.Errorf("got AgentType=%q Description=%q, want both empty", metas[0].AgentType, metas[0].Description)
	}
}
