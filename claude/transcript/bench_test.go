package transcript_test

import (
	"fmt"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

// benchEntry returns a realistic assistant JSONL line with tool_use blocks.
// Mirrors tail-claude-hud's synthetic-entry benchmarks; these are acceptance
// gates for the HUD's per-tick latency budget (fresh process every ~300ms).
func benchEntry(i int) []byte {
	uuid := fmt.Sprintf("uuid-%08d", i)
	ts := "2024-03-15T14:22:33.123456789Z"
	toolID1 := fmt.Sprintf("tu-%08d-a", i)
	toolID2 := fmt.Sprintf("tu-%08d-b", i)
	return fmt.Appendf(nil,
		`{"type":"assistant","uuid":%q,"timestamp":%q,"slug":"bench-session","message":{"role":"assistant","model":"claude-opus-4-8","stop_reason":"tool_use","content":[{"type":"tool_use","id":%q,"name":"Bash","input":{"command":"go test ./..."}},{"type":"tool_use","id":%q,"name":"Read","input":{"file_path":"/some/path/file.go"}}]}}`,
		uuid, ts, toolID1, toolID2,
	)
}

func BenchmarkParseEntry(b *testing.B) {
	b.ReportAllocs()
	line := benchEntry(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = transcript.ParseEntry(line)
	}
}

func BenchmarkParseEntryLenient(b *testing.B) {
	b.ReportAllocs()
	line := benchEntry(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = transcript.ParseEntryLenient(line)
	}
}

func BenchmarkExtractContentBlocks(b *testing.B) {
	b.ReportAllocs()
	e, ok := transcript.ParseEntry(benchEntry(1))
	if !ok {
		b.Fatal("ParseEntry failed")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transcript.ExtractContentBlocks(e)
	}
}

func BenchmarkClassify(b *testing.B) {
	b.ReportAllocs()
	e, ok := transcript.ParseEntry(benchEntry(1))
	if !ok {
		b.Fatal("ParseEntry failed")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = transcript.Classify(e)
	}
}
