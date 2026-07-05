package agents

import "encoding/json"

// TeamSpecFromInput exposes teamSpecFromInput for external tests.
func TeamSpecFromInput(input json.RawMessage) (teamName, agentName string, ok bool) {
	return teamSpecFromInput(input)
}
