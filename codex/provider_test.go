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
	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
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

// Rollout bodies for the Ongoing tests: no lifecycle signal, a mid-turn
// trailing signal, and a completed turn.
var (
	idleBody    = `{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj"}}` + "\n"
	runningBody = strings.Join([]string{
		`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj"}}`,
		`{"timestamp":"t2","type":"event_msg","payload":{"type":"user_message"}}`,
	}, "\n") + "\n"
	doneBody = strings.Join([]string{
		`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/proj"}}`,
		`{"timestamp":"t2","type":"event_msg","payload":{"type":"task_complete"}}`,
	}, "\n") + "\n"
)

// Ongoing must mean "Running AND recently written". Running alone is not
// enough: a killed or crashed Codex never appends a terminal event, so a
// stale rollout's trailing status reads Running forever
// (rollout.OngoingStalenessThreshold is the cutoff, mirroring
// claude/discover's use of transcript.OngoingStalenessThreshold). And
// freshness alone is not enough either: idle or completed rollouts are
// not ongoing no matter how new the file is.
func TestCodexProvider_Ongoing(t *testing.T) {
	fresh := time.Now()
	stale := time.Now().Add(-rollout.OngoingStalenessThreshold - time.Minute)

	tests := []struct {
		name    string
		body    string
		modTime time.Time
		want    bool
	}{
		{name: "fresh running is ongoing", body: runningBody, modTime: fresh, want: true},
		{name: "stale running is not ongoing", body: runningBody, modTime: stale, want: false},
		{name: "fresh done is not ongoing", body: doneBody, modTime: fresh, want: false},
		{name: "stale done is not ongoing", body: doneBody, modTime: stale, want: false},
		{name: "fresh idle-only is not ongoing", body: idleBody, modTime: fresh, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := codexdir.Root(filepath.Join(t.TempDir(), ".codex"))
			dir := filepath.Join(root.SessionsDir(), "2026", "07", "10")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			id := "00000000-0000-4000-8000-000000000010"
			path := filepath.Join(dir, "rollout-2026-07-10T01-00-00-"+id+".jsonl")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, tt.modTime, tt.modTime); err != nil {
				t.Fatal(err)
			}

			refs, err := codex.New(root).Discover(sessions.Query{})
			if err != nil {
				t.Fatal(err)
			}
			if len(refs) != 1 {
				t.Fatalf("len(refs) = %d, want 1: %+v", len(refs), refs)
			}
			if refs[0].Ongoing != tt.want {
				t.Errorf("Ongoing = %v, want %v", refs[0].Ongoing, tt.want)
			}
		})
	}
}
