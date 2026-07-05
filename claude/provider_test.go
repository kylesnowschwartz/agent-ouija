package claude_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/claude"
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/sessionstest"
)

// seedSession writes a minimal real session JSONL (one user turn, one
// assistant turn, a custom-title record) under the root's projects dir and
// stamps the seed's ModTime.
func seedSession(t *testing.T, root claudedir.Root, i int, s sessionstest.Seed) {
	t.Helper()
	dir := root.ProjectDirFor(s.ProjectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := s.ModTime.UTC().Format("2006-01-02T15:04:05Z")
	lines := fmt.Sprintf(
		`{"type":"user","uuid":"u1","timestamp":%q,"cwd":%q,"message":{"role":"user","content":"hello"}}`+"\n"+
			`{"type":"assistant","uuid":"a1","timestamp":%q,"cwd":%q,"message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"done"}]}}`+"\n",
		ts, s.ProjectDir, ts, s.ProjectDir)
	if s.Title != "" {
		lines += fmt.Sprintf(`{"type":"custom-title","customTitle":%q}`+"\n", s.Title)
	}
	path := filepath.Join(dir, fmt.Sprintf("sess-%d.jsonl", i))
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, s.ModTime, s.ModTime); err != nil {
		t.Fatal(err)
	}
}

// The claude provider must pass the same conformance suite as the fake —
// this is the v1 architecture proof for the provider abstraction.
func TestClaudeProviderConformance(t *testing.T) {
	sessionstest.Run(t, sessionstest.Harness{
		Make: func(t *testing.T, seeds []sessionstest.Seed) sessions.Provider {
			root := claudedir.Root(filepath.Join(t.TempDir(), ".claude"))
			if err := os.MkdirAll(root.ProjectsDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			for i, s := range seeds {
				seedSession(t, root, i, s)
			}
			return claude.New(root)
		},
	})
}

// LiveTracker capability: the provider reports registry entries whose
// process is alive, discovered via type assertion (the capability
// pattern), not via a fat mandatory interface.
func TestClaudeProviderLiveTracker(t *testing.T) {
	root := claudedir.Root(filepath.Join(t.TempDir(), ".claude"))
	if err := os.MkdirAll(root.SessionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	alive := fmt.Sprintf(`{"pid":%d,"sessionId":"live-1","cwd":"/proj","startedAt":"2026-07-05T01:00:00Z"}`, os.Getpid())
	dead := `{"pid":99999999,"sessionId":"dead-1","cwd":"/proj","startedAt":"2026-07-05T02:00:00Z"}`
	if err := os.WriteFile(filepath.Join(root.SessionsDir(), "a.json"), []byte(alive), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root.SessionsDir(), "b.json"), []byte(dead), 0o644); err != nil {
		t.Fatal(err)
	}

	var p sessions.Provider = claude.New(root)
	lt, ok := p.(sessions.LiveTracker)
	if !ok {
		t.Fatal("claude.Provider must implement the LiveTracker capability")
	}
	live, err := lt.LiveSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].Ref.ID != "live-1" || live[0].PID != os.Getpid() {
		t.Fatalf("live = %+v, want only live-1 with our pid", live)
	}
	if live[0].Ref.Provider != "claude" {
		t.Errorf("Ref.Provider = %q, want claude", live[0].Ref.Provider)
	}
}
