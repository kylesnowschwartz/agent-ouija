package hooks_test

import (
	"os"

	"github.com/kylesnowschwartz/agent-ouija/claude/hooks"
)

// A hook binary decodes its stdin payload once and switches on the event.
func ExampleDecode() {
	p, err := hooks.Decode(os.Stdin)
	if err != nil {
		return // hooks must never fail loudly: exit 0, no stderr
	}
	switch p.HookEventName {
	case "PermissionRequest":
		_ = p.ToolName
	case "SessionStart":
		_ = p.Source // startup | resume | clear | compact
	}
	_ = p.EffectiveSessionID() // falls back to $CLAUDE_CODE_SESSION_ID
}
