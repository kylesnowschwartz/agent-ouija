// Package codex implements the sessions.Provider interface for Codex
// CLI -- a thin adapter over the native packages (codexdir, discover,
// rollout). Codex-specific consumers should import those packages
// directly; this adapter exists for cross-provider consumers, mirroring
// claude.Provider as the reference implementation of the core
// interfaces for a second agent product.
package codex

import (
	"os"
	"sort"
	"strings"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/codex/codexdir"
	"github.com/kylesnowschwartz/agent-ouija/codex/discover"
	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

// Provider adapts a Codex CLI state directory ($CODEX_HOME) to the
// sessions core.
//
// Unlike claude.Provider, it does not implement sessions.LiveTracker:
// Codex CLI keeps no on-disk live-process registry analogous to Claude
// Code's {root}/sessions/*.json. Codex liveness is a lifecycle-hook
// stream owned by the consuming app (tail-claude-mux's codexwatch),
// not anything this library can read from disk -- forcing a LiveTracker
// implementation here would mean inventing state that doesn't exist.
type Provider struct {
	root codexdir.Root
}

var _ sessions.Provider = (*Provider)(nil)

// New returns a provider rooted at the given Codex CLI state directory.
func New(root codexdir.Root) *Provider {
	return &Provider{root: root}
}

// Name implements sessions.Provider.
func (p *Provider) Name() string { return "codex" }

// Root returns the underlying state directory, for callers that want to
// drop down to the native codex packages.
func (p *Provider) Root() codexdir.Root { return p.root }

// Discover implements sessions.Provider. Every rollout transcript is
// opened once to fold its trailing {Status, Cwd} -- the same per-file
// cost profile as claude.Provider (which reads every session file for
// turn/ongoing metadata), though wider fan-out: Codex has no
// project-dir-encoding to narrow the walk the way claude's
// ProjectDirFor does, so a ProjectDir-scoped query still opens every
// rollout under the sessions tree to find matches.
func (p *Provider) Discover(q sessions.Query) ([]sessions.SessionRef, error) {
	files, err := discover.DiscoverRollouts(p.root.SessionsDir())
	if err != nil {
		return nil, err
	}
	names, err := discover.ThreadNames(p.root.SessionIndexPath())
	if err != nil {
		return nil, err
	}

	title := strings.ToLower(strings.TrimSpace(q.Title))
	refs := make([]sessions.SessionRef, 0, len(files))
	for _, f := range files {
		state := readTrailingState(f.Path)
		if q.ProjectDir != "" && state.Cwd != q.ProjectDir {
			continue
		}
		name := names[f.SessionID]
		if title != "" && !strings.Contains(strings.ToLower(name), title) {
			continue
		}
		refs = append(refs, sessions.SessionRef{
			Provider: p.Name(),
			ID:       f.SessionID,
			Path:     f.Path,
			Title:    name,
			CWD:      state.Cwd,
			ModTime:  f.ModTime,
			Ongoing:  state.Status == rollout.Running,
		})
	}

	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].ModTime.After(refs[j].ModTime)
	})
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// readTrailingState opens path and folds it into a rollout.State. Read
// errors (missing/unreadable file) fall back to the zero state (Idle, no
// cwd) rather than failing the whole Discover call -- a single unreadable
// transcript must not hide every other session.
func readTrailingState(path string) rollout.State {
	f, err := os.Open(path)
	if err != nil {
		return rollout.State{Status: rollout.Idle}
	}
	defer f.Close()
	state, _ := rollout.TrailingState(f)
	return state
}
