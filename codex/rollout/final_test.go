package rollout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

func TestFinalAssistantMessageFormatVariants(t *testing.T) {
	tests := []struct {
		file       string
		text       string
		sessionID  string
		cwd        string
		identified string
	}{
		{"response-item-final.jsonl", "first answer", "11111111-1111-1111-1111-111111111111", "/work/one", "response_item.message.role=assistant.phase=final_answer"},
		{"agent-message-final.jsonl", "delegate answer", "22222222-2222-2222-2222-222222222222", "/work/shared", "event_msg.agent_message.phase=final_answer"},
		{"agent-message-same-cwd.jsonl", "other delegate answer", "33333333-3333-3333-3333-333333333333", "/work/shared", "event_msg.agent_message.phase=implicit_final"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			path := filepath.Join("testdata", tt.file)
			got, ok, err := rollout.FinalAssistantMessage(path)
			if err != nil {
				t.Fatalf("FinalAssistantMessage() error = %v", err)
			}
			if !ok {
				t.Fatal("FinalAssistantMessage() ok = false, want true")
			}
			if got.Text != tt.text || got.Path != path || got.SessionID != tt.sessionID || got.Cwd != tt.cwd || got.Identification != tt.identified {
				t.Errorf("FinalAssistantMessage() = %+v", got)
			}
		})
	}
}

func TestFinalAssistantMessageNoAssistant(t *testing.T) {
	path := filepath.Join("testdata", "no-assistant.jsonl")
	got, ok, err := rollout.FinalAssistantMessage(path)
	if err != nil {
		t.Fatalf("FinalAssistantMessage() error = %v", err)
	}
	if ok {
		t.Fatalf("FinalAssistantMessage() = %+v, true; want false", got)
	}
	if got.Path != path || got.SessionID == "" || got.Cwd == "" {
		t.Errorf("provenance = %+v, want path, session ID, and cwd", got)
	}
}

func TestFinalAssistantMessageScanIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	header := `{"type":"session_meta","payload":{"id":"77777777-7777-7777-7777-777777777777","cwd":"/work/large"}}` + "\n"
	oldFinal := `{"type":"event_msg","payload":{"type":"agent_message","message":"outside bounded tail"}}` + "\n"
	padding := strings.Repeat(`{"type":"response_item","payload":{"type":"reasoning","padding":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}`+"\n", 3000)
	if err := os.WriteFile(path, []byte(header+oldFinal+padding), 0o600); err != nil {
		t.Fatal(err)
	}
	_, ok, err := rollout.FinalAssistantMessage(path)
	if err != nil {
		t.Fatalf("FinalAssistantMessage() error = %v", err)
	}
	if ok {
		t.Fatal("FinalAssistantMessage() found a message outside its bounded tail window")
	}
}

func TestSessionSnapshotEndings(t *testing.T) {
	tests := []struct {
		file      string
		status    rollout.Status
		hasFinal  bool
		finalText string
	}{
		{"response-item-final.jsonl", rollout.Done, true, "first answer"},
		{"interrupted-after-final.jsonl", rollout.Interrupted, true, "answer before interruption"},
		{"error-after-final.jsonl", rollout.Error, true, "answer before error"},
		{"no-assistant.jsonl", rollout.Interrupted, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := rollout.SessionSnapshot(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("SessionSnapshot() error = %v", err)
			}
			if got.Status != tt.status || got.HasFinal != tt.hasFinal || got.Final.Text != tt.finalText {
				t.Errorf("SessionSnapshot() = %+v", got)
			}
			if got.SessionID == "" || got.Cwd == "" || got.Final.Path == "" {
				t.Errorf("SessionSnapshot() provenance incomplete: %+v", got)
			}
		})
	}
}

func TestSameCwdRolloutsRemainDisambiguated(t *testing.T) {
	one, _, err := rollout.FinalAssistantMessage(filepath.Join("testdata", "agent-message-final.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	two, _, err := rollout.FinalAssistantMessage(filepath.Join("testdata", "agent-message-same-cwd.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if one.Cwd != two.Cwd || one.SessionID == two.SessionID || one.Path == two.Path {
		t.Fatalf("same-cwd provenance does not disambiguate rollouts: one=%+v two=%+v", one, two)
	}
}
