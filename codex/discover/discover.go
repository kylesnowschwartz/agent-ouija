// Package discover enumerates Codex CLI rollout transcripts on disk and
// resolves their thread names.
package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// sessionIDRE matches the UUIDs at the end of a rollout filename. Codex
// CLI names rollouts rollout-<timestamp>-<thread id>.jsonl or
// rollout-<timestamp>-<thread id>_<second id>.jsonl; group 1 is the
// thread id in both forms.
var sessionIDRE = regexp.MustCompile(`([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})(?:_[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})?$`)

// Rollout is one discovered rollout transcript file.
type Rollout struct {
	Path      string
	SessionID string
	ModTime   time.Time
}

// DiscoverRollouts finds every rollout-*.jsonl file under sessionsDir
// (conventionally codexdir.Root.SessionsDir()), which nests files under
// YYYY/MM/DD. SessionID is the thread id from the file name (see
// SessionIDFromPath); files whose name carries none are skipped. A
// missing sessionsDir returns an empty slice, not an error -- mirrors
// claude/discover's convention of a quiet result for a not-yet-existing
// state directory.
func DiscoverRollouts(sessionsDir string) ([]Rollout, error) {
	if _, err := os.Stat(sessionsDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Rollout
	err := filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subdir mid-walk -- skip, don't fail the whole scan
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		id := sessionIDFromName(d.Name())
		if id == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, Rollout{Path: path, SessionID: id, ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return out, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// SessionIDFromPath returns the thread id from a rollout path's
// extension-stripped base name, or "" if there is none. The base name
// ends in <thread id> or <thread id>_<second id>; the thread id is the
// one session_meta and session_index.jsonl use. It uses the
// same filename rule as DiscoverRollouts. For paths obtained outside
// DiscoverRollouts' tree walk (e.g. from lsof on a live Codex process).
func SessionIDFromPath(path string) string {
	return sessionIDFromName(filepath.Base(path))
}

// sessionIDFromName extracts the thread id from a rollout filename, or
// "" if the name doesn't match the convention.
func sessionIDFromName(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	m := sessionIDRE.FindStringSubmatch(base)
	if m == nil {
		return ""
	}
	return m[1]
}

// ThreadRecord is the latest session_index.jsonl line for one thread.
type ThreadRecord struct {
	Name string
	// UpdatedAt is updated_at parsed as RFC 3339; zero when the line
	// has no updated_at or it does not parse.
	UpdatedAt time.Time
}

// ThreadRecords reads a session_index.jsonl file (conventionally
// codexdir.Root.SessionIndexPath()) and returns thread id -> its latest
// record. Each line is {"id":"...","thread_name":"...","updated_at":"..."};
// Codex CLI only appends to this file, so the last line for a given id
// wins. Malformed lines and lines without an id or thread_name are
// skipped. A missing file returns an empty, non-nil map and no error.
func ThreadRecords(path string) (map[string]ThreadRecord, error) {
	records := map[string]ThreadRecord{}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return records, nil
		}
		return records, err
	}
	defer f.Close()

	lr := jsonl.NewReader(f)
	for {
		line, ok := lr.Next()
		if !ok {
			break
		}
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry.ID == "" || entry.ThreadName == "" {
			continue
		}
		updated, _ := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
		records[entry.ID] = ThreadRecord{Name: entry.ThreadName, UpdatedAt: updated}
	}
	return records, lr.Err()
}

// ThreadNames returns thread id -> thread_name from a session_index.jsonl
// file, with the same rules as ThreadRecords.
func ThreadNames(path string) (map[string]string, error) {
	records, err := ThreadRecords(path)
	names := make(map[string]string, len(records))
	for id, r := range records {
		names[id] = r.Name
	}
	return names, err
}
