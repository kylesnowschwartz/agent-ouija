package rollout_test

import (
	"encoding/json"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

func TestParseEntry(t *testing.T) {
	tests := []struct {
		name string
		line string
		ok   bool
		want rollout.Entry
	}{
		{
			name: "turn_context",
			line: `{"timestamp":"2026-07-10T01:00:00Z","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:00Z", Type: "turn_context", Payload: rollout.Payload{Cwd: "/work/proj"}},
		},
		{
			name: "event_msg task_complete",
			line: `{"timestamp":"2026-07-10T01:00:01Z","type":"event_msg","payload":{"type":"task_complete"}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:01Z", Type: "event_msg", Payload: rollout.Payload{Type: "task_complete"}},
		},
		{
			name: "response_item assistant message",
			line: `{"timestamp":"2026-07-10T01:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary"}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:02Z", Type: "response_item", Payload: rollout.Payload{Type: "message", Role: "assistant", Phase: "commentary"}},
		},
		{
			name: "session_meta string source",
			line: `{"timestamp":"2026-07-10T01:00:00Z","type":"session_meta","payload":{"cwd":"/work/proj","source":"cli","cli_version":"0.144.1"}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:00Z", Type: "session_meta", Payload: rollout.Payload{Cwd: "/work/proj", Source: rollout.Source{Kind: "cli", Raw: `"cli"`}}},
		},
		{
			name: "session_meta object source",
			line: `{"timestamp":"2026-07-10T01:00:00Z","type":"session_meta","payload":{"cwd":"/work/proj","source":{"subagent":{"other":"guardian"}}}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:00Z", Type: "session_meta", Payload: rollout.Payload{Cwd: "/work/proj", Source: rollout.Source{Kind: "", Raw: `{"subagent":{"other":"guardian"}}`}}},
		},
		{
			name: "malformed JSON",
			line: `{"type":`,
			ok:   false,
		},
		{
			name: "empty line",
			line: "",
			ok:   false,
		},
		{
			name: "unknown fields tolerated",
			line: `{"timestamp":"x","type":"turn_context","payload":{"cwd":"/p","model":"gpt-x","extra":{"nested":true}}}`,
			ok:   true,
			want: rollout.Entry{Timestamp: "x", Type: "turn_context", Payload: rollout.Payload{Cwd: "/p"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := rollout.ParseEntry([]byte(tt.line))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSourceUnmarshalJSONObjectClearsKind(t *testing.T) {
	source := rollout.Source{Kind: "cli"}
	if err := json.Unmarshal([]byte(`{"subagent":{"other":"guardian"}}`), &source); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if source.Kind != "" {
		t.Errorf("Kind = %q, want empty", source.Kind)
	}
}
