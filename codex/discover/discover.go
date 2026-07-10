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

// sessionIDRE matches the trailing UUID Codex CLI appends to every
// rollout filename: rollout-<timestamp>-<uuid>.jsonl.
var sessionIDRE = regexp.MustCompile(`[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

// Rollout is one discovered rollout transcript file.
type Rollout struct {
	Path      string
	SessionID string
	ModTime   time.Time
}

// DiscoverRollouts finds every rollout-*.jsonl file under sessionsDir
// (conventionally codexdir.Root.SessionsDir()), which nests files under
// YYYY/MM/DD. Files whose name carries no trailing UUID are skipped. A
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

// SessionIDFromPath extracts a trailing lowercase UUID from the
// extension-stripped base name, or "" if there is none. It uses the
// same filename rule as DiscoverRollouts. For paths obtained outside
// DiscoverRollouts' tree walk (e.g. from lsof on a live Codex process).
func SessionIDFromPath(path string) string {
	return sessionIDFromName(filepath.Base(path))
}

// sessionIDFromName extracts the trailing UUID from a rollout filename,
// or "" if the name doesn't match the convention.
func sessionIDFromName(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return sessionIDRE.FindString(base)
}

// ThreadNames reads a session_index.jsonl file (conventionally
// codexdir.Root.SessionIndexPath()) and returns id -> thread_name. Each
// line is {"id":"...","thread_name":"...","updated_at":"..."}; Codex CLI
// only appends to this file, so the last line for a given id wins.
// Malformed lines are skipped. A missing file returns an empty, non-nil
// map and no error.
func ThreadNames(path string) (map[string]string, error) {
	names := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return names, nil
		}
		return names, err
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
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if entry.ID == "" || entry.ThreadName == "" {
			continue
		}
		names[entry.ID] = entry.ThreadName
	}
	return names, lr.Err()
}
