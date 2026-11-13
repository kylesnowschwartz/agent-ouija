package pidir_test

import (
	"fmt"

	"github.com/kylesnowschwartz/agent-ouija/pi/pidir"
)

func ExampleRoot() {
	root := pidir.Root("/home/u/.pi/agent")
	fmt.Println(root.SessionsDir())
	// Output:
	// /home/u/.pi/agent/sessions
}
