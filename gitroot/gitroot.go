// Package gitroot resolves git repository roots, including worktrees and
// submodules, without invoking the git binary. Claude Code stores sessions
// under the MAIN working tree's path, so a consumer inside a worktree must
// resolve back to the main root before encoding a project directory.
package gitroot

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveGitRoot returns the git toplevel for the given directory. If the
// directory is inside a git worktree, it resolves to the main working tree
// root via the .git file's gitdir reference and commondir.
//
// Falls back to the original path if anything fails (not a git repo, etc).
func ResolveGitRoot(dir string) string {
	if root := findGitRepoRoot(dir); root != "" {
		return root
	}
	return dir
}

// FindRepoRoot is like ResolveGitRoot but returns the empty string, rather
// than the input, when dir is not inside a git repository.
func FindRepoRoot(dir string) string {
	return findGitRepoRoot(dir)
}

// findGitRepoRoot walks up from dir looking for .git. Handles both .git
// directories (normal repos) and .git files (worktrees/submodules). For
// .git files, reads the gitdir reference and resolves via commondir to
// find the main repository root.
//
// Returns empty string if no .git is found.
func findGitRepoRoot(dir string) string {
	if dir == "" {
		return ""
	}

	current := dir
	// If dir isn't a directory (e.g. a file path), start from its parent.
	if info, err := os.Stat(current); err == nil {
		if !info.IsDir() {
			current = filepath.Dir(current)
		}
	} else {
		// Path doesn't exist -- avoid walking non-paths.
		if !strings.ContainsRune(current, filepath.Separator) {
			return ""
		}
		current = filepath.Dir(current)
	}

	for {
		gitPath := filepath.Join(current, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				// Normal git repo -- this directory is the root.
				return current
			}
			if info.Mode().IsRegular() {
				// .git file -- worktree or submodule.
				if root := repoRootFromGitFile(current, gitPath); root != "" {
					return root
				}
				// Conservative fallback: treat the worktree directory as root.
				return current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// repoRootFromGitFile resolves the main repository root from a .git file.
// Reads the gitdir reference, then checks commondir to find the real .git
// directory. Falls back to parsing the worktrees path structure.
func repoRootFromGitFile(repoDir, gitFilePath string) string {
	gitDir := readGitDirFromFile(gitFilePath)
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Clean(filepath.Join(filepath.Dir(gitFilePath), gitDir))
	}

	commonDir := readCommonDir(gitDir)
	if commonDir != "" {
		if filepath.Base(commonDir) == ".git" {
			return filepath.Dir(commonDir)
		}
	}

	// Fallback: parse the worktrees path structure.
	// gitDir looks like /repo/.git/worktrees/<name>
	marker := string(filepath.Separator) + ".git" +
		string(filepath.Separator) + "worktrees" +
		string(filepath.Separator)
	if root, _, found := strings.Cut(gitDir, marker); found {
		if root != "" {
			return filepath.Clean(root)
		}
	}

	return repoDir
}

// readGitDirFromFile reads the "gitdir: <path>" reference from a .git file.
func readGitDirFromFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const prefix = "gitdir:"
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}

// readCommonDir reads the commondir file from a git directory (used by
// worktrees to reference the main repo's .git).
func readCommonDir(gitDir string) string {
	b, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(b))
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(gitDir, value))
}
