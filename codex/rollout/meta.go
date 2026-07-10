package rollout

import (
	"io"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// SessionMeta reads a rollout stream up to its first parseable entry
// and returns it if it is the "session_meta" header. Codex CLI writes
// session_meta as every rollout's first line (verified live on
// 0.144.1). It stops at the first parseable entry rather than reading
// the whole file. Unparseable leading lines (a partially written header
// on a brand-new file) are skipped, and the scan stops at the first
// entry that does parse, whatever its type. ok is false when the stream holds no
// parseable entry or its first parseable entry is not session_meta.
// The returned error reports only a failure to read the underlying
// stream (mirrors TrailingState).
func SessionMeta(r io.Reader) (meta Entry, ok bool, err error) {
	lr := jsonl.NewReader(r)
	for {
		line, more := lr.Next()
		if !more {
			return Entry{}, false, lr.Err()
		}
		entry, parsed := ParseEntry([]byte(line))
		if !parsed {
			continue
		}
		if entry.Type != "session_meta" {
			return Entry{}, false, nil
		}
		return entry, true, nil
	}
}
