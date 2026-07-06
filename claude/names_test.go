package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude"
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/discover"
	"github.com/kylesnowschwartz/agent-ouija/claude/registry"
)

// restampedSessionID matches the sessionId inside testdata/restamped_title.jsonl.
const restampedSessionID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

// TestDisplayNameRestampedFixture is the end-to-end story behind the
// resolver (issue #1, verified against Claude Code 2.1.195): a launcher
// stamps custom-title with the project directory name on every flush, a
// /rename lands once mid-file, the next flush overwrites it. Discovery's
// last-occurrence-wins rule therefore reports the stamp; the rename
// survives only in the registry, and DisplayName must recover it.
func TestDisplayNameRestampedFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "restamped_title.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, restampedSessionID+".jsonl"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := discover.DiscoverProjectSessions(projDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	info := sessions[0]

	// The transcript alone reports the stamp — this is the documented
	// last-occurrence-wins behavior the resolver exists to correct.
	if info.Title != "my-project" {
		t.Fatalf("SessionInfo.Title = %q, want the stamp %q", info.Title, "my-project")
	}

	r := claude.NewNameResolver([]registry.Live{
		{PID: 12839, SessionID: restampedSessionID, Cwd: "/Users/x/Code/my-project", Name: "dependabot-manager-okr"},
	})
	if got := r.DisplayName(info); got != "dependabot-manager-okr" {
		t.Errorf("DisplayName = %q, want the /rename %q", got, "dependabot-manager-okr")
	}
}

