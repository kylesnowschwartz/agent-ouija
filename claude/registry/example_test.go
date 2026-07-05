package registry_test

import (
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/registry"
)

// Resolve which Claude session runs in a tmux pane: a registry pid inside
// the pane's process tree wins; cwd equality is the fallback.
func ExampleResolvePane() {
	root, err := claudedir.DefaultRoot()
	if err != nil {
		return
	}
	panePID, paneCwd := 12345, "/work/proj"
	if live, ok := registry.ResolvePane(root.SessionsDir(), panePID, paneCwd); ok {
		_ = live.SessionID // feed transcript.LastAssistantModel etc.
	}
}
