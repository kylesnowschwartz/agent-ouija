package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/pi/session"
)

const header = `{"type":"session","version":3,"id":"s1","timestamp":"2026-11-13T10:00:00.000Z","cwd":"/work/proj"}`

func user(id, ts, text string) string {
	return `{"type":"message","id":"` + id + `","parentId":null,"timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"` + text + `"}]}}`
}

func assistant(id, ts, stop, calls string) string {
	content := `[{"type":"text","text":"ok"}` + calls + `]`
	return `{"type":"message","id":"` + id + `","parentId":"x","timestamp":"` + ts + `","message":{"role":"assistant","content":` + content + `,"stopReason":"` + stop + `"}}`
}

func call(id, name string) string {
	return `,{"type":"toolCall","id":"` + id + `","name":"` + name + `","arguments":{"command":"ls"}}`
}

func result(id, ts, callID string) string {
	return `{"type":"message","id":"` + id + `","parentId":"x","timestamp":"` + ts + `","message":{"role":"toolResult","toolCallId":"` + callID + `","toolName":"bash","content":[{"type":"text","text":"ok"}],"isError":false}}`
}

func writeLines(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "2026-11-13T10-00-00-000Z_s1.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return ts
}

func TestParseEntry(t *testing.T) {
	e, ok := session.ParseEntry([]byte(header))
	if !ok || e.Type != "session" || e.Version != 3 || e.Cwd != "/work/proj" || e.ID != "s1" {
		t.Fatalf("header = %+v, %v", e, ok)
	}
	e, ok = session.ParseEntry([]byte(`{"type":"session_info","id":"e1","parentId":"e0","timestamp":"t","name":"refactor"}`))
	if !ok || e.Name != "refactor" || e.ParentID != "e0" {
		t.Fatalf("session_info = %+v", e)
	}
	e2, _ := session.ParseEntry([]byte(result("r", "t", "c1")))
	e3, _ := session.ParseEntry([]byte(result("r", "t", "c1")))
	if e2 != e3 || e2.Message.ToolCallID != "c1" || e2.Message.ToolName != "bash" {
		t.Fatalf("toolResult = %+v", e2)
	}
	if _, ok := session.ParseEntry([]byte(`{"type":`)); ok {
		t.Error("invalid JSON must be rejected")
	}
}

