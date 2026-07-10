package discover_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/codex/discover"
)

func writeRollout(t *testing.T, sessionsDir, dateParts, id string) string {
	t.Helper()
	dir := filepath.Join(sessionsDir, dateParts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-07-10T01-00-00-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"timestamp":"t","type":"turn_context","payload":{"cwd":"/work/proj"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverRollouts_NestedDirs(t *testing.T) {
	sessionsDir := t.TempDir()
	id1 := "00000000-0000-4000-8000-000000000001"
	id2 := "00000000-0000-4000-8000-000000000002"
	p1 := writeRollout(t, sessionsDir, filepath.Join("2026", "07", "10"), id1)
	p2 := writeRollout(t, sessionsDir, filepath.Join("2026", "07", "11"), id2)

	got, err := discover.DiscoverRollouts(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	byID := map[string]discover.Rollout{}
	for _, r := range got {
		byID[r.SessionID] = r
	}
	if byID[id1].Path != p1 {
		t.Errorf("id1 path = %q, want %q", byID[id1].Path, p1)
	}
	if byID[id2].Path != p2 {
		t.Errorf("id2 path = %q, want %q", byID[id2].Path, p2)
	}
}

func TestDiscoverRollouts_SkipsFilesWithoutUUID(t *testing.T) {
	sessionsDir := t.TempDir()
	dir := filepath.Join(sessionsDir, "2026", "07", "10")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Not the rollout naming convention -- no trailing UUID.
	if err := os.WriteFile(filepath.Join(dir, "notes.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-jsonl file, even with a UUID-shaped name.
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-07-10-00000000-0000-4000-8000-000000000003.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := discover.DiscoverRollouts(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0: %+v", len(got), got)
	}
}

func TestDiscoverRollouts_MissingDir(t *testing.T) {
	got, err := discover.DiscoverRollouts(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestDiscoverRollouts_SortedByModTimeDesc(t *testing.T) {
	sessionsDir := t.TempDir()
	id1 := "00000000-0000-4000-8000-000000000001"
	id2 := "00000000-0000-4000-8000-000000000002"
	p1 := writeRollout(t, sessionsDir, filepath.Join("2026", "07", "10"), id1)
	p2 := writeRollout(t, sessionsDir, filepath.Join("2026", "07", "11"), id2)

	older := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(p1, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p2, newer, newer); err != nil {
		t.Fatal(err)
	}

	got, err := discover.DiscoverRollouts(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != id2 || got[1].SessionID != id1 {
		t.Fatalf("got = %+v, want id2 first (newer)", got)
	}
}

func TestThreadNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	content := `{"id":"abc","thread_name":"first name","updated_at":"2026-07-10T00:00:00Z"}` + "\n" +
		`not valid json` + "\n" +
		`{"id":"abc","thread_name":"renamed","updated_at":"2026-07-10T01:00:00Z"}` + "\n" +
		`{"id":"def","thread_name":"other thread"}` + "\n" +
		`{"id":"","thread_name":"no id, skipped"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := discover.ThreadNames(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"abc": "renamed", "def": "other thread"}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("names[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestThreadNames_MissingFile(t *testing.T) {
	got, err := discover.ThreadNames(filepath.Join(t.TempDir(), "no-such-file.jsonl"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestThreadNames_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session_index.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := discover.ThreadNames(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}
