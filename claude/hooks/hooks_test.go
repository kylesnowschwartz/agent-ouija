package hooks

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	doc := `{"hook_event_name":"PermissionRequest","session_id":"s1","cwd":"/proj","tool_name":"Bash","tool_input":{"command":"ls"},"future_field":42}`
	p, err := Decode(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if p.HookEventName != "PermissionRequest" || p.SessionID != "s1" || p.CWD != "/proj" || p.ToolName != "Bash" {
		t.Errorf("Decode = %+v", p)
	}
	if string(p.ToolInput) != `{"command":"ls"}` {
		t.Errorf("ToolInput not preserved: %s", p.ToolInput)
	}
	// Raw must carry unmodeled fields.
	if !strings.Contains(string(p.Raw), "future_field") {
		t.Error("Raw does not preserve unmodeled fields")
	}
}

func TestDecode_Invalid(t *testing.T) {
	if _, err := Decode(strings.NewReader(`{`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestEffectiveSessionID(t *testing.T) {
	if got := (Payload{SessionID: "explicit"}).EffectiveSessionID(); got != "explicit" {
		t.Errorf("got %q, want explicit", got)
	}

	t.Setenv("CLAUDE_CODE_SESSION_ID", "from-env")
	if got := (Payload{}).EffectiveSessionID(); got != "from-env" {
		t.Errorf("got %q, want from-env (env fallback)", got)
	}
}

func TestSessionStartOutputShape(t *testing.T) {
	out := NewSessionStartOutput("proj · main")
	if out.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Error("event name not pre-filled")
	}
	if out.HookSpecificOutput.SessionTitle != "proj · main" {
		t.Error("title not set")
	}
}
