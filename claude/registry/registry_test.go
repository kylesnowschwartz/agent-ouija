package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Ported from gearshifter@e718c8e internal/agent/claude/session_test.go.

func TestDescendantsOf(t *testing.T) {
	tree := ProcessTree{1: {10, 20}, 10: {30}, 30: {40}}
	desc := tree.DescendantsOf(10)
	for _, pid := range []int{10, 30, 40} {
		if !desc[pid] {
			t.Errorf("pid %d should be a descendant of 10", pid)
		}
	}
	if desc[20] || desc[1] {
		t.Error("siblings and ancestors are not descendants")
	}
	if len(tree.DescendantsOf(0)) != 0 {
		t.Error("root 0 must yield nothing")
	}
}

func TestResolve(t *testing.T) {
	alive := os.Getpid() // liveness check needs a real process
	entries := []Live{
		{PID: alive, SessionID: "in-tree", Cwd: "/a", StartedAt: 1782774895166},
		{PID: alive, SessionID: "same-cwd-old", Cwd: "/b", StartedAt: 1782774895166},
		{PID: alive, SessionID: "same-cwd-new", Cwd: "/b", StartedAt: 1782774899999},
		{PID: 99999999, SessionID: "dead", Cwd: "/b", StartedAt: 1782774999999},
	}
	if e, ok := Resolve(entries, map[int]bool{alive: true}, "/x"); !ok || e.SessionID != "in-tree" {
		t.Errorf("pid-in-tree match: got %v %v, want in-tree", e.SessionID, ok)
	}
	if e, ok := Resolve(entries, nil, "/b"); !ok || e.SessionID != "same-cwd-new" {
		t.Errorf("cwd fallback: got %v %v, want same-cwd-new (newest alive)", e.SessionID, ok)
	}
	if _, ok := Resolve(entries, nil, "/nowhere"); ok {
		t.Error("no match must report not-found")
	}
	// A registry entry with no startedAt is still a live cwd match — the
	// first live match must win even with a zero startedAt (0 > 0 is
	// false, so a bare > comparison never selects it).
	bare := []Live{{PID: alive, SessionID: "no-started-at", Cwd: "/c"}}
	if e, ok := Resolve(bare, nil, "/c"); !ok || e.SessionID != "no-started-at" {
		t.Errorf("zero startedAt: got %v %v, want no-started-at", e.SessionID, ok)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	entry := fmt.Sprintf(`{"pid":%d,"sessionId":"s1","cwd":"/proj","startedAt":1782774895166}`, os.Getpid())
	if err := os.WriteFile(filepath.Join(dir, "1.json"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	// No sessionId: skipped.
	if err := os.WriteFile(filepath.Join(dir, "2.json"), []byte(`{"pid":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Invalid JSON: skipped.
	if err := os.WriteFile(filepath.Join(dir, "3.json"), []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := Read(dir)
	if len(entries) != 1 || entries[0].SessionID != "s1" || entries[0].Cwd != "/proj" {
		t.Errorf("Read = %+v, want one s1 entry", entries)
	}

	if got := Read(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("missing dir: got %v, want nil", got)
	}
}

// TestRead_CurrentFormatFixture is the format-drift regression test: a
// sanitized copy of a real 2.1.195 sessions file, every key intact. The
// original bug: startedAt drifted from an RFC3339 string to an epoch-ms
// number, a strict `string` field failed the unmarshal, and Read skipped
// every entry — the registry went completely blind (0 of 6 live files
// decoded). Dependent consumers: gearshifter (pane→session), tcm
// (liveness probe).
func TestRead_CurrentFormatFixture(t *testing.T) {
	const fixture = `{"pid":12839,"sessionId":"11111111-2222-4333-b444-555555555555","cwd":"/Users/u/Code/proj","startedAt":1782774895166,"procStart":"Mon Jun 29 23:14:54 2026","version":"2.1.195","peerProtocol":1,"kind":"interactive","entrypoint":"cli","name":"example-session","updatedAt":1783047882193,"status":"idle","statusUpdatedAt":1783047882193,"agent":"rb-orchestrator"}`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "12839.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := Read(dir)
	if len(entries) != 1 {
		t.Fatalf("current-format file must decode; got %d entries", len(entries))
	}
	e := entries[0]
	want := Live{
		PID:             12839,
		SessionID:       "11111111-2222-4333-b444-555555555555",
		Cwd:             "/Users/u/Code/proj",
		StartedAt:       1782774895166,
		Name:            "example-session",
		Kind:            "interactive",
		Status:          "idle",
		UpdatedAt:       1783047882193,
		StatusUpdatedAt: 1783047882193,
		Agent:           "rb-orchestrator",
	}
	if e != want {
		t.Errorf("decoded entry:\n got %+v\nwant %+v", e, want)
	}
}

func TestEpochMS_TolerantDecode(t *testing.T) {
	cases := []struct {
		name string
		json string
		want EpochMS
	}{
		{"number (current format)", `1782774895166`, 1782774895166},
		{"numeric string", `"1782774895166"`, 1782774895166},
		{"rfc3339 string (old format)", `"2026-07-05T01:00:00Z"`,
			EpochMS(time.Date(2026, 7, 5, 1, 0, 0, 0, time.UTC).UnixMilli())},
		{"null", `null`, 0},
		{"garbage string", `"soon"`, 0},
		{"object", `{}`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var e EpochMS
			if err := e.UnmarshalJSON([]byte(c.json)); err != nil {
				t.Fatalf("tolerant decode must never error, got %v", err)
			}
			if e != c.want {
				t.Errorf("decode %s = %d, want %d", c.json, e, c.want)
			}
		})
	}
}

func TestAlive(t *testing.T) {
	if !(Live{PID: os.Getpid()}).Alive() {
		t.Error("current process must be alive")
	}
	if (Live{PID: 99999999}).Alive() {
		t.Error("bogus pid must not be alive")
	}
	if (Live{PID: 0}).Alive() {
		t.Error("pid 0 must not be alive")
	}
}

func TestCurrentProcessTree(t *testing.T) {
	tree := CurrentProcessTree()
	if tree == nil {
		t.Skip("ps unavailable")
	}
	// The current process must appear somewhere in its parent's children.
	desc := tree.DescendantsOf(os.Getppid())
	if !desc[os.Getpid()] {
		t.Error("current pid not found under its parent in the process tree")
	}
}
