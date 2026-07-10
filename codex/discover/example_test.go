package discover_test

import (
	"github.com/kylesnowschwartz/agent-ouija/codex/codexdir"
	"github.com/kylesnowschwartz/agent-ouija/codex/discover"
)

func ExampleDiscoverRollouts() {
	root, err := codexdir.DefaultRoot()
	if err != nil {
		return
	}
	rollouts, _ := discover.DiscoverRollouts(root.SessionsDir())
	names, _ := discover.ThreadNames(root.SessionIndexPath())
	for _, r := range rollouts {
		_ = names[r.SessionID] // newest first: rollout path, session id, thread name
	}
}
