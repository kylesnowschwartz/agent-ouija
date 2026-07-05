// Package jsonl provides line-oriented IO primitives for JSONL files that
// may be appended to while being read. It is provider-neutral: callers get
// raw lines and decide what to parse, so unmodeled entry shapes remain
// reachable without a library release.
//
// The oversized-line rule is load-bearing: lines exceeding the max line
// size are silently skipped, never aborting iteration (unlike
// bufio.Scanner, whose ErrTooLong permanently kills a scan). The default
// max of 64 MiB pins tail-claude's semantics verbatim; callers with
// different needs supply their own via NewReaderSize.
package jsonl

import (
	"bufio"
	"io"
)

const (
	// initialBufSize is the starting buffer capacity for the line reader.
	initialBufSize = 64 * 1024

	// DefaultMaxLineSize is the maximum allowed line length when the caller
	// does not supply one via NewReaderSize. Lines exceeding the limit are
	// silently skipped rather than aborting the entire scan. 64 MiB
	// accommodates even the largest Claude API responses (inline images,
	// giant pre-2.1.19x tool results).
	DefaultMaxLineSize = 64 * 1024 * 1024
)

// Reader reads JSONL files line by line, skipping lines that exceed the
// max line size rather than aborting. The buffer starts small and grows on
// demand. After iteration, call Err() to check for I/O errors (not EOF).
//
// Ported from agentsview's internal/parser/linereader.go with the addition
// of BytesRead() for incremental offset tracking.
type Reader struct {
	r               *bufio.Reader
	maxLen          int // 0 means use DefaultMaxLineSize
	buf             []byte
	err             error
	bytesRead       int64
	terminatedBytes int64 // bytes consumed through the last \n-terminated line
	lastTerminated  bool  // whether the most recently read line ended with \n
}

// NewReader returns a Reader with the DefaultMaxLineSize limit.
func NewReader(r io.Reader) *Reader {
	return NewReaderSize(r, 0)
}

// NewReaderSize returns a Reader that skips lines longer than maxLineSize
// bytes. A maxLineSize of 0 (or less) means DefaultMaxLineSize.
func NewReaderSize(r io.Reader, maxLineSize int) *Reader {
	if maxLineSize < 0 {
		maxLineSize = 0
	}
	return &Reader{
		r:      bufio.NewReaderSize(r, initialBufSize),
		maxLen: maxLineSize,
		buf:    make([]byte, 0, initialBufSize),
	}
}

// ScanLines calls fn for each non-empty line of r, stopping early when fn
// returns false. Lines exceeding DefaultMaxLineSize are skipped rather than
// aborting the scan — unlike bufio.Scanner, whose ErrTooLong permanently
// kills iteration, so one huge line (e.g. pasted image data) would hide
// every line after it. Returns the first I/O error encountered, or nil.
func ScanLines(r io.Reader, fn func(line string) bool) error {
	lr := NewReader(r)
	for {
		line, ok := lr.Next()
		if !ok {
			return lr.Err()
		}
		if !fn(line) {
			return nil
		}
	}
}

// Next returns the next non-empty line (without trailing newline) and true,
// or ("", false) at EOF or I/O error. After the loop, call Err() to
// distinguish EOF from I/O failure.
func (lr *Reader) Next() (string, bool) {
	for {
		line, err := lr.readLine()
		if err != nil {
			if err != io.EOF {
				lr.err = err
			}
			return "", false
		}
		if line != "" {
			return line, true
		}
		// Empty line or skipped oversized line -- continue.
	}
}

// Err returns the first non-EOF I/O error encountered, or nil.
func (lr *Reader) Err() error {
	return lr.err
}

// BytesRead returns the total bytes consumed from the reader, including
// skipped lines, newline delimiters, and any EOF-truncated trailing line.
func (lr *Reader) BytesRead() int64 {
	return lr.bytesRead
}

// TerminatedBytesRead returns the bytes consumed through the last
// newline-terminated line, excluding an EOF-truncated tail. Used by
// ReadSessionIncremental for offset tracking during live tailing: a
// half-written trailing line must be re-read intact on the next call.
func (lr *Reader) TerminatedBytesRead() int64 {
	return lr.terminatedBytes
}

// LastLineTerminated reports whether the most recently returned line ended
// with a newline. False means the line was cut off at EOF -- during live
// tailing that is typically an append still in progress.
func (lr *Reader) LastLineTerminated() bool {
	return lr.lastTerminated
}

// readLine reads a full line, returning "" for blank/oversized lines and
// a non-nil error only at EOF or read failure.
//
// Uses bufio.Reader.ReadSlice('\n') rather than ReadLine because ReadSlice
// reports whether the delimiter was actually found: a complete line returns
// err == nil, while an EOF-truncated final line (a JSONL append still in
// progress) returns io.EOF alongside the partial data. That distinction
// drives lastTerminated/terminatedBytes so incremental readers can exclude
// a half-written tail from their offset and re-read it intact later.
//
// When accumulated bytes exceed the max line size, the buffer is discarded
// and the rest of the line is consumed (to keep bytesRead accurate), then ""
// is returned so Next() skips to the following line.
func (lr *Reader) readLine() (string, error) {
	lr.buf = lr.buf[:0]
	oversized := false
	var lineBytes int64

	limit := DefaultMaxLineSize
	if lr.maxLen > 0 {
		limit = lr.maxLen
	}

	for {
		chunk, err := lr.r.ReadSlice('\n')
		// Count data bytes from every chunk, including oversized lines.
		lineBytes += int64(len(chunk))

		switch err {
		case bufio.ErrBufferFull:
			// Partial read -- keep accumulating.
			if !oversized {
				lr.buf = append(lr.buf, chunk...)
				if len(lr.buf) > limit {
					oversized = true
					lr.buf = lr.buf[:0]
				}
			}
			continue

		case nil:
			// Delimiter found; chunk includes the trailing \n.
			lr.bytesRead += lineBytes
			lr.terminatedBytes = lr.bytesRead
			lr.lastTerminated = true
			if oversized {
				return "", nil // done skipping
			}
			data := chunk[:len(chunk)-1]
			if len(data) > 0 && data[len(data)-1] == '\r' {
				data = data[:len(data)-1]
			}
			lr.buf = append(lr.buf, data...)
			if len(lr.buf) > limit {
				return "", nil
			}
			return string(lr.buf), nil

		case io.EOF:
			if lineBytes == 0 {
				// Clean EOF at a line boundary.
				return "", io.EOF
			}
			// Final line ended at EOF without \n. Count the bytes but
			// leave terminatedBytes at the last complete line.
			lr.bytesRead += lineBytes
			lr.lastTerminated = false
			if oversized {
				return "", nil
			}
			lr.buf = append(lr.buf, chunk...)
			if len(lr.buf) > limit {
				return "", nil
			}
			return string(lr.buf), nil

		default:
			return "", err
		}
	}
}
