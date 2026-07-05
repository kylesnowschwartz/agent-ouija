package claudedir_test

import (
	"fmt"

	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
)

func ExampleEncodeProjectPath() {
	fmt.Println(claudedir.EncodeProjectPath("/Users/kyle/.config/nvim"))
	// Output: -Users-kyle--config-nvim
}

func ExampleRoot() {
	root := claudedir.Root("/home/u/.claude")
	fmt.Println(root.ProjectDirFor("/work/proj"))
	fmt.Println(root.SettingsPath())
	// Output:
	// /home/u/.claude/projects/-work-proj
	// /home/u/.claude/settings.json
}
