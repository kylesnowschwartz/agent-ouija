// Package claudedir locates the on-disk state of a Claude Code installation.
//
// All paths hang off a Root — the directory Claude Code calls ~/.claude.
// The library never resolves the home directory implicitly; construct a
// Root explicitly (or via DefaultRoot) and pass it down. This keeps every
// path computation testable against a temp directory.
package claudedir

import (
	"os"
	"path/filepath"
	"strings"
)

// Root is a Claude Code state directory (conventionally ~/.claude).
type Root string

// DefaultRoot returns the root Claude Code itself would use:
// $CLAUDE_CONFIG_DIR when set, otherwise $HOME/.claude.
//
// Verified against the Claude Code 2.1.201 bundle: its config-root
// resolver is CLAUDE_CONFIG_DIR ?? homedir()/.claude, and projects/,
// sessions/, and settings.json all derive from it — so honoring the
// override here keeps every consumer reading the directory Claude Code
// actually writes. One knowing divergence: Claude Code NFC-normalizes
// the path (JS String.normalize); stdlib-only Go cannot without x/text,
// which only matters for paths containing decomposed Unicode.
func DefaultRoot() (Root, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return Root(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return Root(filepath.Join(home, ".claude")), nil
}

// String returns the root directory path.
func (r Root) String() string { return string(r) }

// ProjectsDir returns the directory holding per-project session directories.
func (r Root) ProjectsDir() string { return filepath.Join(string(r), "projects") }

// SettingsPath returns the path of the global settings.json.
func (r Root) SettingsPath() string { return filepath.Join(string(r), "settings.json") }

// SessionsDir returns the live-session process registry directory.
func (r Root) SessionsDir() string { return filepath.Join(string(r), "sessions") }

// EncodeProjectPath encodes an absolute filesystem path into a Claude Code
// project directory name. Three characters are replaced with "-": path
// separators, dots, and underscores. The encoding is lossy (cannot be
// reversed for paths containing literal dashes).
//
// Verified empirically against Claude Code's on-disk output across 273
// project directories including dotfile paths (.claude, .config), worktree
// paths (.claude/worktrees/), and macOS temp paths (containing underscores).
//
// Symlinks are NOT resolved here — this is the pure string transform.
// Use Root.ProjectDirFor for the symlink-resolving variant that matches
// what Claude Code produces on disk.
func EncodeProjectPath(absPath string) string {
	r := strings.NewReplacer(
		string(filepath.Separator), "-",
		".", "-",
		"_", "-",
	)
	return r.Replace(absPath)
}

// ProjectDirFor returns the Claude Code projects directory for an absolute
// path. Example:
//
//	/Users/kyle/Code/proj -> {root}/projects/-Users-kyle-Code-proj
//	/Users/kyle/.config   -> {root}/projects/-Users-kyle--config
//
// Symlinks are resolved so the encoded path matches what Claude Code
// produces (e.g. macOS /tmp -> /private/tmp).
func (r Root) ProjectDirFor(absPath string) string {
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	return filepath.Join(r.ProjectsDir(), EncodeProjectPath(absPath))
}

// SessionTranscriptPath returns the transcript path for a session
// identified by its working directory and session ID:
//
//	{root}/projects/{encoded-cwd}/{sessionID}.jsonl
//
// This is the forward construction of the filename convention the rest of
// this module parses (discover, agents, DebugLogPath all trim ".jsonl").
// The natural input is a registry entry: registry.Resolve returns the
// (Cwd, SessionID) pair this completes. The path is where Claude Code
// writes the transcript; the file may not exist yet for a brand-new
// session.
func (r Root) SessionTranscriptPath(cwd, sessionID string) string {
	return filepath.Join(r.ProjectDirFor(cwd), sessionID+".jsonl")
}

// NewestTranscript returns the most recently modified session transcript
// (*.jsonl, non-recursive) in a project directory, conventionally one
// returned by ProjectDirFor. Statusline-style consumers use this to find
// "the current session" when nothing hands them a transcript path.
// Returns os.ErrNotExist when the directory holds no transcripts.
func NewestTranscript(projectDir string) (string, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", err
	}
	var newest string
	var newestTime int64
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".jsonl") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().UnixNano(); mt > newestTime {
			newestTime = mt
			newest = filepath.Join(projectDir, de.Name())
		}
	}
	if newest == "" {
		return "", os.ErrNotExist
	}
	return newest, nil
}

// ListProjectDirs returns every Claude Code project directory under
// {root}/projects. Used for name-based session lookup that spans projects;
// name resolution inside a single project should prefer ProjectDirFor.
func (r Root) ListProjectDirs() ([]string, error) {
	root := r.ProjectsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, filepath.Join(root, e.Name()))
	}
	return dirs, nil
}

// DebugLogPath returns the debug log file path for a given session JSONL
// path. Claude Code stores debug logs at {root}/debug/{session-uuid}.txt.
// Returns empty string if the debug file doesn't exist.
func (r Root) DebugLogPath(sessionPath string) string {
	// Extract the session UUID from the filename (strip .jsonl extension).
	base := filepath.Base(sessionPath)
	uuid := strings.TrimSuffix(base, ".jsonl")
	if uuid == "" || uuid == base {
		return "" // not a .jsonl file
	}

	debugPath := filepath.Join(string(r), "debug", uuid+".txt")
	if _, err := os.Stat(debugPath); err != nil {
		return ""
	}

	return debugPath
}
