package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// tailScanBytes bounds bottom-up transcript scans. Transcripts grow to
// hundreds of MB; the entries these scans want sit at the end. 256 KiB is
// the window gearshifter shipped with — treat it as part of the
// LastAssistantModel contract.
const tailScanBytes = 256 * 1024

// ScanTailEntries calls fn for each parseable entry in the last maxBytes of
// the session file, newest first, stopping when fn returns false. Entries
// are parsed leniently (uuid-less session-metadata records are included).
// Unparseable segments — including the almost-certainly-truncated oldest
// line when the window starts mid-file — are skipped silently.
//
// A maxBytes of 0 means the default 256 KiB window.
func ScanTailEntries(path string, maxBytes int64, fn func(Entry) bool) error {
	if maxBytes <= 0 {
		maxBytes = tailScanBytes
	}
	return jsonl.ReverseScan(path, maxBytes, func(line []byte) bool {
		entry, ok := ParseEntryLenient(line)
		if !ok {
			return true // skip unparseable (possibly truncated) segment
		}
		return fn(entry)
	})
}

// LastAssistantModel scans the session transcript bottom-up for the last
// assistant entry's message.model (the ccusage statusline pattern),
// returning it with the transcript's mtime, or ("", zero time) when
// unresolvable.
//
// Contract (pinned to gearshifter's semantics — do not change):
//   - only the last 256 KiB are read, so huge transcripts stay cheap;
//   - "<synthetic>" models are skipped and the scan CONTINUES to the next
//     older assistant entry (a synthetic entry is often the newest one);
//   - the returned mtime is the transcript file's, letting callers
//     arbitrate freshness against settings.json (the fresher file wins) —
//     the mtime and the scanned window come from ONE Stat so the pair is
//     always coherent;
//   - lines decode through a minimal {type, message.model} struct, NOT the
//     full Entry: format drift in an unrelated modeled field must never
//     reject the line and silently drop the model.
func LastAssistantModel(path string) (model string, modTime time.Time) {
	model, _, modTime = lastAssistantModelScan(path)
	return model, modTime
}

// LastAssistantModelAt is LastAssistantModel arbitrating by ENTRY time:
// it returns the matched assistant entry's own timestamp instead of the
// transcript file's mtime, falling back to the file mtime when the entry
// carries no parseable timestamp.
//
// Why it exists (gearshifter, 2026-07-05): a transcript's file mtime
// moves on EVERY appended entry — user prompts, local slash commands —
// so "transcript file newer than settings.json" does not mean "the
// model fact is newer". A live session's /model change writes settings
// and appends a user entry in the same breath; file-mtime arbitration
// then keeps showing the pre-change model until the next assistant
// reply. The entry timestamp is when the model was actually observed.
func LastAssistantModelAt(path string) (model string, at time.Time) {
	model, entryTime, fileTime := lastAssistantModelScan(path)
	if !entryTime.IsZero() {
		return model, entryTime
	}
	return model, fileTime
}

// lastAssistantModelScan is the shared bottom-up scan: the matched
// entry's model and parsed timestamp (zero when absent), plus the file
// mtime from the SAME Stat as the scanned window so the pair is always
// coherent.
func lastAssistantModelScan(path string) (model string, entryTime, fileTime time.Time) {
	info, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, time.Time{}
	}
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, time.Time{}
	}
	defer f.Close()
	offset := max(0, info.Size()-tailScanBytes)
	buf := make([]byte, info.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return "", time.Time{}, time.Time{}
	}
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		// Cheap pre-filter: most lines carry no model field at all.
		if !bytes.Contains(lines[i], []byte(`"model"`)) {
			continue
		}
		var entry struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if err := json.Unmarshal(lines[i], &entry); err != nil {
			continue // first line of the tail window may be truncated
		}
		if entry.Type == "assistant" && entry.Message.Model != "" && entry.Message.Model != "<synthetic>" {
			return entry.Message.Model, ParseTimestamp(entry.Timestamp), info.ModTime()
		}
	}
	return "", time.Time{}, time.Time{}
}
