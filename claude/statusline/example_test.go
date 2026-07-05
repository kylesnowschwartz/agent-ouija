package statusline_test

import (
	"os"

	"github.com/kylesnowschwartz/agent-ouija/claude/statusline"
)

// A statusline binary decodes the tick payload from stdin.
func ExampleDecode() {
	p, err := statusline.Decode(os.Stdin)
	if err != nil || p == nil {
		return
	}
	if p.Model != nil {
		_ = p.Model.DisplayName
	}
	if cw := p.ContextWindow; cw != nil && cw.CurrentUsage != nil {
		used := cw.CurrentUsage.InputTokens + cw.CurrentUsage.CacheCreationInputTokens + cw.CurrentUsage.CacheReadInputTokens
		_ = used // used_percentage = used / cw.Size
	}
}
