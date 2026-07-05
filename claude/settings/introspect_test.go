package settings

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Introspect must agree with the three single-purpose readers it combines —
// same file, one read, identical answers.
func TestIntrospect_MatchesSinglePurposeReaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	content := `{
		"model": "opus",
		"effortLevel": "high",
		"outputStyle": "concise",
		"mcpServers": {"slack": {"command": "x"}, "github": {"command": "y"}},
		"hooks": {
			"PreToolUse": [{"matcher": "*"}],
			"Stop": [],
			"SessionStart": [{"matcher": "*"}]
		},
		"env": {"SHOULD_NEVER_BE_DECODED": "sentinel"}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Introspect(path)

	if want := Read(path); got.State != want {
		t.Errorf("State: got %+v, want %+v", got.State, want)
	}
	wantNames := McpServerNames(path)
	sort.Strings(wantNames)
	gotNames := append([]string(nil), got.McpServers...)
	sort.Strings(gotNames)
	if len(gotNames) != 2 || gotNames[0] != wantNames[0] || gotNames[1] != wantNames[1] {
		t.Errorf("McpServers: got %v, want %v", got.McpServers, wantNames)
	}
	if want := NonEmptyHookCount(path); got.HookCount != want || want != 2 {
		t.Errorf("HookCount: got %d, want %d (empty Stop array must not count)", got.HookCount, want)
	}
}

func TestIntrospect_DegradesToZero(t *testing.T) {
	if got := Introspect(filepath.Join(t.TempDir(), "missing.json")); got.State != (State{}) || got.McpServers != nil || got.HookCount != 0 {
		t.Errorf("missing file: got %+v, want zero Introspection", got)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Introspect(bad); got.State != (State{}) || got.McpServers != nil || got.HookCount != 0 {
		t.Errorf("invalid JSON: got %+v, want zero Introspection", got)
	}
}
