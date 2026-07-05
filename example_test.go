package sessions_test

import (
	"fmt"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/claude"
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
)

// Cross-provider consumers enumerate sessions through the neutral core;
// optional capabilities are probed by type assertion.
func ExampleRegistry() {
	root, err := claudedir.DefaultRoot()
	if err != nil {
		return
	}
	reg := sessions.NewRegistry(claude.New(root))

	refs, _ := reg.Discover(sessions.Query{ProjectDir: "/work/proj", Limit: 10})
	for _, r := range refs {
		fmt.Println(r.Provider, r.ID, r.Title)
	}

	if p, ok := reg.Provider("claude"); ok {
		if lt, ok := p.(sessions.LiveTracker); ok {
			live, _ := lt.LiveSessions()
			_ = live // currently-running sessions with pids
		}
	}
}
