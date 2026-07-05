package jsonl_test

import (
	"fmt"
	"strings"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

func ExampleScanLines() {
	data := "{\"a\":1}\n{\"b\":2}\n"
	_ = jsonl.ScanLines(strings.NewReader(data), func(line string) bool {
		fmt.Println(line)
		return true
	})
	// Output:
	// {"a":1}
	// {"b":2}
}

func ExampleNewReader_offsetTracking() {
	r := jsonl.NewReader(strings.NewReader("{\"a\":1}\n{\"partial\":"))
	for {
		if _, ok := r.Next(); !ok {
			break
		}
	}
	// TerminatedBytesRead excludes the unterminated tail, so an
	// incremental reader can resume without splitting the half-written line.
	fmt.Println(r.TerminatedBytesRead() < r.BytesRead())
	// Output: true
}
