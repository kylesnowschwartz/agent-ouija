// Package codexdir locates the on-disk state of a Codex CLI installation.
//
// All paths hang off a Root -- the directory Codex CLI calls $CODEX_HOME
// (conventionally ~/.codex). The library never resolves the home
// directory implicitly; construct a Root explicitly (or via DefaultRoot)
// and pass it down. Mirrors claude/claudedir's Root convention for the
// claude subtree.
package codexdir

import (
	"os"
	"path/filepath"
)

// Root is a Codex CLI state directory (conventionally ~/.codex).
type Root string

// DefaultRoot returns the root Codex CLI itself would use: $CODEX_HOME
// when set, otherwise $HOME/.codex.
//
// Verified live against codex-cli 0.144.1 (2026-07-10).
func DefaultRoot() (Root, error) {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return Root(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return Root(filepath.Join(home, ".codex")), nil
}

// String returns the root directory path.
func (r Root) String() string { return string(r) }

// SessionsDir returns the directory holding rollout transcripts, nested
// YYYY/MM/DD (e.g. {root}/sessions/2026/07/10/rollout-<ts>-<uuid>.jsonl).
func (r Root) SessionsDir() string { return filepath.Join(string(r), "sessions") }

// SessionIndexPath returns the thread-name index file: JSONL lines of
// {"id", "thread_name", "updated_at"}, appended to over time -- the last
// line for a given id wins.
func (r Root) SessionIndexPath() string { return filepath.Join(string(r), "session_index.jsonl") }
