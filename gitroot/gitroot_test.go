package gitroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGitRoot_NormalRepo(t *testing.T) {
	// Create a fake git repo with a .git directory.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	got := ResolveGitRoot(dir)
	if got != dir {
		t.Errorf("ResolveGitRoot(%q) = %q, want %q", dir, got, dir)
	}
}

func TestResolveGitRoot_Worktree(t *testing.T) {
	// Simulate a git worktree:
	// mainRepo/.git/ (directory)
	// mainRepo/.git/worktrees/my-wt/ (directory)
	// worktreeDir/.git (file) -> "gitdir: mainRepo/.git/worktrees/my-wt"
	mainRepo := t.TempDir()
	worktreeDir := t.TempDir()

	// Create main repo .git dir and worktrees subdir.
	gitDir := filepath.Join(mainRepo, ".git")
	os.Mkdir(gitDir, 0755)
	os.MkdirAll(filepath.Join(gitDir, "worktrees", "my-wt"), 0755)

	// Create worktree .git file.
	gitdirPath := filepath.Join(gitDir, "worktrees", "my-wt")
	os.WriteFile(
		filepath.Join(worktreeDir, ".git"),
		[]byte("gitdir: "+gitdirPath+"\n"),
		0644,
	)

	got := ResolveGitRoot(worktreeDir)
	if got != mainRepo {
		t.Errorf("ResolveGitRoot(%q) = %q, want %q (main repo)", worktreeDir, got, mainRepo)
	}
}

func TestResolveGitRoot_SubdirOfWorktree(t *testing.T) {
	// ResolveGitRoot should walk up from a subdirectory.
	mainRepo := t.TempDir()
	worktreeDir := t.TempDir()

	gitDir := filepath.Join(mainRepo, ".git")
	os.Mkdir(gitDir, 0755)
	os.MkdirAll(filepath.Join(gitDir, "worktrees", "wt"), 0755)

	os.WriteFile(
		filepath.Join(worktreeDir, ".git"),
		[]byte("gitdir: "+filepath.Join(gitDir, "worktrees", "wt")+"\n"),
		0644,
	)

	subdir := filepath.Join(worktreeDir, "src", "pkg")
	os.MkdirAll(subdir, 0755)

	got := ResolveGitRoot(subdir)
	if got != mainRepo {
		t.Errorf("ResolveGitRoot(%q) = %q, want %q", subdir, got, mainRepo)
	}
}

func TestResolveGitRoot_NoGit(t *testing.T) {
	// No .git anywhere -- should return the original path.
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b")
	os.MkdirAll(subdir, 0755)

	got := ResolveGitRoot(subdir)
	if got != subdir {
		t.Errorf("ResolveGitRoot(%q) = %q, want original path", subdir, got)
	}
}

func TestResolveGitRoot_RealWorktree(t *testing.T) {
	// Integration test: use the actual worktree we're running in.
	// This test only runs if we detect we're in the tail-claude worktree.
	wtGit := filepath.Join("..", "..", "..", ".git")
	data, err := os.ReadFile(wtGit)
	if err != nil {
		t.Skip("not running from a git worktree")
	}
	content := string(data)
	if len(content) == 0 || content[0] == ' ' {
		t.Skip("not a worktree .git file")
	}

	// If we get here, we're in a worktree. ResolveGitRoot should
	// resolve to the main repo, not the worktree dir.
	cwd, _ := os.Getwd()
	resolved := ResolveGitRoot(cwd)

	// The resolved path should NOT contain ".claude/worktrees".
	if filepath.Base(filepath.Dir(filepath.Dir(resolved))) == ".claude" {
		t.Errorf("ResolveGitRoot still points to worktree: %s", resolved)
	}
}
