package sessionstest_test

import (
	"errors"
	"fmt"
	"testing"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/sessionstest"
)

// The fake provider must pass the same conformance suite as real
// providers — it is the reference for the query semantics.
func TestFakeProviderConformance(t *testing.T) {
	sessionstest.Run(t, sessionstest.Harness{
		Make: func(t *testing.T, seeds []sessionstest.Seed) sessions.Provider {
			f := &sessionstest.FakeProvider{}
			for i, s := range seeds {
				f.Refs = append(f.Refs, sessions.SessionRef{
					ID:      fmt.Sprintf("fake-%d", i),
					Title:   s.Title,
					CWD:     s.ProjectDir,
					ModTime: s.ModTime,
				})
			}
			return f
		},
	})
}

func TestFakeProviderDiscoverErr(t *testing.T) {
	boom := errors.New("boom")
	f := &sessionstest.FakeProvider{DiscoverErr: boom}
	if _, err := f.Discover(sessions.Query{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}
