package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HookCommand describes one hook registration: which event fires it and
// the command to run. Exec form (Command + Args) avoids shell interpolation
// of arguments; a HookCommand with no Args registers a bare command.
type HookCommand struct {
	Event   string   // e.g. "PermissionRequest", "PostToolUse", "Stop"
	Command string   // binary name or path
	Args    []string // exec-form arguments, e.g. ["hook", "cleanup"]
	Async   bool     // fire-and-forget: Claude Code does not wait for the hook
}

// legacyString is the shell-string form of the same command, matched for
// idempotency so re-registering after an upgrade from string-form hooks
// does not duplicate entries.
func (h HookCommand) legacyString() string {
	if len(h.Args) == 0 {
		return h.Command
	}
	return h.Command + " " + strings.Join(h.Args, " ")
}

// RegisterHooks patches the settings.json at path to register the given
// hook commands, creating the file if needed. Returns the events that were
// newly registered; (nil, nil) when everything was already present.
//
// Registration is idempotent against BOTH serialization forms: an existing
// entry matches if its command equals the legacy shell string
// ("cmd arg1 arg2") or if it is exec-form with equal command and args.
// The write is atomic (temp file + rename).
func RegisterHooks(path string, cmds []HookCommand) ([]string, error) {
	settings, err := readSettingsMap(path)
	if err != nil {
		return nil, err
	}

	hooksMap := ensureHooksMap(settings)

	var added []string
	for _, h := range cmds {
		if hasHookCommand(hooksMap, h) {
			continue
		}
		appendHook(hooksMap, h)
		added = append(added, h.Event)
	}

	if len(added) == 0 {
		return nil, nil
	}

	if err := writeSettingsMap(path, settings); err != nil {
		return nil, err
	}
	return added, nil
}

// readSettingsMap reads and parses a settings.json. Returns an empty map if
// the file doesn't exist; returns an error for other read/parse failures.
func readSettingsMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

// ensureHooksMap returns the "hooks" sub-map from settings, creating it if absent.
func ensureHooksMap(settings map[string]any) map[string]any {
	v, ok := settings["hooks"]
	if !ok {
		m := make(map[string]any)
		settings["hooks"] = m
		return m
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	// Unexpected type — overwrite with an empty map.
	m := make(map[string]any)
	settings["hooks"] = m
	return m
}

// hasHookCommand checks whether the hooks map already contains a matching
// hook for the given command. The Claude Code hooks schema is:
//
//	"EventName": [{"hooks": [{"type": "command", "command": "...", "args": [...]}]}]
func hasHookCommand(hooksMap map[string]any, h HookCommand) bool {
	arr, ok := hooksMap[h.Event]
	if !ok {
		return false
	}
	entries, ok := arr.([]any)
	if !ok {
		return false
	}
	legacy := h.legacyString()
	for _, entry := range entries {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		innerArr, ok := em["hooks"]
		if !ok {
			continue
		}
		innerHooks, ok := innerArr.([]any)
		if !ok {
			continue
		}
		for _, ih := range innerHooks {
			hm, ok := ih.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if cmd == legacy {
				return true // legacy shell-string form
			}
			if cmd == h.Command && argsMatch(hm["args"], h.Args) {
				return true // exec form
			}
		}
	}
	return false
}

// argsMatch reports whether a decoded JSON args value equals want.
func argsMatch(v any, want []string) bool {
	arr, ok := v.([]any)
	if !ok {
		return len(want) == 0 && v == nil
	}
	if len(arr) != len(want) {
		return false
	}
	for i, item := range arr {
		s, _ := item.(string)
		if s != want[i] {
			return false
		}
	}
	return true
}

// appendHook adds a new exec-form hook entry for the given command.
func appendHook(hooksMap map[string]any, h HookCommand) {
	args := make([]any, len(h.Args))
	for i, a := range h.Args {
		args[i] = a
	}
	inner := map[string]any{
		"type":    "command",
		"command": h.Command,
	}
	if len(args) > 0 {
		inner["args"] = args
	}
	if h.Async {
		inner["async"] = true
	}
	entry := map[string]any{
		"hooks": []any{inner},
	}

	existing, ok := hooksMap[h.Event]
	if !ok {
		hooksMap[h.Event] = []any{entry}
		return
	}
	if arr, ok := existing.([]any); ok {
		hooksMap[h.Event] = append(arr, entry)
	} else {
		hooksMap[h.Event] = []any{entry}
	}
}

// writeSettingsMap writes the settings map back to disk atomically (temp
// file + rename) with readable formatting. settings.json holds secrets and
// is read concurrently by Claude Code — a partial write must never be
// observable.
//
// The rename targets the symlink-resolved path (a dotfiles-managed
// settings.json must keep pointing at its repo copy, and the repo copy is
// what must change), and the replacement file keeps the existing file's
// permissions (a hardened 0600 must not silently widen to 0644).
func writeSettingsMap(path string, settings map[string]any) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup; the rename failure is the error that matters
		return fmt.Errorf("rename %s: %w", target, err)
	}
	return nil
}
