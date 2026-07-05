package tools

import (
	"encoding/json"
)

// ParseInputFields unmarshals a JSON tool input into a field map.
// Returns nil on error or empty input.
func ParseInputFields(input json.RawMessage) map[string]json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil
	}
	return fields
}

// SubagentInfo holds metadata extracted from a Task tool_use input.
type SubagentInfo struct {
	Type        string // "Explore", "Plan", "general-purpose", etc.
	Description string // Task description or truncated prompt
	MemberName  string // team member name (only for team Task calls)
}

// ExtractSubagentInfo extracts metadata from Task/Agent/Skill tool input
// fields. Single decoder for the type and description fallback chains —
// summaryTask builds its display string from this result, so the summary
// prefix and DisplayItem.SubagentType can never disagree.
func ExtractSubagentInfo(fields map[string]json.RawMessage) SubagentInfo {
	var info SubagentInfo

	info.Type = GetString(fields, "subagent_type")
	if info.Type == "" {
		// Some sessions carry the camelCase variant.
		info.Type = GetString(fields, "subagentType")
	}
	if info.Type == "" {
		// Skill tool uses "skill" for type and "args" for description.
		info.Type = GetString(fields, "skill")
	}

	// Try "description" first, then "prompt", then "args" (Skill tool).
	info.Description = GetString(fields, "description")
	if info.Description == "" {
		info.Description = Truncate(GetString(fields, "prompt"), 80)
	}
	if info.Description == "" {
		info.Description = Truncate(GetString(fields, "args"), 80)
	}

	// Team member name (present when team_name + name are both set).
	info.MemberName = GetString(fields, "name")
	return info
}
