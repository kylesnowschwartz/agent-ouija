package claudedir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectDirFor(t *testing.T) {
	root := Root("/home/u/.claude")

	tests := []struct {
		name    string
		path    string
		wantDir string // just the encoded part after {root}/projects/
	}{
		{"plain path", "/Users/kyle/Code/proj", "-Users-kyle-Code-proj"},
		{"dotfile path", "/Users/kyle/.config/nvim", "-Users-kyle--config-nvim"},
		{"worktree with .claude", "/Users/kyle/Code/proj/.claude/worktrees/wt", "-Users-kyle-Code-proj--claude-worktrees-wt"},
		{"underscore in path", "/private/var/folders/s0/abc_def/T/proj", "-private-var-folders-s0-abc-def-T-proj"},
		{"dots in project name", "/Users/kyle/Code/my.project.name", "-Users-kyle-Code-my-project-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := root.ProjectDirFor(tt.path)
			want := filepath.Join(root.ProjectsDir(), tt.wantDir)
			if dir != want {
				t.Errorf("got  %q\nwant %q", dir, want)
			}
		})
	}
}

func TestEncodeProjectPath(t *testing.T) {
	got := EncodeProjectPath("/Users/kyle/Code/my_projects/gear.shifter")
	want := "-Users-kyle-Code-my-projects-gear-shifter"
	if got != want {
		t.Errorf("EncodeProjectPath = %q, want %q", got, want)
	}
}

func TestDebugLogPath(t *testing.T) {
	tmp := t.TempDir()
	root := Root(filepath.Join(tmp, ".claude"))

	uuid := "abc12345-6789-0abc-def0-123456789abc"
	sessionPath := filepath.Join(tmp, uuid+".jsonl")

	// Debug file absent: empty result.
	if got := root.DebugLogPath(sessionPath); got != "" {
		t.Errorf("expected empty when debug file missing, got %q", got)
	}

	// Debug file present: full path returned.
	debugDir := filepath.Join(root.String(), "debug")
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(debugDir, uuid+".txt")
	if err := os.WriteFile(want, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := root.DebugLogPath(sessionPath); got != want {
		t.Errorf("DebugLogPath = %q, want %q", got, want)
	}

	// Non-.jsonl file: empty result.
	if got := root.DebugLogPath(filepath.Join(tmp, "not-a-session.txt")); got != "" {
		t.Errorf("expected empty for non-.jsonl file, got %q", got)
	}
}

func TestDefaultRoot_HonorsClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude-state")
	root, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if root.String() != "/custom/claude-state" {
		t.Errorf("root = %q, want /custom/claude-state (CLAUDE_CONFIG_DIR must win)", root)
	}
	// Everything must hang off the override, matching Claude Code's own
	// resolver (2.1.201: projects/, sessions/, settings.json all derive
	// from CLAUDE_CONFIG_DIR ?? $HOME/.claude).
	if root.ProjectsDir() != "/custom/claude-state/projects" {
		t.Errorf("ProjectsDir = %q", root.ProjectsDir())
	}

	t.Setenv("CLAUDE_CONFIG_DIR", "")
	root, err = DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot without override: %v", err)
	}
	if !strings.HasSuffix(root.String(), ".claude") {
		t.Errorf("root = %q, want $HOME/.claude fallback", root)
	}
}

func TestSessionTranscriptPath(t *testing.T) {
	root := Root("/home/kyle/.claude")
	got := root.SessionTranscriptPath("/Users/kyle/Code/proj", "abc-123")
	want := "/home/kyle/.claude/projects/-Users-kyle-Code-proj/abc-123.jsonl"
	if got != want {
		t.Errorf("SessionTranscriptPath = %q, want %q", got, want)
	}
}

func TestNewestTranscript(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	for _, p := range []string{old, newer} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Distinct mtimes without sleeping.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	// Noise that must be ignored: directories and non-jsonl files.
	if err := os.MkdirAll(filepath.Join(dir, "newest-dir.jsonl.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := NewestTranscript(dir)
	if err != nil {
		t.Fatalf("NewestTranscript: %v", err)
	}
	if got != newer {
		t.Errorf("NewestTranscript = %q, want %q", got, newer)
	}

	// Empty project dir: os.ErrNotExist.
	if _, err := NewestTranscript(t.TempDir()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty dir: err = %v, want os.ErrNotExist", err)
	}
}
