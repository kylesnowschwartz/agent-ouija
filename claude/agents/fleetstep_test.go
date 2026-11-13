package agents_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/claude/agents"
)

const (
	fleetLines1 = `{"type":"user","timestamp":"2026-11-01T10:00:00.000Z","isSidechain":true,"message":{"role":"user","content":"do the task"}}
{"type":"assistant","timestamp":"2026-11-01T10:00:01.000Z","isSidechain":true,"message":{"role":"assistant","model":"gpt-x","content":[{"type":"tool_use","id":"s1","name":"mcp__plugin_fleet_step__step","input":{"kind":"execute","title":"go test ./..."}},{"type":"tool_use","id":"r1","name":"Read","input":{"file_path":"/work/proj/a.go"}}]}}
{"type":"user","timestamp":"2026-11-01T10:00:02.000Z","isSidechain":true,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"s1","content":"ok"}]}}
{"type":"assistant","timestamp":"2026-11-01T10:00:03.000Z","isSidechain":true,"message":{"role":"assistant","model":"gpt-x","content":[{"type":"tool_use","id":"s2","name":"mcp__plugin_fleet_step__step","input":{"kind":"edit","title":"Edit a.go","path":"/work/proj/a.go"}}]}}
`
	fleetLines2 = `{"type":"assistant","timestamp":"2026-11-01T10:00:04.000Z","isSidechain":true,"message":{"role":"assistant","model":"gpt-x","content":[{"type":"tool_use","id":"s3","name":"mcp__plugin_fleet_step__step","input":{"kind":7,"title":"rg foo"}}]}}
{"type":"assistant","timestamp":"2026-11-01T10:00:05.000Z","isSidechain":true,"message":{"role":"assistant","model":"gpt-x","content":[{"type":"tool_use","id":"s4","name":"mcp__plugin_fleet_step__step","input":"not an object"}]}}
`
)

func TestReadFleetSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-a1.jsonl")
	if err := os.WriteFile(path, []byte(fleetLines1), 0o644); err != nil {
		t.Fatal(err)
	}

	steps, next, err := agents.ReadFleetSteps(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []agents.FleetStep{
		{ID: "s1", Timestamp: time.Date(2026, 11, 1, 10, 0, 1, 0, time.UTC), Kind: "execute", Title: "go test ./..."},
		{ID: "s2", Timestamp: time.Date(2026, 11, 1, 10, 0, 3, 0, time.UTC), Kind: "edit", Title: "Edit a.go", Path: "/work/proj/a.go"},
	}
	assertSteps(t, steps, want)
	if next != int64(len(fleetLines1)) {
		t.Fatalf("next = %d, want %d", next, len(fleetLines1))
	}

	// At EOF: no steps, same offset.
	steps, again, err := agents.ReadFleetSteps(path, next)
	if err != nil || len(steps) != 0 || again != next {
		t.Fatalf("at EOF: steps=%v next=%d err=%v, want none, %d, nil", steps, again, err, next)
	}

	// Append lines plus an unterminated tail; only the terminated new steps return.
	tail := `{"type":"assistant","timestamp":"2026-11-01T10:00:06.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","id":"s5","name":"mcp__plugin_fleet_step__step","input":{"kind":"read","title":"cat"}}]}}`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(fleetLines2 + tail); err != nil {
		t.Fatal(err)
	}
	f.Close()

	steps, next2, err := agents.ReadFleetSteps(path, next)
	if err != nil {
		t.Fatal(err)
	}
	assertSteps(t, steps, []agents.FleetStep{
		{ID: "s3", Timestamp: time.Date(2026, 11, 1, 10, 0, 4, 0, time.UTC), Title: "rg foo"},
		{ID: "s4", Timestamp: time.Date(2026, 11, 1, 10, 0, 5, 0, time.UTC)},
	})
	if want := next + int64(len(fleetLines2)); next2 != want {
		t.Fatalf("next = %d, want %d (unterminated tail deferred)", next2, want)
	}
}

func assertSteps(t *testing.T, got, want []agents.FleetStep) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d steps %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || !got[i].Timestamp.Equal(want[i].Timestamp) ||
			got[i].Kind != want[i].Kind || got[i].Title != want[i].Title || got[i].Path != want[i].Path {
			t.Errorf("step %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScanSubagentMeta_FinishedAndInterrupted(t *testing.T) {
	const first = `{"type":"user","timestamp":"2026-11-01T10:00:00Z","isSidechain":true,"message":{"role":"user","content":"go"}}` + "\n"
	const asst = `{"type":"assistant","timestamp":"2026-11-01T10:00:01Z","isSidechain":true,"message":{"role":"assistant","model":"m1","content":[{"type":"text","text":"working"}]}}` + "\n"
	const intStr = `{"type":"user","timestamp":"2026-11-01T10:00:02Z","isSidechain":true,"message":{"role":"user","content":"[Request interrupted by user]"}}` + "\n"
	const intBlocks = `{"type":"user","timestamp":"2026-11-01T10:00:02Z","isSidechain":true,"message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}` + "\n"

	tests := []struct {
		name            string
		transcript      string
		sidecar         *string
		wantFinished    bool
		wantInterrupted bool
	}{
		{"finished true", first + asst, strPtr(`{"agentType":"fleet:codex","finished":true}`), true, false},
		{"finished false", first + asst, strPtr(`{"agentType":"fleet:codex","finished":false}`), false, false},
		{"finished key absent", first + asst, strPtr(`{"agentType":"fleet:codex"}`), false, false},
		{"missing sidecar", first + asst, nil, false, false},
		{"interrupted string content", first + asst + intStr, nil, false, true},
		{"interrupted block content", first + asst + intBlocks, nil, false, true},
		{"interrupted then later line", first + intBlocks + asst, nil, false, false},
		{"only first prompt", first, nil, false, false},
		{"trailing garbage after interruption", first + intStr + "{not json\n", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			sessionPath := filepath.Join(tmp, "s.jsonl")
			dir := filepath.Join(tmp, "s", "subagents")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "agent-x1.jsonl"), []byte(tt.transcript), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.sidecar != nil {
				if !json.Valid([]byte(*tt.sidecar)) {
					t.Fatal("invalid sidecar fixture")
				}
				if err := os.WriteFile(filepath.Join(dir, "agent-x1.meta.json"), []byte(*tt.sidecar), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			metas := agents.ScanSubagentMeta(sessionPath)
			if len(metas) != 1 {
				t.Fatalf("len = %d, want 1", len(metas))
			}
			if metas[0].Finished != tt.wantFinished {
				t.Errorf("Finished = %v, want %v", metas[0].Finished, tt.wantFinished)
			}
			if metas[0].Interrupted != tt.wantInterrupted {
				t.Errorf("Interrupted = %v, want %v", metas[0].Interrupted, tt.wantInterrupted)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
