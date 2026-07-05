package claudedir

import (
	"os"
	"path/filepath"
	"testing"
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
