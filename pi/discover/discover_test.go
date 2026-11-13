package discover_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/pi/discover"
)

func TestSessionDirForCwd(t *testing.T) {
	tests := []struct {
		name, cwd, want string
	}{
		{"unix path", "/work/proj", "--work-proj--"},
		{"nested", "/a/b.c/d", "--a-b.c-d--"},
		{"root", "/", "----"},
		{"windows path", `C:\Users\u\proj`, "--C--Users-u-proj--"},
		{"leading backslash", `\srv\share`, "--srv-share--"},
		{"only one leading separator stripped", "//x", "---x--"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discover.SessionDirForCwd("/s", tt.cwd)
			if want := filepath.Join("/s", tt.want); got != want {
				t.Errorf("SessionDirForCwd(%q) = %q, want %q", tt.cwd, got, want)
			}
		})
	}
}

func TestSessionIDFromPath(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		{"uuid", "/s/--p--/2026-11-13T10-02-46-506Z_01a1d45c-1369-7516-b02b-042ec93fbe30.jsonl", "01a1d45c-1369-7516-b02b-042ec93fbe30"},
		{"custom id with . _ -", "2026-11-13T10-02-46-506Z_my.id_with-parts_2.jsonl", "my.id_with-parts_2"},
		{"no timestamp", "notes_abc.jsonl", ""},
		{"wrong extension", "2026-11-13T10-02-46-506Z_abc.json", ""},
		{"id ends with separator", "2026-11-13T10-02-46-506Z_abc_.jsonl", ""},
		{"empty id", "2026-11-13T10-02-46-506Z_.jsonl", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discover.SessionIDFromPath(tt.path); got != tt.want {
				t.Errorf("SessionIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func write(t *testing.T, dir, name, body string, mod time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscoverSessions(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	older := write(t, dir, "2026-11-13T10-00-00-000Z_a1.jsonl",
		`{"type":"session","version":3,"id":"a1","timestamp":"2026-11-13T10:00:00.000Z","cwd":"/work/proj"}`+"\n", base)
	newer := write(t, dir, "2026-11-13T11-00-00-000Z_b_2.jsonl",
		`{"type":"session","version":3,"id":"b_2","timestamp":"2026-11-13T11:00:00.500Z","cwd":"/work/proj","parentSession":"/s/x.jsonl"}`+"\n"+`{"type":"message"}`+"\n", base.Add(time.Hour))
	broken := write(t, dir, "2026-11-13T09-00-00-000Z_c3.jsonl", "not json\n", base)
	write(t, dir, "notes.jsonl", "{}\n", base)
	write(t, dir, "2026-11-13T09-00-00-000Z_d4.txt", "{}\n", base)
	if err := os.Mkdir(filepath.Join(dir, "2026-11-13T09-00-00-000Z_e5.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := discover.DiscoverSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []discover.Session{
		{Path: newer, ID: "b_2", Cwd: "/work/proj", Started: base.Add(time.Hour + 500*time.Millisecond), ParentSession: "/s/x.jsonl", ModTime: base.Add(time.Hour)},
		// Equal ModTime: Path ascending.
		{Path: older, ID: "a1", Cwd: "/work/proj", Started: base, ModTime: base},
		{Path: broken, ID: "c3", ModTime: base},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	// The first two share no ModTime with each other; older and broken do.
	if broken < older {
		want[1], want[2] = want[2], want[1]
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Path != w.Path || g.ID != w.ID || g.Cwd != w.Cwd || !g.Started.Equal(w.Started) ||
			g.ParentSession != w.ParentSession || !g.ModTime.Equal(w.ModTime) {
			t.Errorf("[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestDiscoverSessions_MissingDir(t *testing.T) {
	got, err := discover.DiscoverSessions(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want empty, nil", got, err)
	}
}
