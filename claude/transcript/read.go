package transcript

import (
	"os"
	"path/filepath"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// ReadSession reads a JSONL session file and returns the fully processed chunk list.
func ReadSession(path string) ([]Chunk, error) {
	msgs, _, err := ReadSessionIncremental(path, 0)
	if err != nil {
		return nil, err
	}
	return BuildChunks(msgs), nil
}

// ReadSessionIncremental reads new lines from a session file starting at the
// given byte offset. Returns newly classified messages, the updated offset,
// and any error. This is the building block for live tailing -- the caller
// accumulates classified messages and re-runs BuildChunks after each call.
//
// Partial-last-line rule (do not change; tail-claude depends on it): an
// unterminated final line is KEPT when it already parses as complete JSON.
// A resident watcher may get no further file event for that line, so
// deferring it could mean never showing the final message. This is the
// opposite of offsetstore's rule, which always defers — see that package.
func ReadSessionIncremental(path string, offset int64) ([]ClassifiedMsg, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset, err
	}

	lr := jsonl.NewReader(f)

	var msgs []ClassifiedMsg

	for {
		line, ok := lr.Next()
		if !ok {
			break
		}
		if !lr.LastLineTerminated() {
			// EOF-truncated tail. A JSONL line is one JSON object, so a
			// half-written append can never parse as a complete entry
			// (json.Unmarshal rejects both prefixes and trailing garbage).
			// If the tail parses, the record is complete and the file just
			// lacks a trailing newline -- keep it and consume its bytes.
			// Otherwise it is an append still in progress: skip it and
			// exclude it from the offset (TerminatedBytesRead below) so the
			// next incremental read picks up the completed line intact.
			entry, ok := ParseEntry([]byte(line))
			if !ok {
				break
			}
			if msg, ok := Classify(entry); ok {
				msgs = append(msgs, msg)
			}
			ResolvePersistedOutputs(msgs, filepath.Dir(path))
			return msgs, offset + lr.BytesRead(), nil
		}
		entry, ok := ParseEntry([]byte(line))
		if !ok {
			continue
		}
		msg, ok := Classify(entry)
		if !ok {
			continue
		}
		msgs = append(msgs, msg)
	}
	if err := lr.Err(); err != nil {
		return msgs, offset + lr.TerminatedBytesRead(), err
	}

	// Inline externalized tool results ({projectDir}/{session}/tool-results/).
	ResolvePersistedOutputs(msgs, filepath.Dir(path))

	return msgs, offset + lr.TerminatedBytesRead(), nil
}