func TestNewNameResolverTieBreak(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	info := discover.SessionInfo{SessionID: id}

	tests := []struct {
		name    string
		entries []registry.Live
		want    string
	}{
		{
			name: "freshest UpdatedAt wins",
			entries: []registry.Live{
				{PID: 100, SessionID: id, Name: "stale", UpdatedAt: 100, StartedAt: 999},
				{PID: 200, SessionID: id, Name: "fresh", UpdatedAt: 200, StartedAt: 1},
			},
			want: "fresh",
		},
		{
			name: "StartedAt breaks equal UpdatedAt",
			entries: []registry.Live{
				{PID: 100, SessionID: id, Name: "older-start", UpdatedAt: 100, StartedAt: 1},
				{PID: 200, SessionID: id, Name: "newer-start", UpdatedAt: 100, StartedAt: 2},
			},
			want: "newer-start",
		},
		{
			name: "PID breaks zeroed timestamps (format drift decodes to 0)",
			entries: []registry.Live{
				{PID: 200, SessionID: id, Name: "high-pid"},
				{PID: 100, SessionID: id, Name: "low-pid"},
			},
			want: "high-pid",
		},
		{
			name: "nameless entries never win",
			entries: []registry.Live{
				{PID: 100, SessionID: id, Name: "named", UpdatedAt: 1},
				{PID: 200, SessionID: id, Name: "", UpdatedAt: 999},
			},
			want: "named",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.NewNameResolver(tt.entries).DisplayName(info); got != tt.want {
				t.Errorf("DisplayName = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNameResolverKeepsDeadEntries is the regression guard for the
// no-liveness-filter rule: lingering registry files from exited sessions
// are the only surviving record of a /rename, and tail-claude's picker
// depends on them resolving. The resolver must never consult Live.Alive.
func TestNameResolverKeepsDeadEntries(t *testing.T) {
	const id = "22222222-2222-2222-2222-222222222222"
	// PID -1 can never be a live process; Alive() would reject it.
	r := claude.NewNameResolver([]registry.Live{
		{PID: -1, SessionID: id, Name: "renamed-after-exit"},
	})
	got := r.DisplayName(discover.SessionInfo{SessionID: id})
	if got != "renamed-after-exit" {
		t.Errorf("DisplayName = %q, want %q from the dead entry", got, "renamed-after-exit")
	}
}

func TestDisplayName(t *testing.T) {
	// A real repo layout for the worktree-basename arm: sessions in a
	// subdirectory of a git repo are stamped with the repo's name, not
	// the subdirectory's.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	repoName := filepath.Base(repo)

	const id = "33333333-3333-3333-3333-333333333333"
	tests := []struct {
		name  string
		info  discover.SessionInfo
		rname string // registry name for id ("" = no entry)
		want  string
	}{
		{
			name:  "genuine title beats registry name",
			info:  discover.SessionInfo{SessionID: id, Title: "hand-picked", Cwd: "/Users/x/Code/my-project"},
			rname: "auto-name-3f",
			want:  "hand-picked",
		},
		{
			name:  "untitled session takes registry name",
			info:  discover.SessionInfo{SessionID: id, Cwd: "/Users/x/Code/my-project"},
			rname: "my-project-3f",
			want:  "my-project-3f",
		},
		{
			name:  "cwd-basename stamp takes registry name",
			info:  discover.SessionInfo{SessionID: id, Title: "my-project", Cwd: "/Users/x/Code/my-project"},
			rname: "real-rename",
			want:  "real-rename",
		},
		{
			name:  "git-root-basename stamp takes registry name",
			info:  discover.SessionInfo{SessionID: id, Title: repoName, Cwd: nested},
			rname: "real-rename",
			want:  "real-rename",
		},
		{
			name: "stamp with no registry name is kept — beats a blank",
			info: discover.SessionInfo{SessionID: id, Title: "my-project", Cwd: "/Users/x/Code/my-project"},
			want: "my-project",
		},
		{
			name:  "composed launcher title is not a stamp",
			info:  discover.SessionInfo{SessionID: id, Title: "my-project · worktree-thing", Cwd: "/Users/x/Code/my-project"},
			rname: "real-rename",
			want:  "my-project · worktree-thing",
		},
		{
			name:  "empty cwd disables stamp detection",
			info:  discover.SessionInfo{SessionID: id, Title: "my-project"},
			rname: "real-rename",
			want:  "my-project",
		},
		{
			name: "no title and no registry name is empty",
			info: discover.SessionInfo{SessionID: id, Cwd: "/Users/x/Code/my-project"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entries []registry.Live
			if tt.rname != "" {
				entries = append(entries, registry.Live{PID: 1, SessionID: id, Name: tt.rname})
			}
			if got := claude.NewNameResolver(entries).DisplayName(tt.info); got != tt.want {
				t.Errorf("DisplayName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyOverlaysInPlace(t *testing.T) {
	sessions := []discover.SessionInfo{
		{SessionID: "s1", Title: "genuine", Cwd: "/Users/x/proj"},
		{SessionID: "s2", Title: "proj", Cwd: "/Users/x/proj"}, // stamped
		{SessionID: "s3", Cwd: "/Users/x/proj"},                // untitled
	}
	r := claude.NewNameResolver([]registry.Live{
		{PID: 1, SessionID: "s2", Name: "renamed"},
		{PID: 2, SessionID: "s3", Name: "proj-a1"},
	})
	r.Apply(sessions)
	want := []string{"genuine", "renamed", "proj-a1"}
	for i, w := range want {
		if sessions[i].Title != w {
			t.Errorf("sessions[%d].Title = %q, want %q", i, sessions[i].Title, w)
		}
	}
}

func TestApplyZeroResolverNoOps(t *testing.T) {
	sessions := []discover.SessionInfo{{SessionID: "s1", Title: "keep"}}
	var zero claude.NameResolver
	zero.Apply(sessions)
	if sessions[0].Title != "keep" {
		t.Errorf("zero-value Apply changed Title to %q", sessions[0].Title)
	}
}

func TestFindNameMatches(t *testing.T) {
	root := claudedir.Root(t.TempDir())

	// writeTranscript materializes an empty transcript file where the
	// registry entry's (cwd, sessionID) pair says it must live.
	writeTranscript := func(cwd, sessionID string) {
		t.Helper()
		path := root.SessionTranscriptPath(cwd, sessionID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const (
		idExact   = "44444444-4444-4444-4444-444444444444"
		idPartial = "55555555-5555-5555-5555-555555555555"
		idNoFile  = "66666666-6666-6666-6666-666666666666"
		idRenamed = "77777777-7777-7777-7777-777777777777"
	)
	writeTranscript("/Users/x/a", idExact)
	writeTranscript("/Users/x/b", idPartial)
	writeTranscript("/Users/x/d", idRenamed)

	entries := []registry.Live{
		{PID: 1, SessionID: idExact, Cwd: "/Users/x/a", Name: "dependabot-manager-okr"},
		{PID: 2, SessionID: idPartial, Cwd: "/Users/x/b", Name: "dependabot-triage"},
		{PID: 3, SessionID: idNoFile, Cwd: "/Users/x/c", Name: "dependabot-ghost"},
		// A resumed session: the stale pre-rename entry must not match.
		{PID: 4, SessionID: idRenamed, Cwd: "/Users/x/d", Name: "old-name", UpdatedAt: 1},
		{PID: 5, SessionID: idRenamed, Cwd: "/Users/x/d", Name: "new-name", UpdatedAt: 2},
	}

	t.Run("exact beats substring", func(t *testing.T) {
		got := claude.FindNameMatches("dependabot-manager-okr", root, entries)
		if len(got) != 1 || got[0].SessionID != idExact {
			t.Fatalf("got %+v, want the single exact match", got)
		}
		if got[0].Path != root.SessionTranscriptPath("/Users/x/a", idExact) {
			t.Errorf("Path = %q, want the transcript path", got[0].Path)
		}
	})

	t.Run("substring matches skip sessions without transcripts", func(t *testing.T) {
		got := claude.FindNameMatches("dependabot", root, entries)
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2 (ghost has no transcript)", len(got))
		}
		for _, r := range got {
			if r.SessionID == idNoFile {
				t.Errorf("matched %s, whose transcript does not exist", idNoFile)
			}
		}
	})

	t.Run("stale duplicate name does not match", func(t *testing.T) {
		if got := claude.FindNameMatches("old-name", root, entries); len(got) != 0 {
			t.Errorf("got %+v, want none — the tie-break winner is new-name", got)
		}
		if got := claude.FindNameMatches("new-name", root, entries); len(got) != 1 {
			t.Errorf("got %+v, want the winning entry", got)
		}
	})

	t.Run("blank query matches nothing", func(t *testing.T) {
		if got := claude.FindNameMatches("  ", root, entries); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}
