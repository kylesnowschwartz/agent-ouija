package gitroot_test

import (
	"github.com/kylesnowschwartz/agent-ouija/gitroot"
)

// Claude Code stores sessions under the MAIN working tree's path, so a
// consumer running inside a git worktree must resolve back to the main
// root before encoding a project directory.
func ExampleResolveGitRoot() {
	main := gitroot.ResolveGitRoot("/repo/.worktrees/feature-x")
	_ = main // "/repo" when the worktree metadata resolves; input path otherwise
}
