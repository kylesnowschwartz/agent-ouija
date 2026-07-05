package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"model":"claude-opus-4-8","effortLevel":"high","outputStyle":"butterfield","env":{"SECRET":"x"}}`)

	got := Read(path)
	if got.Model != "claude-opus-4-8" || got.Effort != "high" || got.Style != "butterfield" {
		t.Errorf("Read = %+v, want model claude-opus-4-8, effort high, style butterfield", got)
	}
}

func TestRead_FailsOpen(t *testing.T) {
	if got := Read(filepath.Join(t.TempDir(), "missing.json")); got != (State{}) {
		t.Errorf("missing file: got %+v, want zero State", got)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{invalid`)
	if got := Read(path); got != (State{}) {
		t.Errorf("invalid JSON: got %+v, want zero State", got)
	}
}

func TestMcpServerNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"mcpServers":{"slack":{},"github":{}}}`)

	names := McpServerNames(path)
	slices.Sort(names)
	if len(names) != 2 || names[0] != "github" || names[1] != "slack" {
		t.Errorf("McpServerNames = %v, want [github slack]", names)
	}

	if n := McpServerNames(filepath.Join(t.TempDir(), "missing.json")); n != nil {
		t.Errorf("missing file: got %v, want nil", n)
	}
}

func TestNonEmptyHookCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"hooks":{"Stop":[{"hooks":[]}],"PreToolUse":[],"PostToolUse":null}}`)

	if got := NonEmptyHookCount(path); got != 1 {
		t.Errorf("NonEmptyHookCount = %d, want 1 (only Stop is non-empty)", got)
	}
}

// --- RegisterHooks (ported from tail-claude-hud setup_test.go) ---

var testHooks = []HookCommand{
	{Event: "PermissionRequest", Command: "tail-claude-hud", Args: []string{"hook", "permission-request"}},
	{Event: "PostToolUse", Command: "tail-claude-hud", Args: []string{"hook", "cleanup"}},
	{Event: "Stop", Command: "tail-claude-hud", Args: []string{"hook", "cleanup"}},
}

func readHooksMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	m, _ := s["hooks"].(map[string]any)
	return m
}

func TestRegisterHooks_FreshExecForm(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")

	added, err := RegisterHooks(path, testHooks)
	if err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}
	if len(added) != len(testHooks) {
		t.Errorf("added %d hooks, want %d", len(added), len(testHooks))
	}

	hooksMap := readHooksMap(t, path)
	for _, h := range testHooks {
		if !hasHookCommand(hooksMap, h) {
			t.Errorf("hook %s not registered", h.Event)
		}
	}

	// Verify the exec form was written (command + args), not a shell string.
	entries := hooksMap["PermissionRequest"].([]any)
	inner := entries[0].(map[string]any)["hooks"].([]any)
	hm := inner[0].(map[string]any)
	if hm["command"] != "tail-claude-hud" {
		t.Errorf("command = %v, want tail-claude-hud", hm["command"])
	}
	if !argsMatch(hm["args"], []string{"hook", "permission-request"}) {
		t.Errorf("args = %v, want [hook permission-request]", hm["args"])
	}
}

func TestRegisterHooks_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")

	if _, err := RegisterHooks(path, testHooks); err != nil {
		t.Fatalf("first RegisterHooks: %v", err)
	}
	added, err := RegisterHooks(path, testHooks)
	if err != nil {
		t.Fatalf("second RegisterHooks: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("second run added %v, want none", added)
	}
}

func TestRegisterHooks_DetectsLegacyShellForm(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	writeFile(t, path, `{
  "hooks": {
    "PermissionRequest": [
      {"hooks": [{"type": "command", "command": "tail-claude-hud hook permission-request"}]}
    ]
  }
}`)

	added, err := RegisterHooks(path, testHooks)
	if err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}
	if slices.Contains(added, "PermissionRequest") {
		t.Error("legacy PermissionRequest hook was duplicated")
	}
}

func TestRegisterHooks_PreservesUnrelatedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	writeFile(t, path, `{"model":"opus","env":{"KEY":"value"}}`)

	if _, err := RegisterHooks(path, testHooks); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "opus" {
		t.Errorf("model clobbered: %v", s["model"])
	}
	env, _ := s["env"].(map[string]any)
	if env["KEY"] != "value" {
		t.Errorf("env clobbered: %v", s["env"])
	}
	// No temp file left behind (atomic write).
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("temp file persists after RegisterHooks")
	}
}

func TestRegisterHooks_PreservesFileMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "x", Args: []string{"hook", "cleanup"}}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("settings.json mode = %o after rewrite, want 600 preserved (file holds secrets)", perm)
	}
}

func TestRegisterHooks_WritesThroughSymlink(t *testing.T) {
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "dotfiles")
	linkDir := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "settings.json")
	linkPath := filepath.Join(linkDir, "settings.json")
	if err := os.WriteFile(realPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	if _, err := RegisterHooks(linkPath, []HookCommand{{Event: "Stop", Command: "x", Args: []string{"hook", "cleanup"}}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	// The link must survive and the dotfiles target must hold the change.
	if info, err := os.Lstat(linkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("settings.json symlink was replaced by a regular file (dotfiles setups break)")
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"Stop"`) {
		t.Errorf("symlink target does not contain the registered hook: %s", data)
	}
}
