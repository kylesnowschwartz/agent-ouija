package pidir_test

import (
	"path/filepath"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/pi/pidir"
)

func TestRootPaths(t *testing.T) {
	root := pidir.Root("/home/u/.pi/agent")
	if got, want := root.String(), "/home/u/.pi/agent"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := root.SessionsDir(), filepath.Join("/home/u/.pi/agent", "sessions"); got != want {
		t.Errorf("SessionsDir() = %q, want %q", got, want)
	}
}

func TestDefaultRoot(t *testing.T) {
	tests := []struct {
		name, env, want string
	}{
		{"unset falls back to home", "", filepath.Join("/home/fallback", ".pi", "agent")},
		{"override", "/custom/pi", "/custom/pi"},
		{"tilde alone", "~", "/home/fallback"},
		{"tilde prefix", "~/pi-state", filepath.Join("/home/fallback", "pi-state")},
		{"tilde mid-path kept", "/a/~/b", "/a/~/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", "/home/fallback")
			t.Setenv("PI_CODING_AGENT_DIR", tt.env)
			root, err := pidir.DefaultRoot()
			if err != nil {
				t.Fatal(err)
			}
			if got := root.String(); got != tt.want {
				t.Errorf("DefaultRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionDirOverride(t *testing.T) {
	t.Setenv("HOME", "/home/fallback")
	t.Setenv("PI_CODING_AGENT_DIR", "/custom/pi")

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	if got, err := pidir.SessionDirOverride(); err != nil || got != "" {
		t.Errorf("unset: got %q, %v; want \"\"", got, err)
	}

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "~/flat")
	if got, err := pidir.SessionDirOverride(); err != nil || got != filepath.Join("/home/fallback", "flat") {
		t.Errorf("tilde: got %q, %v", got, err)
	}

	// The session override does not move the agent root.
	root, err := pidir.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root.String() != "/custom/pi" {
		t.Errorf("DefaultRoot() = %q, want /custom/pi", root)
	}
}
