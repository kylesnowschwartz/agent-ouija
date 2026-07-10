package codexdir_test

import (
	"path/filepath"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/codexdir"
)

func TestRootPaths(t *testing.T) {
	root := codexdir.Root("/home/u/.codex")

	if got, want := root.String(), "/home/u/.codex"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := root.SessionsDir(), filepath.Join("/home/u/.codex", "sessions"); got != want {
		t.Errorf("SessionsDir() = %q, want %q", got, want)
	}
	if got, want := root.SessionIndexPath(), filepath.Join("/home/u/.codex", "session_index.jsonl"); got != want {
		t.Errorf("SessionIndexPath() = %q, want %q", got, want)
	}
}

func TestDefaultRoot_CodexHomeOverride(t *testing.T) {
	t.Setenv("CODEX_HOME", "/custom/codex/home")
	root, err := codexdir.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := root.String(), "/custom/codex/home"; got != want {
		t.Errorf("DefaultRoot() = %q, want %q", got, want)
	}
}

func TestDefaultRoot_FallsBackToHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOME", "/home/fallback")
	root, err := codexdir.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := root.String(), filepath.Join("/home/fallback", ".codex"); got != want {
		t.Errorf("DefaultRoot() = %q, want %q", got, want)
	}
}
