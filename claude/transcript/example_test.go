package transcript_test

import (
	"fmt"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

func ExampleReadSession() {
	chunks, err := transcript.ReadSession("../testdata/minimal.jsonl")
	if err != nil {
		panic(err)
	}
	for _, c := range chunks {
		fmt.Println(c.Type == transcript.UserChunk, len(c.Items))
	}
}

func ExampleExtractContentBlocks() {
	e, _ := transcript.ParseEntryLenient([]byte(`{"uuid":"x","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`))
	blocks := transcript.ExtractContentBlocks(e)
	fmt.Println(blocks.ToolUse[0].Name, string(blocks.ToolUse[0].Input))
	// Output: Bash {"command":"ls"}
}
