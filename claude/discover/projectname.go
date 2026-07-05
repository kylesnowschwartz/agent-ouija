package discover

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kylesnowschwartz/agent-ouija/gitroot"
)

// ProjectName returns a display name for a project directory.
//
// If cwd is inside a git repository (including worktrees and submodules),
// resolves to the main repository root directory name. For worktree
// directories named "project-branch", trims the branch suffix so the
// display shows the canonical project name.
//
// Falls back to filepath.Base(cwd) if no .git is found.
func ProjectName(cwd, gitBranch string) string {
	if cwd == "" {
		return ""
	}
	cleaned := filepath.Clean(cwd)

	if root := gitroot.FindRepoRoot(cleaned); root != "" {
		return filepath.Base(root)
	}

	// No git repo found. Try trimming branch suffix from the directory name
	// (handles offline worktree paths where the worktree directory exists
	// but its .git file points to a non-existent main repo).
	name := filepath.Base(cleaned)
	name = trimBranchSuffix(name, gitBranch)
	return name
}

// trimBranchSuffix strips a git branch name suffix from a directory name.
// Worktree directories are commonly named "project-branch-name". This
// normalizes back to the project name. Default branches (main, master,
// trunk, develop, dev) are not trimmed -- a directory called "project-main"
// is likely named intentionally.
func trimBranchSuffix(name, gitBranch string) string {
	branch := strings.TrimSpace(gitBranch)
	if name == "" || branch == "" {
		return name
	}
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branchToken := normalizeBranchToken(branch)
	if branchToken == "" {
		return name
	}
	if isDefaultBranch(branchToken) {
		return name
	}

	for _, sep := range []string{"-", "_"} {
		suffix := sep + branchToken
		if strings.HasSuffix(strings.ToLower(name), strings.ToLower(suffix)) {
			base := strings.TrimRight(name[:len(name)-len(suffix)], "-_")
			if base != "" {
				return base
			}
		}
	}
	return name
}

// normalizeBranchToken converts a branch name to a comparable token.
// Slashes, dashes, underscores, dots, and spaces become single dashes.
// Letters are lowered. Other characters become dashes.
func normalizeBranchToken(branch string) string {
	var b strings.Builder
	b.Grow(len(branch))

	lastDash := false
	for _, r := range branch {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '/' || r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// isDefaultBranch returns true for common default branch names that should
// not be trimmed from directory names.
func isDefaultBranch(branch string) bool {
	switch strings.ToLower(strings.TrimSpace(branch)) {
	case "main", "master", "trunk", "develop", "dev":
		return true
	default:
		return false
	}
}
