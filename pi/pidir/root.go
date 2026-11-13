// Package pidir locates the on-disk state of a Pi coding agent
// installation.
//
// All paths hang off a Root -- the directory Pi calls its agent
// directory (conventionally ~/.pi/agent). The library never resolves the
// home directory implicitly; construct a Root explicitly (or via
// DefaultRoot) and pass it down. Mirrors codex/codexdir's Root
// convention.
package pidir

import (
	"os"
	"path/filepath"
	"strings"
)

// Root is a Pi agent directory (conventionally ~/.pi/agent).
type Root string

// DefaultRoot returns the root Pi itself would use:
// $PI_CODING_AGENT_DIR when set and non-empty, otherwise
// $HOME/.pi/agent. A leading "~" or "~/" in the variable expands to the
// home directory, as Pi expands it.
func DefaultRoot() (Root, error) {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		expanded, err := expandTilde(dir)
		return Root(expanded), err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return Root(filepath.Join(home, ".pi", "agent")), nil
}

// SessionDirOverride returns $PI_CODING_AGENT_SESSION_DIR with a leading
// "~" expanded, or "" when the variable is unset or empty.
//
// Pi picks a session directory in this order: the --session-dir flag,
// then $PI_CODING_AGENT_SESSION_DIR, then the "sessionDir" key of
// settings.json, then the per-cwd folder under Root.SessionsDir. An
// override directory is flat: it holds the session files of every cwd
// directly, with no per-cwd folders, and each file's header carries its
// cwd. This function reads only the environment variable; the flag is
// per process and settings.json is not read.
func SessionDirOverride() (string, error) {
	dir := os.Getenv("PI_CODING_AGENT_SESSION_DIR")
	if dir == "" {
		return "", nil
	}
	return expandTilde(dir)
}

// String returns the root directory path.
func (r Root) String() string { return string(r) }

// SessionsDir returns the directory holding one folder per working
// directory (see discover.SessionDirForCwd), each holding that cwd's
// session files: {root}/sessions/--work-proj--/<timestamp>_<id>.jsonl.
func (r Root) SessionsDir() string { return filepath.Join(string(r), "sessions") }

// expandTilde expands a path that is exactly "~" or starts with "~/".
func expandTilde(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, p[2:]), nil
}
