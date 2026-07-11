package rollout

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

const finalAssistantScanBytes = 256 * 1024

// FinalMessage is the final assistant-authored answer and its provenance.
type FinalMessage struct {
	Text           string
	Path           string
	SessionID      string
	Cwd            string
	Identification string
}

// FinalAssistantMessage returns the newest final-phase assistant message in
// path. It reads only the session_meta header and the last 256 KiB of the
// rollout; it never scans a large rollout forward. ok is false when the
// bounded window contains no final assistant message.
func FinalAssistantMessage(path string) (message FinalMessage, ok bool, err error) {
	message.Path = path
	if err := addSessionProvenance(path, &message); err != nil {
		return message, false, err
	}

	err = jsonl.ReverseScan(path, finalAssistantScanBytes, func(line []byte) bool {
		entry, parsed := ParseEntry(line)
		if !parsed || entry.Payload.Phase == "commentary" {
			return true
		}

		switch {
		case entry.Type == "response_item" && entry.Payload.Type == "message" && entry.Payload.Role == "assistant":
			message.Text = contentText(entry.Payload.Content)
			if message.Text == "" {
				return true
			}
			message.Identification = "response_item.message.role=assistant.phase=" + phaseName(entry.Payload.Phase)
			ok = true
			return false
		case entry.Type == "event_msg" && entry.Payload.Type == "agent_message":
			message.Text = entry.Payload.Message
			if message.Text == "" {
				return true
			}
			message.Identification = "event_msg.agent_message.phase=" + phaseName(entry.Payload.Phase)
			ok = true
			return false
		default:
			return true
		}
	})
	return message, ok, err
}

func addSessionProvenance(path string, message *FinalMessage) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	meta, ok, err := SessionMeta(f)
	if err != nil {
		return err
	}
	if ok {
		message.SessionID = meta.Payload.ID
		message.Cwd = meta.Payload.Cwd
	}
	return nil
}

func contentText(content Content) string {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(content.Raw), &blocks) != nil {
		return ""
	}
	var text []string
	for _, block := range blocks {
		if (block.Type == "output_text" || block.Type == "text") && block.Text != "" {
			text = append(text, block.Text)
		}
	}
	return strings.Join(text, "")
}

func phaseName(phase string) string {
	if phase == "" {
		return "implicit_final"
	}
	return phase
}

// Snapshot is the one-call view of what a Codex session concluded.
type Snapshot struct {
	SessionID string
	Cwd       string
	Status    Status
	Final     FinalMessage
	HasFinal  bool
}

// SessionSnapshot composes SessionMeta, TrailingState, and
// FinalAssistantMessage for a rollout path.
func SessionSnapshot(path string) (Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	state, stateErr := TrailingState(f)
	closeErr := f.Close()
	if stateErr != nil {
		return Snapshot{}, stateErr
	}
	if closeErr != nil {
		return Snapshot{}, closeErr
	}

	final, ok, err := FinalAssistantMessage(path)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		SessionID: final.SessionID,
		Cwd:       final.Cwd,
		Status:    state.Status,
		Final:     final,
		HasFinal:  ok,
	}
	if snapshot.Cwd == "" {
		snapshot.Cwd = state.Cwd
	}
	if snapshot.Final.Cwd == "" {
		snapshot.Final.Cwd = snapshot.Cwd
	}
	return snapshot, nil
}
