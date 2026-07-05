package discover_test

import (
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/discover"
)

func ExampleDiscoverProjectSessions() {
	root, err := claudedir.DefaultRoot()
	if err != nil {
		return
	}
	sessions, _ := discover.DiscoverProjectSessions(root.ProjectDirFor("/work/proj"))
	for _, s := range sessions {
		_ = s.Title // newest first: title, preview, turn count, ongoing flag, ...
	}
}
