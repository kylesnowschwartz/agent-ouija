package session

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// ReadToolCalls returns the tool calls in the assistant messages written
// from offset on, in file order, plus the offset to pass on the next
// call. Each call carries its assistant entry's timestamp.
//
// Partial-last-line rule: an unterminated last line is always deferred —
// it is neither returned nor counted in next, even when it parses — so
// the next call reads it once its newline lands. A call is reported at
// most one poll late and never twice. A resident poller (such as a
// sidebar server ticking each second) passes next back unchanged.
//
// Cost: proportional to the bytes written since offset.
func ReadToolCalls(path string, offset int64) (calls []ToolCall, next int64, err error) {
	next, err = readFrom(path, offset, func(line []byte) {
		if !bytes.Contains(line, []byte(`"toolCall"`)) {
			return
		}
		e, ok := ParseEntry(line)
		if !ok || e.Type != "message" || e.Message.Role != "assistant" {
			return
		}
		ts, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
		for _, c := range toolCalls(e.Message.Content) {
			c.Time = ts
			calls = append(calls, c)
		}
	})
	return calls, next, err
}

// Summary holds a session's display fields.
type Summary struct {
	// Name is the name of the last session_info entry; "" when there is
	// none or the last one carries no name.
	Name string
	// FirstPrompt is the first non-blank line of the first user
	// message's text, trimmed; "" until a user message with text exists.
	FirstPrompt string
}

// ReadSummary folds the lines written from offset on into prev and
// returns the result plus the offset to pass on the next call. Start
// with offset 0 and a zero Summary. The session name can change at any
// point in the file, so a poller keeps the returned Summary and offset
// and passes them back; the partial-last-line rule of ReadToolCalls
// applies.
//
// Cost: proportional to the bytes written since offset. The first call
// reads the whole file, including the system message near the top
// (often tens of KiB); later calls read only appended bytes.
func ReadSummary(path string, offset int64, prev Summary) (Summary, int64, error) {
	s := prev
	next, err := readFrom(path, offset, func(line []byte) {
		info := bytes.Contains(line, []byte(`"session_info"`))
		user := s.FirstPrompt == "" && bytes.Contains(line, []byte(`"user"`))
		if !info && !user {
			return
		}
		e, ok := ParseEntry(line)
		if !ok {
			return
		}
		switch {
		case e.Type == "session_info":
			s.Name = e.Name
		case user && e.Type == "message" && e.Message.Role == "user":
			s.FirstPrompt = firstLine(e.Message.Content)
		}
	})
	return s, next, err
}

// readFrom calls fn for each terminated line from offset on and returns
// the offset after the last terminated line.
func readFrom(path string, offset int64, fn func(line []byte)) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	lr := jsonl.NewReader(f)
	for {
		line, ok := lr.Next()
		if !ok || !lr.LastLineTerminated() {
			break
		}
		fn([]byte(line))
	}
	return offset + lr.TerminatedBytesRead(), lr.Err()
}

// firstLine returns the first non-blank line of a user message's text
// content (a JSON string or an array of blocks), trimmed.
func firstLine(content Raw) string {
	var texts []string
	var s string
	if json.Unmarshal([]byte(content.Raw), &s) == nil {
		texts = []string{s}
	} else {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content.Raw), &blocks) != nil {
			return ""
		}
		for _, b := range blocks {
			if b.Type == "text" {
				texts = append(texts, b.Text)
			}
		}
	}
	for _, t := range texts {
		for _, l := range strings.Split(t, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				return l
			}
		}
	}
	return ""
}
