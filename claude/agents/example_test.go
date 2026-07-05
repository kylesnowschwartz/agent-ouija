package agents_test

import (
	"github.com/kylesnowschwartz/agent-ouija/claude/agents"
	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

// Full pipeline: discover both subagent kinds, then link them to the
// parent's Task tool calls.
func ExampleLinkSubagents() {
	sessionPath := "/path/to/session.jsonl"
	chunks, err := transcript.ReadSession(sessionPath)
	if err != nil {
		return
	}
	procs, _ := agents.DiscoverSubagents(sessionPath)
	teamProcs, _ := agents.DiscoverTeamSessions(sessionPath, chunks)
	all := append(procs, teamProcs...)
	colorMap := agents.LinkSubagents(all, chunks, sessionPath)
	_ = colorMap
}

// Sub-second tick consumers use the metadata-only scan instead — it never
// parses beyond each transcript's first line.
func ExampleScanSubagentMeta() {
	for _, m := range agents.ScanSubagentMeta("/path/to/session.jsonl") {
		_ = m.Description // sidecar metadata + file times; status policy is yours
	}
}
