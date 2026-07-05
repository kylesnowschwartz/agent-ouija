// Package registry reads Claude Code's live-session process registry
// ({root}/sessions/*.json) and resolves which session belongs to a given
// terminal pane.
//
// Mechanism (verified against Claude Code 2.1.195/2.1.201): each running
// session writes a JSON file with its pid, sessionId, cwd, startedAt, and
// a liveness map (status/updatedAt/statusUpdatedAt). Files linger after
// exit, so entries must be liveness-checked before use.
//
// Format drift, absorbed 2026-07-05: startedAt was once an RFC3339 string
// and is an epoch-milliseconds number on current builds. EpochMS accepts
// both; a strict string decode made the whole registry invisible.
//
// Ported from gearshifter@e718c8e internal/agent/claude/session.go with the
// process tree made injectable for tests.
package registry

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// EpochMS is a millisecond Unix timestamp tolerant of every encoding the
// registry has used: a JSON number (current builds), an RFC3339 string or
// numeric string (older builds). Unrecognized shapes decode to 0 rather
// than failing the whole entry — a timestamp is never load-bearing enough
// to hide a session.
type EpochMS int64

// UnmarshalJSON implements the tolerant decode described on the type.
func (e *EpochMS) UnmarshalJSON(data []byte) error {
	*e = 0
	s := strings.TrimSpace(string(data))
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*e = EpochMS(n)
		return nil
	}
	var str string
	if json.Unmarshal(data, &str) != nil {
		return nil
	}
	if n, err := strconv.ParseInt(str, 10, 64); err == nil {
		*e = EpochMS(n)
	} else if t, err := time.Parse(time.RFC3339, str); err == nil {
		*e = EpochMS(t.UnixMilli())
	}
	return nil
}

// Time returns the timestamp as a time.Time (zero EpochMS → Unix epoch;
// check for 0 before formatting if "unknown" must render differently).
func (e EpochMS) Time() time.Time { return time.UnixMilli(int64(e)) }

// Live is one file of Claude Code's live-session registry. Only the fields
// consumers need are decoded.
//
// Status/UpdatedAt/StatusUpdatedAt are the session's self-reported
// liveness map: observed status values are "busy", "idle", and "waiting",
// refreshed while the process runs. Files written by non-interactive
// entrypoints (Kind "sdk-cli") may omit status entirely — treat an empty
// Status as "unknown", not idle. Name is the session's display name when
// one has been set; Kind is the entrypoint kind ("interactive",
// "sdk-cli", ...).
type Live struct {
	PID             int     `json:"pid"`
	SessionID       string  `json:"sessionId"`
	Cwd             string  `json:"cwd"`
	StartedAt       EpochMS `json:"startedAt"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Status          string  `json:"status"`
	UpdatedAt       EpochMS `json:"updatedAt"`
	StatusUpdatedAt EpochMS `json:"statusUpdatedAt"`
}

// Alive reports whether the entry's process still exists (signal 0 probe).
// Registry files linger after exit; always check before trusting an entry.
func (l Live) Alive() bool {
	return l.PID > 0 && syscall.Kill(l.PID, 0) == nil
}

// Read returns all parseable entries from a sessions registry directory
// (conventionally claudedir.Root.SessionsDir()). Entries without a
// sessionId and unreadable files are skipped. No liveness filtering is
// applied — see Live.Alive and Resolve.
func Read(dir string) []Live {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var entries []Live
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var e Live
		if json.Unmarshal(raw, &e) == nil && e.SessionID != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// ProcessTree maps every pid to its children. Build one with
// CurrentProcessTree (a single ps call) or construct it directly in tests.
type ProcessTree map[int][]int

// CurrentProcessTree snapshots the system process table via one ps call.
// Returns nil when ps fails.
func CurrentProcessTree() ProcessTree {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	children := ProcessTree{}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			children[ppid] = append(children[ppid], pid)
		}
	}
	return children
}

// DescendantsOf returns root and every process below it in the tree.
func (t ProcessTree) DescendantsOf(root int) map[int]bool {
	desc := map[int]bool{}
	if root <= 0 {
		return desc
	}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if desc[pid] {
			continue
		}
		desc[pid] = true
		queue = append(queue, t[pid]...)
	}
	return desc
}

// Resolve matches a pane's Claude session: a registry pid inside the pane's
// process tree is definitive; otherwise fall back to cwd equality,
// preferring the newest startedAt. Dead entries are ignored.
//
// paneTree is the descendant set of the pane's root pid (see
// ProcessTree.DescendantsOf); nil disables the pid match and relies on cwd.
func Resolve(entries []Live, paneTree map[int]bool, paneCwd string) (Live, bool) {
	var byCwd Live
	for _, e := range entries {
		if !e.Alive() {
			continue
		}
		if paneTree[e.PID] {
			return e, true
		}
		if e.Cwd == paneCwd && (byCwd.SessionID == "" || e.StartedAt > byCwd.StartedAt) {
			byCwd = e // first live match wins even with a zero startedAt
		}
	}
	return byCwd, byCwd.SessionID != ""
}

// ResolvePane is the one-call convenience: read the registry at dir, build
// the current process tree, and resolve the session for the pane rooted at
// panePID with working directory paneCwd.
func ResolvePane(dir string, panePID int, paneCwd string) (Live, bool) {
	tree := CurrentProcessTree()
	return Resolve(Read(dir), tree.DescendantsOf(panePID), paneCwd)
}
