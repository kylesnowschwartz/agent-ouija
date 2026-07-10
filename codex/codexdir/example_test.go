package codexdir_test

import (
	"fmt"

	"github.com/kylesnowschwartz/agent-ouija/codex/codexdir"
)

func ExampleRoot() {
	root := codexdir.Root("/home/u/.codex")
	fmt.Println(root.SessionsDir())
	fmt.Println(root.SessionIndexPath())
	// Output:
	// /home/u/.codex/sessions
	// /home/u/.codex/session_index.jsonl
}
