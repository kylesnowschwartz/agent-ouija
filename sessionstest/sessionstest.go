// Package sessionstest provides a fake provider and a conformance suite
// for implementations of the sessions core interfaces.
//
// Both FakeProvider and claude.Provider pass Run — that shared contract is
// the architecture proof for the provider abstraction: a future provider
// (codex, pi, ...) is correct when its harness passes the same suite.
package sessionstest

import (
	"sort"
	"strings"
	"testing"
	"time"

	sessions "github.com/kylesnowschwartz/agent-ouija"
)

// FakeProvider is an in-memory sessions.Provider (and LiveTracker) for
// tests and for consumers exercising cross-provider code paths.
type FakeProvider struct {
	// ProviderName is returned by Name. Defaults to "fake".
	ProviderName string

	// Refs are the sessions Discover filters and returns. The Provider
	// field of each ref is overwritten with ProviderName.
	Refs []sessions.SessionRef

	// Live is returned by LiveSessions.
	Live []sessions.LiveSession

	// DiscoverErr, when set, is returned by every Discover call.
	DiscoverErr error
}

var (
	_ sessions.Provider    = (*FakeProvider)(nil)
	_ sessions.LiveTracker = (*FakeProvider)(nil)
)

// Name implements sessions.Provider.
func (f *FakeProvider) Name() string {
	if f.ProviderName == "" {
		return "fake"
	}
	return f.ProviderName
}

// Discover implements sessions.Provider with the canonical query
// semantics: ProjectDir equality (against SessionRef.CWD), case-insensitive
// substring title match, ModTime-descending order, Limit cap.
func (f *FakeProvider) Discover(q sessions.Query) ([]sessions.SessionRef, error) {
	if f.DiscoverErr != nil {
		return nil, f.DiscoverErr
	}
	title := strings.ToLower(strings.TrimSpace(q.Title))
	refs := make([]sessions.SessionRef, 0, len(f.Refs))
	for _, r := range f.Refs {
		if q.ProjectDir != "" && r.CWD != q.ProjectDir {
			continue
		}
		if title != "" && !strings.Contains(strings.ToLower(r.Title), title) {
			continue
		}
		r.Provider = f.Name()
		refs = append(refs, r)
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].ModTime.After(refs[j].ModTime)
	})
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// LiveSessions implements sessions.LiveTracker.
func (f *FakeProvider) LiveSessions() ([]sessions.LiveSession, error) {
	return f.Live, nil
}

// Seed describes one session the harness must materialize before the
// suite queries the provider under test.
type Seed struct {
	// ProjectDir is the absolute project path the session belongs to.
	ProjectDir string

	// Title is the session's title. Empty means untitled.
	Title string

	// ModTime is the session's last-activity time. The suite relies on
	// distinct values for ordering assertions.
	ModTime time.Time
}

// Harness adapts a provider implementation to the conformance suite.
type Harness struct {
	// Make returns a fresh provider with the given sessions
	// materialized (for claude, that means real JSONL files under a
	// temp root; for a fake, in-memory refs).
	Make func(t *testing.T, seeds []Seed) sessions.Provider
}

// Run executes the provider conformance suite against the harness.
func Run(t *testing.T, h Harness) {
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	projA := "/proj/alpha"
	projB := "/proj/beta"
	seeds := []Seed{
		{ProjectDir: projA, Title: "refactor parser", ModTime: base.Add(3 * time.Hour)},
		{ProjectDir: projA, Title: "Fix Login Bug", ModTime: base.Add(2 * time.Hour)},
		{ProjectDir: projB, Title: "beta planning", ModTime: base.Add(1 * time.Hour)},
	}

	t.Run("Name", func(t *testing.T) {
		p := h.Make(t, nil)
		if p.Name() == "" {
			t.Fatal("Name() must be non-empty")
		}
		if p.Name() != strings.ToLower(p.Name()) {
			t.Errorf("Name() = %q, want lowercase", p.Name())
		}
	})

	t.Run("DiscoverAll_SortedByModTimeDesc", func(t *testing.T) {
		p := h.Make(t, seeds)
		refs, err := p.Discover(sessions.Query{})
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != len(seeds) {
			t.Fatalf("len = %d, want %d: %+v", len(refs), len(seeds), refs)
		}
		for i := 1; i < len(refs); i++ {
			if refs[i].ModTime.After(refs[i-1].ModTime) {
				t.Errorf("refs not sorted ModTime desc at %d: %v after %v", i, refs[i].ModTime, refs[i-1].ModTime)
			}
		}
		for _, r := range refs {
			if r.Provider != p.Name() {
				t.Errorf("ref.Provider = %q, want %q", r.Provider, p.Name())
			}
			if r.ID == "" {
				t.Error("ref.ID must be non-empty")
			}
		}
	})

	t.Run("DiscoverByProject", func(t *testing.T) {
		p := h.Make(t, seeds)
		refs, err := p.Discover(sessions.Query{ProjectDir: projA})
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != 2 {
			t.Fatalf("len = %d, want 2 (only %s sessions): %+v", len(refs), projA, refs)
		}
	})

	t.Run("DiscoverByTitle_CaseInsensitive", func(t *testing.T) {
		p := h.Make(t, seeds)
		refs, err := p.Discover(sessions.Query{Title: "fix login"})
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != 1 || !strings.EqualFold(refs[0].Title, "Fix Login Bug") {
			t.Fatalf("refs = %+v, want the Fix Login Bug session", refs)
		}
	})

	t.Run("DiscoverLimit", func(t *testing.T) {
		p := h.Make(t, seeds)
		refs, err := p.Discover(sessions.Query{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != 2 {
			t.Fatalf("len = %d, want 2 (Limit)", len(refs))
		}
		// The cap must keep the NEWEST sessions.
		if refs[0].Title != "refactor parser" {
			t.Errorf("Limit must keep newest-first order, got %+v", refs)
		}
	})

	t.Run("DiscoverUnknownProject_EmptyNotError", func(t *testing.T) {
		p := h.Make(t, seeds)
		refs, err := p.Discover(sessions.Query{ProjectDir: "/does/not/exist"})
		if err != nil {
			t.Fatalf("unknown project must not error: %v", err)
		}
		if len(refs) != 0 {
			t.Fatalf("len = %d, want 0", len(refs))
		}
	})

	t.Run("LiveTrackerCapability", func(t *testing.T) {
		p := h.Make(t, nil)
		lt, ok := p.(sessions.LiveTracker)
		if !ok {
			t.Skip("provider does not implement LiveTracker")
		}
		if _, err := lt.LiveSessions(); err != nil {
			t.Fatalf("LiveSessions: %v", err)
		}
	})
}
