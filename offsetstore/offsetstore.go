// Package offsetstore persists byte offsets for incremental JSONL reads
// between short-lived process invocations, so each tick reads only the new
// bytes written since last time (O(delta) vs O(n)).
//
// Partial-last-line rule (do not change; tail-claude-hud depends on it): an
// unterminated final line is ALWAYS deferred to the next tick, even when it
// already parses as complete JSON. The consumer is a fresh process that
// gets another chance in ~300ms, so waiting is free and guarantees it never
// acts on a half-written line. This is the opposite of
// transcript.ReadSessionIncremental's rule, which keeps a parseable tail —
// a resident watcher may never get another file event.
//
// The store also carries an opaque extraction snapshot alongside the
// offset, versioned by a CALLER-SUPPLIED schema version: bump the version
// whenever downstream extraction semantics change and the transcript is
// re-read from byte 0 with a nil snapshot.
package offsetstore

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// stateFileTTL is how long a state file must go unmodified before it is
	// eligible for deletion. 30 days covers any realistic session gap.
	stateFileTTL = 30 * 24 * time.Hour

	// sweepOdds controls how often a save triggers a stale-file sweep.
	// At 1-in-100, a session ticking at 5/s sweeps roughly every 20 seconds —
	// precise enough given the 30-day TTL.
	sweepOdds = 100
)

// stateFile is the JSON structure persisted to disk.
type stateFile struct {
	SchemaVersion      int             `json:"schema_version,omitempty"`
	TranscriptPath     string          `json:"transcript_path"`
	ByteOffset         int64           `json:"byte_offset"`
	SessionStart       string          `json:"session_start"` // RFC3339, informational
	ExtractionSnapshot json.RawMessage `json:"extraction_snapshot,omitempty"`
}

// Store handles byte-offset tracking for incremental reads. One Store
// manages a directory of per-transcript state files (keyed by a hash of
// the transcript path).
type Store struct {
	stateDir       string
	schemaVersion  int
	offset         int64
	lastPath       string
	snapshot       json.RawMessage // set by SetSnapshot; included in next Save
	loadedSnapshot json.RawMessage // loaded from disk by loadState; returned by LoadSnapshot
}

// New creates a store using the given directory for state files.
//
// schemaVersion versions the caller's extraction snapshot: when the stored
// version differs, the snapshot is discarded and the transcript re-read
// from byte 0. Bump it whenever extraction logic changes in a way that
// would produce different results from the same transcript data.
func New(stateDir string, schemaVersion int) *Store {
	return &Store{stateDir: stateDir, schemaVersion: schemaVersion}
}

// pathHash returns the first 12 characters of the SHA-256 hex digest of a path.
// This is the key used in the state file name.
func pathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", sum)[:12]
}

// stateFilePath returns the full path to the state file for a given transcript path.
func (s *Store) stateFilePath(transcriptPath string) string {
	name := ".ts-" + pathHash(transcriptPath) + ".json"
	return filepath.Join(s.stateDir, name)
}

// loadState reads and parses the state file. Returns zero-value stateFile if
// the file is missing or contains invalid JSON (spec: start from byte 0).
// As a side-effect it stores the extraction_snapshot in s.loadedSnapshot so
// callers can retrieve it via LoadSnapshot after calling ReadIncremental.
func (s *Store) loadState(transcriptPath string) stateFile {
	data, err := os.ReadFile(s.stateFilePath(transcriptPath))
	if err != nil {
		s.loadedSnapshot = nil
		return stateFile{}
	}
	var sf stateFile
	if json.Unmarshal(data, &sf) != nil {
		s.loadedSnapshot = nil
		return stateFile{}
	}
	// Schema mismatch: extraction logic has changed since this snapshot was
	// written. Discard it so the transcript is re-read from byte 0.
	if sf.SchemaVersion != s.schemaVersion {
		s.loadedSnapshot = nil
		return stateFile{}
	}
	s.loadedSnapshot = sf.ExtractionSnapshot
	return sf
}

// LoadSnapshot returns the extraction snapshot that was loaded from disk during
// the most recent ReadIncremental call. Returns nil when no snapshot is
// available (e.g., first run, corrupt state, schema bump, or path mismatch).
func (s *Store) LoadSnapshot() json.RawMessage {
	return s.loadedSnapshot
}

