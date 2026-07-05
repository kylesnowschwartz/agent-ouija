package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SubagentMeta is a lightweight subagent record built from file metadata
// alone: directory listing, the JSONL's first line, and the .meta.json
// sidecar. No chunk pipeline, no full parse.
type SubagentMeta struct {
	ID          string    // hex UUID from the agent-{id}.jsonl filename
	AgentType   string    // from the .meta.json sidecar ("Explore", "rb-worker", ...)
	Description string    // from the .meta.json sidecar (human task name)
	FirstTime   time.Time // first entry's timestamp; zero when unparseable
	ModTime     time.Time // transcript file mtime
	Size        int64     // transcript file size in bytes
}

// ScanSubagentMeta scans {session}/subagents/ for agent transcripts and
// returns metadata-only records. Warmup agents, compact agents
// ("acompact*"), and empty files are filtered — matching DiscoverSubagents'
// filtering — but nothing is parsed beyond each file's first line.
//
// This is a DIFFERENT algorithm from DiscoverSubagents, on purpose: the
// full-parse variant runs every subagent JSONL through the chunk pipeline,
// which is far too slow for callers on a sub-second tick (tail-claude-hud
// invokes this every ~300ms). Status/duration heuristics built on ModTime
// stay caller-side.
//
// Ported from tail-claude-hud@f6959f1 internal/gather/gather.go
// discoverSubagents, with the display-name and status policy removed.
func ScanSubagentMeta(sessionPath string) []SubagentMeta {
	dir := filepath.Dir(sessionPath)
	base := strings.TrimSuffix(filepath.Base(sessionPath), ".jsonl")
	subagentsDir := filepath.Join(dir, base, "subagents")

	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		return nil
	}

	var agents []SubagentMeta
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		agentID := strings.TrimPrefix(name, "agent-")
		agentID = strings.TrimSuffix(agentID, ".jsonl")

		// Filter compact agents (context compaction artifacts).
		if strings.HasPrefix(agentID, "acompact") {
			continue
		}

		info, err := de.Info()
		if err != nil || info.Size() == 0 {
			continue
		}

		// Filter warmup agents and parse the first-entry timestamp.
		first := scanFirstEntry(filepath.Join(subagentsDir, name))
		if first.isWarmup {
			continue
		}

		meta := readSidecarMeta(filepath.Join(subagentsDir, "agent-"+agentID+".meta.json"))

		agents = append(agents, SubagentMeta{
			ID:          agentID,
			AgentType:   meta.agentType,
			Description: meta.description,
			FirstTime:   first.timestamp,
			ModTime:     info.ModTime(),
			Size:        info.Size(),
		})
	}

	return agents
}

// firstEntryInfo holds the parsed results from a subagent JSONL's first line.
type firstEntryInfo struct {
	isWarmup  bool
	timestamp time.Time
}

// scanFirstEntry reads only the first line of a subagent JSONL file and
// returns the warmup status and timestamp. Returns a zero-value
// firstEntryInfo on any read or parse error.
func scanFirstEntry(path string) firstEntryInfo {
	line := readFirstLine(path)
	if line == nil {
		return firstEntryInfo{}
	}

	var entry struct {
		Timestamp string `json:"timestamp"`
		Message   struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &entry); err != nil {
		return firstEntryInfo{}
	}

	var content string
	isWarmup := false
	if err := json.Unmarshal(entry.Message.Content, &content); err == nil {
		isWarmup = content == "Warmup"
	}

	ts, _ := time.Parse(time.RFC3339, entry.Timestamp)
	return firstEntryInfo{
		isWarmup:  isWarmup,
		timestamp: ts,
	}
}

// sidecarMeta holds the parsed fields from a .meta.json sidecar file.
type sidecarMeta struct {
	agentType   string
	description string
}

// readSidecarMeta reads a .meta.json sidecar file. Returns a zero
// sidecarMeta when the file is missing, empty, or unparseable.
func readSidecarMeta(path string) sidecarMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return sidecarMeta{}
	}
	var meta struct {
		AgentType   string `json:"agentType"`
		Description string `json:"description"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return sidecarMeta{}
	}
	return sidecarMeta{agentType: meta.AgentType, description: meta.Description}
}

// readFirstLine opens a file and returns its first newline-terminated line.
// Returns nil when the file is missing, empty, or unreadable.
func readFirstLine(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	if n == 0 {
		return nil
	}

	line := buf[:n]
	for i, b := range line {
		if b == '\n' {
			line = line[:i]
			break
		}
	}
	return line
}
