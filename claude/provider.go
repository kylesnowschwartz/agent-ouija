// Package claude implements the sessions.Provider interface for Claude
// Code — a thin adapter over the native packages (claudedir, discover,
// registry). Claude-specific consumers should import those packages
// directly; this adapter exists for cross-provider consumers and as the
// reference implementation of the core interfaces.
package claude

import (
	"os"
	"strings"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/discover"
	"github.com/kylesnowschwartz/agent-ouija/claude/registry"
)

// Provider adapts a Claude Code state directory to the sessions core.
type Provider struct {
	root claudedir.Root
}

var (
	_ sessions.Provider    = (*Provider)(nil)
	_ sessions.LiveTracker = (*Provider)(nil)
)

// New returns a provider rooted at the given Claude Code state directory.
func New(root claudedir.Root) *Provider {
	return &Provider{root: root}
}

// Name implements sessions.Provider.
func (p *Provider) Name() string { return "claude" }

// Root returns the underlying state directory, for callers that want to
// drop down to the native claude packages.
func (p *Provider) Root() claudedir.Root { return p.root }

// Discover implements sessions.Provider.
func (p *Provider) Discover(q sessions.Query) ([]sessions.SessionRef, error) {
	var dirs []string
	if q.ProjectDir != "" {
		dirs = []string{p.root.ProjectDirFor(q.ProjectDir)}
	} else {
		all, err := p.root.ListProjectDirs()
		if err != nil {
			if os.IsNotExist(err) {
				return []sessions.SessionRef{}, nil
			}
			return nil, err
		}
		dirs = all
	}

	infos, err := discover.DiscoverAllProjectSessions(dirs)
	if err != nil {
		return nil, err
	}

	title := strings.ToLower(strings.TrimSpace(q.Title))
	refs := make([]sessions.SessionRef, 0, len(infos))
	for _, si := range infos {
		if title != "" && !strings.Contains(strings.ToLower(si.Title), title) {
			continue
		}
		refs = append(refs, sessions.SessionRef{
			Provider: p.Name(),
			ID:       si.SessionID,
			Path:     si.Path,
			Title:    si.Title,
			CWD:      si.Cwd,
			ModTime:  si.ModTime,
			Ongoing:  si.IsOngoing,
		})
	}
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// LiveSessions implements the sessions.LiveTracker capability, backed by
// Claude Code's live-process registry ({root}/sessions/*.json). Only
// entries whose process is still alive are returned.
func (p *Provider) LiveSessions() ([]sessions.LiveSession, error) {
	entries := registry.Read(p.root.SessionsDir())
	live := make([]sessions.LiveSession, 0, len(entries))
	for _, e := range entries {
		if !e.Alive() {
			continue
		}
		live = append(live, sessions.LiveSession{
			Ref: sessions.SessionRef{
				Provider: p.Name(),
				ID:       e.SessionID,
				CWD:      e.Cwd,
			},
			PID: e.PID,
		})
	}
	return live, nil
}
