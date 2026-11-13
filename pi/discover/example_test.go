package discover_test

import (
	"github.com/kylesnowschwartz/agent-ouija/pi/discover"
	"github.com/kylesnowschwartz/agent-ouija/pi/pidir"
)

func ExampleDiscoverSessions() {
	root, err := pidir.DefaultRoot()
	if err != nil {
		return
	}
	dir := discover.SessionDirForCwd(root.SessionsDir(), "/work/proj")
	if override, _ := pidir.SessionDirOverride(); override != "" {
		dir = override // flat: filter on Session.Cwd
	}
	sessions, _ := discover.DiscoverSessions(dir)
	for _, s := range sessions {
		_ = s // newest first: path, id, header cwd, start time
	}
}
