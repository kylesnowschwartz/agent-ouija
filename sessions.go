// Package sessions defines the thin provider-neutral core of agent-ouija:
// just enough surface for a cross-provider consumer to enumerate agent
// sessions, plus optional capability interfaces discovered by type
// assertion.
//
// Everything lossless and provider-specific lives in ordinary subpackages
// (claude/transcript, claude/discover, ...) that provider-specific
// consumers import directly. This core is deliberately NOT a funnel for
// rich data: forcing a provider's full fidelity through a neutral
// interface loses information (the lossy-normalization mistake). A
// consumer that knows it is talking to Claude Code should use the claude
// packages; a consumer that works across providers uses this one.
//
// Capabilities follow the io.ReaderAt pattern: a provider advertises an
// optional capability by implementing its interface, and consumers probe
// with a type assertion. A fat mandatory interface would force future
// providers to stub methods they cannot honor.
package sessions

import (
	"sort"
	"time"
)

// SessionRef is a provider-neutral reference to one recorded agent
// session: enough to list, rank, and open it — no conversation content.
type SessionRef struct {
	// Provider is the Name() of the provider that produced this ref.
	Provider string

	// ID is the provider-scoped session identifier.
	ID string

	// Path is the on-disk location of the session's primary artifact
	// (for Claude Code, the transcript JSONL). May be empty for
	// providers without a file-per-session model.
	Path string

	// Title is the human-readable session title, when one exists.
	Title string

	// CWD is the working directory the session ran in, when known.
	CWD string

	// ModTime is the last-activity timestamp used for recency ordering.
	ModTime time.Time

	// Ongoing reports whether the provider believes the session is still
	// in progress.
	Ongoing bool
}

// Query narrows a Discover call. The zero value matches everything.
type Query struct {
	// ProjectDir limits results to sessions belonging to this absolute
	// project path. Empty means all projects.
	ProjectDir string

	// Title filters to sessions whose title matches case-insensitively
	// (substring). Empty means no title filtering.
	Title string

	// Limit caps the number of results after sorting. 0 means no cap.
	Limit int
}

// Provider enumerates recorded sessions for one agent product.
type Provider interface {
	// Name returns a stable, lowercase provider identifier ("claude").
	Name() string

	// Discover returns sessions matching the query, sorted by ModTime
	// descending. A query matching nothing returns an empty slice, not
	// an error.
	Discover(Query) ([]SessionRef, error)
}

// LiveSession is a currently-running agent process associated with a
// session.
type LiveSession struct {
	Ref SessionRef // at minimum Provider, ID, and CWD are set
	PID int
}

// LiveTracker is an optional capability: providers that maintain a
// live-process registry can report which sessions are running right now.
// Probe with a type assertion:
//
//	if lt, ok := provider.(sessions.LiveTracker); ok { ... }
type LiveTracker interface {
	LiveSessions() ([]LiveSession, error)
}

// Registry aggregates providers and fans Discover out across them.
type Registry struct {
	providers []Provider
}

// NewRegistry returns a Registry over the given providers.
func NewRegistry(providers ...Provider) *Registry {
	return &Registry{providers: providers}
}

// Providers returns the registered providers in registration order.
func (r *Registry) Providers() []Provider {
	return r.providers
}

// Provider returns the registered provider with the given name.
func (r *Registry) Provider(name string) (Provider, bool) {
	for _, p := range r.providers {
		if p.Name() == name {
			return p, true
		}
	}
	return nil, false
}

// Discover queries every provider and merges the results, sorted by
// ModTime descending. Provider errors are skipped (a broken provider must
// not hide the others); the first error is returned alongside whatever was
// gathered.
func (r *Registry) Discover(q Query) ([]SessionRef, error) {
	var all []SessionRef
	var firstErr error
	for _, p := range r.providers {
		refs, err := p.Discover(q)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, refs...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].ModTime.After(all[j].ModTime)
	})
	if q.Limit > 0 && len(all) > q.Limit {
		all = all[:q.Limit]
	}
	return all, firstErr
}
