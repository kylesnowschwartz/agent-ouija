// Package discover enumerates Pi coding agent session files on disk.
//
// Pi keeps one folder per working directory under pidir.Root.SessionsDir
// (see SessionDirForCwd) and one JSONL file per session in it, named
// <timestamp>_<session id>.jsonl. A directory named by
// $PI_CODING_AGENT_SESSION_DIR instead holds the files of every cwd
// directly; DiscoverSessions lists either kind of directory.
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

// fileNameRE matches Pi's session file name without the .jsonl
// extension: an ISO-8601 UTC timestamp with ':' and '.' replaced by '-',
// an underscore, then the session id. The timestamp holds no underscore,
// so the first underscore ends it; the id may itself contain '_', '.'
// and '-' but starts and ends with an alphanumeric character.
var fileNameRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z_([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)$`)

// headerScanBytes bounds the first-line read. Pi bounds its own header
// scan at 1 MiB.
const headerScanBytes = 1 << 20

// Session is one discovered session file and the fields of its header
// line. Header fields are zero when the first line is missing, is not
// valid JSON, or is not a "session" header.
type Session struct {
	Path string
	// ID is the session id from the file name (see SessionIDFromPath).
	ID string
	// Cwd is the header's cwd: the working directory the session runs
	// in. It is the only reliable source; the folder name is lossy.
	Cwd string
	// Started is the header's timestamp; zero when absent or unparseable.
	Started time.Time
	// ParentSession is the header's parentSession (the session file this
	// one was forked from); "" when absent.
	ParentSession string
	ModTime       time.Time
}

// SessionDirForCwd returns the folder Pi stores cwd's sessions in under
// sessionsDir (conventionally pidir.Root.SessionsDir()): "--", cwd with
// one leading '/' or '\' removed and every '/', '\' and ':' replaced by
// '-', then "--". Pi resolves cwd to an absolute path first; pass an
// absolute, cleaned path. The encoding is lossy, so a folder name is
// never decoded back to a cwd -- read Session.Cwd instead.
func SessionDirForCwd(sessionsDir, cwd string) string {
	p := cwd
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		p = p[1:]
	}
	p = strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(p)
	return filepath.Join(sessionsDir, "--"+p+"--")
}

// DiscoverSessions lists the session files directly inside dir (one
// per-cwd folder, or a flat override directory), reading only each
// file's first line. Files whose name does not follow Pi's
// <timestamp>_<id>.jsonl convention are skipped; a file whose header is
// unreadable is returned with zero header fields. Results are sorted by
// ModTime, newest first, ties broken by Path ascending. A missing dir
// returns an empty slice, not an error.
func DiscoverSessions(dir string) ([]Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id := sessionIDFromName(e.Name())
		if id == "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		s := Session{Path: filepath.Join(dir, e.Name()), ID: id, ModTime: info.ModTime()}
		readHeader(&s)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// SessionIDFromPath returns the session id from a session file path, or
// "" when the base name does not follow Pi's <timestamp>_<id>.jsonl
// convention. It is the rule DiscoverSessions applies.
func SessionIDFromPath(path string) string {
	return sessionIDFromName(filepath.Base(path))
}

func sessionIDFromName(name string) string {
	base, ok := strings.CutSuffix(name, ".jsonl")
	if !ok {
		return ""
	}
	m := fileNameRE.FindStringSubmatch(base)
	if m == nil {
		return ""
	}
	return m[1]
}

// readHeader fills s's header fields from the first line of s.Path,
// leaving them zero on any failure.
func readHeader(s *Session) {
	f, err := os.Open(s.Path)
	if err != nil {
		return
	}
	defer f.Close()
	lr := jsonl.NewReaderSize(f, headerScanBytes)
	line, ok := lr.Next()
	if !ok {
		return
	}
	var h struct {
		Type          string `json:"type"`
		Timestamp     string `json:"timestamp"`
		Cwd           string `json:"cwd"`
		ParentSession string `json:"parentSession"`
	}
	if json.Unmarshal([]byte(line), &h) != nil || h.Type != "session" {
		return
	}
	s.Cwd = h.Cwd
	s.ParentSession = h.ParentSession
	s.Started, _ = time.Parse(time.RFC3339Nano, h.Timestamp)
}
