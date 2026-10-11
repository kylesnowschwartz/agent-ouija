package settings

import (
	"bytes"
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

// hookGroup and hookEntry are the shape RegisterHooks writes for a new
// registration. Field order is the order Claude Code itself writes.
type hookGroup struct {
	Hooks []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Async   bool     `json:"async,omitempty"`
}

// RegisterHooks patches the settings.json at path to register the given
// hook commands, creating the file if needed. Returns the events that were
// newly registered; (nil, nil) when everything was already present, in
// which case the file is not written.
//
// Registration is idempotent against BOTH serialization forms: an existing
// entry matches if its command equals the legacy shell string
// ("cmd arg1 arg2") or if it is exec-form with equal command and args.
//
// The rewrite keeps the file's key order and every value it does not
// change byte for byte, then re-indents with two spaces and no HTML
// escaping. A new event goes at the end of "hooks"; a new group for an
// existing event goes at the end of that event's array. The write is
// atomic (temp file + rename).
func RegisterHooks(path string, cmds []HookCommand) ([]string, error) {
	settings, err := readSettings(path)
	if err != nil {
		return nil, err
	}

	hooks, err := hooksObject(settings)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var added []string
	for _, h := range cmds {
		if hasHookCommand(hooks, h) {
			continue
		}
		if err := appendHook(hooks, h); err != nil {
			return nil, err
		}
		added = append(added, h.Event)
	}

	if len(added) == 0 {
		return nil, nil
	}

	encodedHooks, err := hooks.encode()
	if err != nil {
		return nil, fmt.Errorf("encode hooks: %w", err)
	}
	settings.set("hooks", encodedHooks)

	if err := writeSettings(path, settings); err != nil {
		return nil, err
	}
	return added, nil
}

// readSettings reads and parses a settings.json. Returns an empty object if
// the file doesn't exist; returns an error for other read/parse failures,
// including a file whose top level is not a JSON object.
func readSettings(path string) (*jsonObject, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &jsonObject{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	settings := &jsonObject{}
	if err := json.Unmarshal(data, settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

// hooksObject returns the decoded "hooks" object from settings. An absent
// "hooks" key, or one holding something other than an object, yields an
// empty object that replaces it when written back.
func hooksObject(settings *jsonObject) (*jsonObject, error) {
	hooks := &jsonObject{}
	raw, ok := settings.get("hooks")
	if !ok || jsonKind(raw) != '{' {
		return hooks, nil
	}
	if err := json.Unmarshal(raw, hooks); err != nil {
		return nil, fmt.Errorf("hooks: %w", err)
	}
	return hooks, nil
}

// hasHookCommand checks whether the hooks object already contains a matching
// hook for the given command. The Claude Code hooks schema is:
//
//	"EventName": [{"hooks": [{"type": "command", "command": "...", "args": [...]}]}]
//
// An event value that is not an array, and entries of any other shape,
// never match.
func hasHookCommand(hooks *jsonObject, h HookCommand) bool {
	raw, ok := hooks.get(h.Event)
	if !ok || jsonKind(raw) != '[' {
		return false
	}
	var entries []any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return false // unreachable: raw is a valid JSON array
	}
	legacy := h.legacyString()
	for _, entry := range entries {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := em["hooks"].([]any)
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

// appendHook adds a new exec-form hook group for the given command at the
// end of its event's array. Existing groups keep their bytes; an event
// value that is not an array is replaced by a one-group array.
func appendHook(hooks *jsonObject, h HookCommand) error {
	group, err := marshalUnescaped(hookGroup{Hooks: []hookEntry{{
		Type:    "command",
		Command: h.Command,
		Args:    h.Args,
		Async:   h.Async,
	}}})
	if err != nil {
		return fmt.Errorf("encode %s hook: %w", h.Event, err)
	}

	var groups []json.RawMessage
	if raw, ok := hooks.get(h.Event); ok && jsonKind(raw) == '[' {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return fmt.Errorf("hooks.%s: %w", h.Event, err)
		}
	}
	groups = append(groups, group)

	encoded, err := marshalUnescaped(groups)
	if err != nil {
		return fmt.Errorf("encode %s hooks: %w", h.Event, err)
	}
	hooks.set(h.Event, encoded)
	return nil
}

// writeSettings writes settings back to disk atomically (temp file +
// rename), indented with two spaces, with a trailing newline and without
// HTML escaping. settings.json holds secrets and is read concurrently by
// Claude Code — a partial write must never be observable.
//
// The rename targets the symlink-resolved path (a dotfiles-managed
// settings.json must keep pointing at its repo copy, and the repo copy is
// what must change), and the replacement file keeps the existing file's
// permissions (a hardened 0600 must not silently widen to 0644).
func writeSettings(path string, settings *jsonObject) error {
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

	compact, err := settings.encode()
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return fmt.Errorf("indent settings: %w", err)
	}
	buf.WriteByte('\n')

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), mode); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup; the rename failure is the error that matters
		return fmt.Errorf("rename %s: %w", target, err)
	}
	return nil
}
