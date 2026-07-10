package rollout_test

import (
	"strings"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

func TestTrailingState(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		want   rollout.State
	}{
		{
			name:   "empty stream",
			stream: "",
			want:   rollout.State{Status: rollout.Idle, Cwd: ""},
		},
		{
			name:   "turn_context only, no lifecycle signal",
			stream: `{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}` + "\n",
			want:   rollout.State{Status: rollout.Idle, Cwd: "/work/proj"},
		},
		{
			name: "user message then task_complete",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"user_message"}}`,
				`{"timestamp":"t3","type":"event_msg","payload":{"type":"task_complete"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Done, Cwd: "/work/proj"},
		},
		{
			name: "turn_aborted",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"user_message"}}`,
				`{"timestamp":"t3","type":"event_msg","payload":{"type":"turn_aborted"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Interrupted, Cwd: "/work/proj"},
		},
		{
			name: "error event",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"error"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Error, Cwd: "/work/proj"},
		},
		{
			name: "agent_message commentary phase still running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"agent_message","phase":"commentary"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "agent_message final phase is done",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"agent_message"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Done, Cwd: "/work/proj"},
		},
		{
			name: "response_item user message running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"response_item","payload":{"type":"message","role":"user"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "response_item assistant commentary running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "response_item function_call is running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"response_item","payload":{"type":"function_call"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "response_item function_call_output is running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"response_item","payload":{"type":"function_call_output"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "response_item reasoning is running",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"response_item","payload":{"type":"reasoning"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "later entries override earlier status",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"task_complete"}}`,
				`{"timestamp":"t3","type":"event_msg","payload":{"type":"user_message"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Running, Cwd: "/work/proj"},
		},
		{
			name: "first cwd wins across multiple turn_context entries",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"turn_context","payload":{"cwd":"/should/not/win"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Idle, Cwd: "/work/proj"},
		},
		{
			name: "malformed line skipped, surrounding lines still parsed",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{not valid json`,
				`{"timestamp":"t3","type":"event_msg","payload":{"type":"task_complete"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Done, Cwd: "/work/proj"},
		},
		{
			name: "unrecognized entry type leaves status unchanged",
			stream: strings.Join([]string{
				`{"timestamp":"t1","type":"turn_context","payload":{"cwd":"/work/proj"}}`,
				`{"timestamp":"t2","type":"event_msg","payload":{"type":"task_complete"}}`,
				`{"timestamp":"t3","type":"session_meta","payload":{"type":"whatever"}}`,
			}, "\n") + "\n",
			want: rollout.State{Status: rollout.Done, Cwd: "/work/proj"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rollout.TrailingState(strings.NewReader(tt.stream))
			if err != nil {
				t.Fatalf("TrailingState error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    rollout.Status
		want string
	}{
		{rollout.Idle, "idle"},
		{rollout.Running, "running"},
		{rollout.Done, "done"},
		{rollout.Interrupted, "interrupted"},
		{rollout.Error, "error"},
		{rollout.Status(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}