func TestTrailingState(t *testing.T) {
	const (
		t1 = "2026-11-13T10:00:01.000Z"
		t2 = "2026-11-13T10:00:02.000Z"
		t3 = "2026-11-13T10:00:03.000Z"
	)
	custom := `{"type":"custom","customType":"ext.a","data":{},"id":"c","parentId":"x","timestamp":"2026-11-13T10:00:09.000Z"}`
	bash := `{"type":"message","id":"b","parentId":"x","timestamp":"2026-11-13T10:00:09.000Z","message":{"role":"bashExecution","command":"ls","output":"","exitCode":0,"cancelled":false,"truncated":false}}`
	tests := []struct {
		name  string
		lines []string
		want  session.State
	}{
		{"header only", []string{header}, session.State{Status: session.None}},
		{"user last", []string{header, user("u", t1, "hi")},
			session.State{Status: session.Working, LastMessageAt: mustTime(t1)}},
		{"assistant toolUse, tool running", []string{header, user("u", t1, "hi"), assistant("a", t2, "toolUse", call("c1", "bash"))},
			session.State{Status: session.Working, StopReason: "toolUse", LastMessageAt: mustTime(t2),
				PendingToolCalls: []session.ToolCall{{ID: "c1", Name: "bash", Arguments: `{"command":"ls"}`, Time: mustTime(t2)}}}},
		{"one of two parallel calls answered", []string{header, user("u", t1, "hi"), assistant("a", t2, "toolUse", call("c1", "bash")+call("c2", "read")), result("r", t3, "c1")},
			session.State{Status: session.Working, LastMessageAt: mustTime(t3),
				PendingToolCalls: []session.ToolCall{{ID: "c2", Name: "read", Arguments: `{"command":"ls"}`, Time: mustTime(t2)}}}},
		{"aborted", []string{header, user("u", t1, "hi"), assistant("a", t2, "aborted", "")},
			session.State{Status: session.Interrupted, StopReason: "aborted", LastMessageAt: mustTime(t2)}},
		{"error", []string{header, user("u", t1, "hi"), assistant("a", t2, "error", "")},
			session.State{Status: session.Error, StopReason: "error", LastMessageAt: mustTime(t2)}},
		{"custom entry after stop", []string{header, user("u", t1, "hi"), assistant("a", t2, "stop", ""), custom},
			session.State{Status: session.Done, StopReason: "stop", LastMessageAt: mustTime(t2)}},
		{"bashExecution after stop", []string{header, user("u", t1, "hi"), assistant("a", t2, "length", ""), bash},
			session.State{Status: session.Done, StopReason: "length", LastMessageAt: mustTime(t2)}},
		{"deferred", []string{header, user("u", t1, "hi"), assistant("a", t2, "deferred", "")},
			session.State{Status: session.Done, StopReason: "deferred", LastMessageAt: mustTime(t2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := session.TrailingState(writeLines(t, tt.lines...))
			if err != nil {
				t.Fatal(err)
			}
			assertState(t, got, tt.want)
		})
	}
}

func assertState(t *testing.T, got, want session.State) {
	t.Helper()
	if got.Status != want.Status || got.StopReason != want.StopReason || !got.LastMessageAt.Equal(want.LastMessageAt) ||
		len(got.PendingToolCalls) != len(want.PendingToolCalls) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want.PendingToolCalls {
		g, w := got.PendingToolCalls[i], want.PendingToolCalls[i]
		if g.ID != w.ID || g.Name != w.Name || g.Arguments != w.Arguments || !g.Time.Equal(w.Time) {
			t.Errorf("pending[%d] = %+v, want %+v", i, g, w)
		}
	}
}

// A toolResult larger than the first tail window forces the window to
// grow until it reaches the assistant message holding the calls.
func TestTrailingState_GrowsWindow(t *testing.T) {
	big := `{"type":"message","id":"r","parentId":"x","timestamp":"2026-11-13T10:00:03.000Z","message":{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"` +
		strings.Repeat("x", 200<<10) + `"}],"isError":false}}`
	p := writeLines(t, header, user("u", "2026-11-13T10:00:01.000Z", "hi"),
		assistant("a", "2026-11-13T10:00:02.000Z", "toolUse", call("c1", "bash")+call("c2", "bash")), big)
	got, err := session.TrailingState(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != session.Working || len(got.PendingToolCalls) != 1 || got.PendingToolCalls[0].ID != "c2" {
		t.Fatalf("got %+v", got)
	}
}

func TestTrailingState_Fixture(t *testing.T) {
	p := filepath.Join("testdata", "two-turns.jsonl")
	got, err := session.TrailingState(p)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, got, session.State{Status: session.Done, StopReason: "stop", LastMessageAt: mustTime("2026-11-13T10:04:54.210Z")})

	calls, next, err := session.ReadToolCalls(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	if len(calls) != 2 || calls[0].ID != "call-1" || calls[1].ID != "call-2" || next != info.Size() {
		t.Fatalf("calls = %+v, next = %d (size %d)", calls, next, info.Size())
	}
	if calls[1].Arguments != `{"command":"sleep 60","timeout":90}` || !calls[1].Time.Equal(mustTime("2026-11-13T10:03:51.034Z")) {
		t.Errorf("call-2 = %+v", calls[1])
	}

	sum, _, err := session.ReadSummary(p, 0, session.Summary{})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Name != "" || sum.FirstPrompt != "Run the shell command: sleep 8 && echo hello-from-pi. Then tell me its output in one short sentence." {
		t.Errorf("summary = %+v", sum)
	}
}

func TestReadToolCalls_PartialLineAndResume(t *testing.T) {
	p := writeLines(t, header, user("u", "2026-11-13T10:00:01.000Z", "hi"))
	partial := assistant("a", "2026-11-13T10:00:02.000Z", "toolUse", call("c1", "bash"))
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(partial); err != nil { // parseable, no newline
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	terminated := info.Size() - int64(len(partial))

	calls, next, err := session.ReadToolCalls(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 || next != terminated {
		t.Fatalf("partial: calls = %+v, next = %d, want none, %d", calls, next, terminated)
	}

	if _, err := f.WriteString("\n" + assistant("b", "2026-11-13T10:00:03.000Z", "toolUse", call("c2", "read")) + "\n"); err != nil {
		t.Fatal(err)
	}
	calls, next2, err := session.ReadToolCalls(p, next)
	if err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(p)
	if len(calls) != 2 || calls[0].ID != "c1" || calls[1].ID != "c2" || next2 != info.Size() {
		t.Fatalf("resume: calls = %+v, next = %d, size %d", calls, next2, info.Size())
	}

	calls, next3, err := session.ReadToolCalls(p, next2)
	if err != nil || len(calls) != 0 || next3 != next2 {
		t.Fatalf("at EOF: calls = %+v, next = %d, err %v", calls, next3, err)
	}
}

func TestReadSummary_Incremental(t *testing.T) {
	p := writeLines(t, header,
		`{"type":"message","id":"s","parentId":null,"timestamp":"t","message":{"role":"system","content":"user rules"}}`,
		`{"type":"message","id":"u","parentId":null,"timestamp":"t","message":{"role":"user","content":"\n  first line  \nsecond"}}`,
		`{"type":"session_info","id":"i1","parentId":"u","timestamp":"t","name":"one"}`)
	sum, next, err := session.ReadSummary(p, 0, session.Summary{})
	if err != nil || sum != (session.Summary{Name: "one", FirstPrompt: "first line"}) {
		t.Fatalf("first = %+v, %v", sum, err)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	defer f.Close()
	f.WriteString(user("u2", "t", "later") + "\n" + `{"type":"session_info","id":"i2","parentId":"u2","timestamp":"t","name":"two"}` + "\n")
	sum, _, err = session.ReadSummary(p, next, sum)
	if err != nil || sum != (session.Summary{Name: "two", FirstPrompt: "first line"}) {
		t.Fatalf("second = %+v, %v", sum, err)
	}
}