// SetSnapshot stores data so it will be included in the next Save call.
func (s *Store) SetSnapshot(data json.RawMessage) {
	s.snapshot = data
}

// ReadIncremental reads new lines from the transcript since the last read.
// It returns complete, valid-JSON lines only. A partial last line (mid-write)
// is discarded; the offset is not advanced past it so the next tick picks it up.
//
// Reset conditions (start from byte 0):
//   - State file missing or corrupt
//   - Schema version mismatch
//   - Stored path differs (new session)
//   - Stored offset exceeds current file size (truncation)
func (s *Store) ReadIncremental(transcriptPath string) ([]string, error) {
	sf := s.loadState(transcriptPath)

	f, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Determine start offset.
	var startOffset int64
	if sf.TranscriptPath == transcriptPath && sf.ByteOffset > 0 {
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if sf.ByteOffset > fi.Size() {
			// Truncated transcript: reset to beginning and discard snapshot.
			startOffset = 0
			s.loadedSnapshot = nil
		} else {
			startOffset = sf.ByteOffset
		}
	} else if sf.TranscriptPath != "" && sf.TranscriptPath != transcriptPath {
		// Path mismatch (new session): discard snapshot.
		s.loadedSnapshot = nil
	}

	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return nil, err
	}

	newBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	lines, consumed := splitLines(newBytes)
	s.offset = startOffset + consumed
	s.lastPath = transcriptPath

	return lines, nil
}

// splitLines splits raw bytes into complete lines, discarding the last segment
// if it is not newline-terminated (partial write protection).
//
// Returns the valid lines and the number of bytes consumed (excluding any
// discarded partial last line).
func splitLines(data []byte) (lines []string, consumed int64) {
	if len(data) == 0 {
		return nil, 0
	}

	// Split on newlines.
	var segments [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			segments = append(segments, data[start:i])
			start = i + 1
		}
	}
	// Any remaining bytes after the last newline form a trailing segment.
	trailing := data[start:]

	// Determine how many bytes are "confirmed complete".
	// If there is a trailing segment (no trailing newline), we need to check
	// whether it is valid JSON before including it.
	confirmedEnd := int64(start) // bytes up to and including the last newline

	// Collect valid JSON lines from newline-terminated segments.
	var result []string
	for _, seg := range segments {
		if len(seg) == 0 {
			continue
		}
		if json.Valid(seg) {
			result = append(result, string(seg))
		}
		// Invalid JSON lines within the file are skipped but we still advance past them
		// (they are complete lines that happen to be non-JSON or malformed entries).
		// The offset advances to confirmedEnd regardless.
	}

	// Handle trailing (no trailing newline): check if valid JSON.
	if len(trailing) > 0 {
		if json.Valid(trailing) {
			// It's a complete line that just happens to lack a newline yet.
			// But per spec, we must not advance past a line that may be a partial write.
			// A line without a trailing newline could be mid-write, so we discard it
			// and do not advance the offset past confirmedEnd.
			_ = trailing // discard: may be partial write
		}
		// Either way, do not advance past confirmedEnd for trailing bytes.
	}

	consumed = confirmedEnd

	return result, consumed
}

// Save persists the current offset (and any snapshot set via SetSnapshot)
// to disk atomically. Writes to a temp file then renames to prevent partial
// reads from concurrent processes.
func (s *Store) Save(transcriptPath string) error {
	if err := os.MkdirAll(s.stateDir, 0o755); err != nil {
		return err
	}

	sf := stateFile{
		SchemaVersion:      s.schemaVersion,
		TranscriptPath:     transcriptPath,
		ByteOffset:         s.offset,
		ExtractionSnapshot: s.snapshot,
	}

	data, err := json.Marshal(sf)
	if err != nil {
		return err
	}

	target := s.stateFilePath(transcriptPath)
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	if err := os.Rename(tmp, target); err != nil {
		return err
	}

	if rand.Intn(sweepOdds) == 0 {
		s.sweepStaleStateFiles()
	}

	return nil
}

// sweepStaleStateFiles removes state files that have not been modified in
// stateFileTTL. It is best-effort: errors are silently ignored so a failed
// sweep never disrupts the normal write path.
func (s *Store) sweepStaleStateFiles() {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-stateFileTTL)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".ts-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(s.stateDir, name)) //nolint:errcheck
		}
	}
}
