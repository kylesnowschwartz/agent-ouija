package codex_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/codex"
	"github.com/kylesnowschwartz/agent-ouija/codex/codexdir"
	"github.com/kylesnowschwartz/agent-ouija/sessionstest"
)

// seedRollout writes a minimal real rollout JSONL (one turn_context entry
// pinning cwd) under the root's sessions dir, nested YYYY/MM/DD, and
// stamps the seed's ModTime. When the seed has a title, it also appends a
// session_index.jsonl entry mapping the rollout's session id to that
// title (Codex's thread-name equivalent).
func seedRollout(t *testing.T, root codexdir.Root, i int, s sessionstest.Seed) {
	t.Helper()
	day := s.ModTime.UTC().Format("2006/01/02")
	dir := filepath.Join(root.SessionsDir(), day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
	ts := s.ModTime.UTC().Format("2006-01-02T15-04-05")
	path := filepath.Join(dir, fmt.Sprintf("rollout-%s-%s.jsonl", ts, id))
	line := fmt.Sprintf(`{"timestamp":%q,"type":"turn_context","payload":{"cwd":%q}}`+"\n",
		s.ModTime.UTC().Format(time.RFC3339), s.ProjectDir)
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, s.ModTime, s.ModTime); err != nil {
		t.Fatal(err)
	}

	if s.Title != "" {
		idxLine := fmt.Sprintf(`{"id":%q,"thread_name":%q,"updated_at":%q}`+"\n",
			id, s.Title, s.ModTime.UTC().Format(time.RFC3339))
		f, err := os.OpenFile(root.SessionIndexPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(idxLine); err != nil {
			t.Fatal(err)
		}
	}
}

// The codex provider must pass the same conformance suite as the fake and
// claude.Provider -- the architecture proof sessionstest's package doc
// names codex as the intended second provider.
func TestCodexProviderConformance(t *testing.T) {
	sessionstest.Run(t, sessionstest.Harness{
		Make: func(t *testing.T, seeds []sessionstest.Seed) sessions.Provider {
			root := codexdir.Root(filepath.Join(t.TempDir(), ".codex"))
			if err := os.MkdirAll(root.SessionsDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			for i, s := range seeds {
				seedRollout(t, root, i, s)
			}
			return codex.New(root)
		},
	})
}

// codex.Provider deliberately does not implement sessions.LiveTracker --
// see the doc comment on Provider for why. sessionstest.Run's
// LiveTrackerCapability subtest already skips providers that don't
// implement it; this test pins that absence so a future accidental
// implementation gets noticed.
func TestCodexProvider_NotALiveTracker(t *testing.T) {
	var p sessions.Provider = codex.New(codexdir.Root(t.TempDir()))
	if _, ok := p.(sessions.LiveTracker); ok {
		t.Error("codex.Provider must not implement sessions.LiveTracker (see Provider doc comment)")
	}
}

// Ongoing must reflect the Running status specifically, not "anything
// non-terminal": an idle rollout that never started a turn (only
// turn_context) must not be reported ongoing.
func TestCodexProvider_OngoingReflectsRunningStatus(t *testing.T) {
	root := codexdir.Root(filepath.Join(t.TempDir(), ".codex"))
	if err := os.MkdirAll(root.SessionsDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(id, dateDir, body string) {
		dir := filepath.Join(root.SessionsDir(), dateDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "rollout-2026-07-10T01-00-00-"+id+".jsonl")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	idleID := "00000000-0000-4000-8000-000000000010"
	runningID := "00000000-0000-4000-8000-000000000011"
	doneID := "00000000-0000-4000-8000-000000000012"

	write(idleID, "2026/07/10", `{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj/idle"}}`+"\n")
	write(runningID, "2026/07/10", strings.Join([]string{
		`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj/running"}}`,
		`{"timestamp":"t2","type":"event_msg","payload":{"type":"user_message"}}`,
	}, "\n")+"\n")
	write(doneID, "2026/07/10", strings.Join([]string{
		`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj/done"}}`,
		`{"timestamp":"t2","type":"event_msg","payload":{"type":"task_complete"}}`,
	}, "\n")+"\n")

	p := codex.New(root)
	refs, err := p.Discover(sessions.Query{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]sessions.SessionRef{}
	for _, r := range refs {
		byID[r.ID] = r
	}

	if byID[idleID].Ongoing {
		t.Error("idle-only rollout (turn_context, no lifecycle signal) must not be Ongoing")
	}
	if !byID[runningID].Ongoing {
		t.Error("rollout trailing on user_message must be Ongoing")
	}
	if byID[doneID].Ongoing {
		t.Error("rollout trailing on task_complete must not be Ongoing")
	}
}
