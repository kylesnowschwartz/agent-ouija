package settings_test

import (
	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/settings"
)

func ExampleRead() {
	root, err := claudedir.DefaultRoot()
	if err != nil {
		return
	}
	state := settings.Read(root.SettingsPath())
	_ = state.Model // "" when unset — callers render stateless, never error
	_ = state.Effort
}

func ExampleRegisterHooks() {
	root, err := claudedir.DefaultRoot()
	if err != nil {
		return
	}
	added, err := settings.RegisterHooks(root.SettingsPath(), []settings.HookCommand{
		{Event: "PermissionRequest", Command: "my-tool", Args: []string{"hook", "permission-request"}},
		{Event: "Stop", Command: "my-tool", Args: []string{"hook", "cleanup"}},
	})
	_ = added // nil when everything was already registered (idempotent)
	_ = err
}
