package jsonl

import (
	"bytes"
	"os"
)

// TailLines reads at most maxBytes from the end of the file at path and
// returns the newline-split segments, oldest first. When the read window
// starts mid-file, the first segment is almost certainly a truncated line —
// callers must tolerate (or skip) an unparseable leading segment. Empty
// segments are omitted.
//
// This is the bounded bottom-up scan pattern for huge transcripts: the
// interesting entry (e.g. the last assistant model) sits near the end of a
// file that may be hundreds of MB.
func TailLines(path string, maxBytes int64) ([][]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxLineSize
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	offset := info.Size() - maxBytes
	if offset < 0 {
		offset = 0
	}
	buf := make([]byte, info.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}

	raw := bytes.Split(buf, []byte("\n"))
	lines := make([][]byte, 0, len(raw))
	for _, l := range raw {
		if len(l) > 0 {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// ReverseScan calls fn for each line in the last maxBytes of the file at
// path, newest first, stopping when fn returns false. The final line handed
// to fn (the oldest in the window) may be truncated when the window starts
// mid-file; callers must tolerate an unparseable segment.
func ReverseScan(path string, maxBytes int64, fn func(line []byte) bool) error {
	lines, err := TailLines(path, maxBytes)
	if err != nil {
		return err
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if !fn(lines[i]) {
			return nil
		}
	}
	return nil
}
