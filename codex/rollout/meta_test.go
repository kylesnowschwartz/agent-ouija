package rollout_test

import (
	"strings"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

func TestSessionMeta(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		ok     bool
		want   rollout.Entry
	}{
		{
			name: "header first line",
			stream: `{"timestamp":"2026-07-10T01:00:00Z","type":"session_meta","payload":{"cwd":"/work/proj","source":"cli"}}
{"timestamp":"2026-07-10T01:00:01Z","type":"turn_context","payload":{"cwd":"/work/proj"}}
`,
			ok:   true,
			want: rollout.Entry{Timestamp: "2026-07-10T01:00:00Z", Type: "session_meta", Payload: rollout.Payload{Cwd: "/work/proj", Source: rollout.Source{Kind: "cli", Raw: `"cli"`}}},
		},
		{
			name: "subagent object source",
			stream: `{"timestamp":"t","type":"session_meta","payload":{"cwd":"/work/sub","source":{"subagent":{"other":"guardian"}}}}
`,
			ok:   true,
			want: rollout.Entry{Timestamp: "t", Type: "session_meta", Payload: rollout.Payload{Cwd: "/work/sub", Source: rollout.Source{Raw: `{"subagent":{"other":"guardian"}}`}}},
		},
		{
			name:   "empty stream",
			stream: "",
			ok:     false,
		},
		{
			name: "first parseable entry is not session_meta",
			stream: `{"timestamp":"t","type":"turn_context","payload":{"cwd":"/work/proj"}}
{"timestamp":"t2","type":"session_meta","payload":{"cwd":"/late"}}
`,
			ok: false,
		},
		{
			name: "unparseable leading line skipped",
			stream: `{"type":
{"timestamp":"t","type":"session_meta","payload":{"cwd":"/work/proj","source":"cli"}}
`,
			ok:   true,
			want: rollout.Entry{Timestamp: "t", Type: "session_meta", Payload: rollout.Payload{Cwd: "/work/proj", Source: rollout.Source{Kind: "cli", Raw: `"cli"`}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := rollout.SessionMeta(strings.NewReader(tt.stream))
			if err != nil {
				t.Fatalf("SessionMeta() error = %v", err)
			}
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
