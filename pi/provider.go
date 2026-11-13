// Package pi implements the sessions.Provider interface for the Pi
// coding agent -- a thin adapter over the native packages (pidir,
// discover, session). Pi-specific consumers import those packages
// directly; this adapter exists for cross-provider consumers.
package pi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/pi/discover"
	"github.com/kylesnowschwartz/agent-ouija/pi/pidir"
	"github.com/kylesnowschwartz/agent-ouija/pi/session"
)

// ongoingStaleness is how long a Working session may go unwritten
// before Discover stops reporting it as ongoing: a killed Pi process
// never appends the assistant message that ends its run.
const ongoingStaleness = 2 * time.Minute

// Provider adapts a Pi agent directory to the sessions core. It reads
// the per-cwd folders under Root.SessionsDir; a flat
// $PI_CODING_AGENT_SESSION_DIR directory is not read.
//
// It does not implement sessions.LiveTracker: Pi keeps no on-disk
// live-process registry.
type Provider struct {
	root pidir.Root
}

var _ sessions.Provider = (*Provider)(nil)

// New returns a provider rooted at the given Pi agent directory.
func New(root pidir.Root) *Provider { return &Provider{root: root} }

// Name implements sessions.Provider.
func (p *Provider) Name() string { return "pi" }

// Root returns the underlying agent directory.
func (p *Provider) Root() pidir.Root { return p.root }

// Discover implements sessions.Provider. A ProjectDir query reads only
// that cwd's folder; otherwise every folder, including symlinked ones,
// is read. Files are taken newest first, and each is read in full for
// its session name (session.ReadSummary) and its tail for its status
// (session.TrailingState) only until Limit refs are collected. An
// unreadable folder is skipped and an unreadable file reports a zero
// name and status, so one bad entry cannot hide the rest.
func (p *Provider) Discover(q sessions.Query) ([]sessions.SessionRef, error) {
	var dirs []string
	if q.ProjectDir != "" {
		dirs = []string{discover.SessionDirForCwd(p.root.SessionsDir(), q.ProjectDir)}
	} else {
		entries, err := os.ReadDir(p.root.SessionsDir())
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, e := range entries {
			dir := filepath.Join(p.root.SessionsDir(), e.Name())
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				dirs = append(dirs, dir)
			}
		}
	}

	var files []discover.Session
	for _, dir := range dirs {
		found, err := discover.DiscoverSessions(dir)
		if err != nil {
			continue
		}
		for _, f := range found {
			// The folder encoding is lossy; the header cwd decides.
			if q.ProjectDir == "" || f.Cwd == q.ProjectDir {
				files = append(files, f)
			}
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })

	title := strings.ToLower(strings.TrimSpace(q.Title))
	refs := []sessions.SessionRef{}
	for _, f := range files {
		if q.Limit > 0 && len(refs) == q.Limit {
			break
		}
		sum, _, _ := session.ReadSummary(f.Path, 0, session.Summary{})
		if title != "" && !strings.Contains(strings.ToLower(sum.Name), title) {
			continue
		}
		state, _ := session.TrailingState(f.Path)
		refs = append(refs, sessions.SessionRef{
			Provider: p.Name(),
			ID:       f.ID,
			Path:     f.Path,
			Title:    sum.Name,
			CWD:      f.Cwd,
			ModTime:  f.ModTime,
			Ongoing:  state.Status == session.Working && time.Since(f.ModTime) <= ongoingStaleness,
		})
	}
	return refs, nil
}
