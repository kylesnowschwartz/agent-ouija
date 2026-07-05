// Package registry reads Claude Code's live-session process registry
// ({root}/sessions/*.json) and resolves which session belongs to a given
// terminal pane.
//
// Mechanism (verified against Claude Code 2.1.201): each running session
// writes a JSON file with its pid, sessionId, cwd, and startedAt. Files
// linger after exit, so entries must be liveness-checked before use.
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
)

// Live is one file of Claude Code's live-session registry. Only the fields
// consumers need are decoded.
type Live struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	StartedAt string `json:"startedAt"`
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
		if e.Cwd == paneCwd && e.StartedAt > byCwd.StartedAt {
			byCwd = e
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
