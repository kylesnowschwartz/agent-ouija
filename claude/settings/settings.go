// Package settings reads and patches Claude Code settings.json files.
//
// settings.json holds sensitive values (env vars, tokens). Read decodes
// ONLY the fields it returns and nothing in this package ever logs or
// returns the raw content. Keep it that way.
package settings

import (
	"encoding/json"
	"os"
)

// State is the global gear state persisted by /model and /effort.
type State struct {
	Model  string // "model" key
	Effort string // "effortLevel" key
}

// Read returns the model and effort level from a settings.json file.
// /model and /effort persist to these keys the moment they change, so this
// is Claude Code's own truth rather than a sniffed approximation. Any
// failure (missing file, invalid JSON) degrades to the zero State — callers
// render stateless, never error.
func Read(path string) State {
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var s struct {
		Model  string `json:"model"`
		Effort string `json:"effortLevel"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}
	}
	return State{Model: s.Model, Effort: s.Effort}
}

// settingsFile is a minimal struct for introspection reads. The Hooks field
// is only present in settings.json; json.Unmarshal silently ignores it when
// decoding .mcp.json files, so both file kinds decode through this type.
type settingsFile struct {
	McpServers map[string]json.RawMessage `json:"mcpServers"`
	Hooks      map[string]json.RawMessage `json:"hooks"`
}

// McpServerNames returns the mcpServers key names from a settings.json or
// .mcp.json file. Missing or invalid files yield nil. Callers aggregating
// across multiple files (global + project + local) dedupe themselves.
func McpServerNames(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil
	}
	names := make([]string, 0, len(sf.McpServers))
	for name := range sf.McpServers {
		names = append(names, name)
	}
	return names
}

// NonEmptyHookCount returns how many hook event keys in a settings.json
// have a non-empty array value. Missing or invalid files return 0.
func NonEmptyHookCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return 0
	}
	count := 0
	for _, v := range sf.Hooks {
		var arr []json.RawMessage
		if err := json.Unmarshal(v, &arr); err == nil && len(arr) > 0 {
			count++
		}
	}
	return count
}
