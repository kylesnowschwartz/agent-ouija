package transcript_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
	"github.com/kylesnowschwartz/agent-ouija/jsonl"
)

// knownTopLevelKeys is the maintained allowlist of top-level JSONL entry
// keys. Seeded empirically from a 1736-file live corpus (Claude Code
// through 2.1.2xx, 2026-07). A NEW key appearing here is exactly the
// schema-drift signal this test exists to catch: decide whether the
// library must model it, then add it deliberately.
var knownTopLevelKeys = map[string]bool{
	"advisorModel": true, "agentColor": true, "agentId": true,
	"agentName": true, "agentSetting": true, "aiTitle": true,
	"apiErrorStatus": true, "apiRefusalCategory": true,
	"apiRefusalExplanation": true, "attachment": true,
	"attributionAgent": true, "attributionMcpServer": true,
	"attributionMcpTool": true, "attributionPlugin": true,
	"attributionSkill": true, "compactMetadata": true, "content": true,
	"contextLength": true, "customTitle": true, "cwd": true,
	"direction": true, "durationMs": true, "entrypoint": true,
	"error": true, "fallbackModel": true, "forkedFrom": true,
	"gitBranch": true,
	"hasOutput": true, "hookAdditionalContext": true, "hookCount": true,
	"hookErrors": true, "hookInfos": true, "imagePasteIds": true,
	"interruptedMessageId": true, "isApiErrorMessage": true,
	"isCompactSummary": true, "isMeta": true, "isSidechain": true,
	"isSnapshotUpdate": true, "isVisibleInTranscriptOnly": true,
	"key": true, "lastPrompt": true, "leafUuid": true, "level": true,
	"logicalParentUuid": true, "maxRetries": true, "mcpMeta": true,
	"message": true, "messageCount": true, "messageId": true,
	"mode": true, "operation": true, "origin": true,
	"originalModel": true, "parentLastUuid": true,
	"parentSessionId": true, "parentUuid": true,
	"pendingBackgroundAgentCount": true, "pendingWorkflowCount": true,
	"permissionMode": true, "planContent": true, "prNumber": true,
	"prRepository": true, "prUrl": true, "preventedContinuation": true,
	"promptId": true, "promptSource": true, "queuePriority": true,
	"relocatedCwd": true, "requestId": true, "result": true,
	"retryAttempt": true, "retryInMs": true, "sessionId": true,
	"sessionKind": true, "slug": true, "snapshot": true,
	"sourceToolAssistantUUID": true, "sourceToolUseID": true,
	"stopReason": true, "subtype": true, "timestamp": true,
	"toolDenialKind": true, "toolEndsTurn": true, "toolUseID": true,
	"toolUseResult": true, "trigger": true, "type": true,
	"userType": true, "uuid": true, "version": true,
	"worktreeSession": true,
}

// knownMessageKeys is the allowlist for keys inside the nested message
// object.
var knownMessageKeys = map[string]bool{
	"container": true, "content": true, "context_management": true,
	"diagnostics": true, "id": true, "model": true, "role": true,
	"stop_details": true, "stop_reason": true, "stop_sequence": true,
	"type": true, "usage": true,
}

// TestCorpus is the gated schema-drift detector. Point CLAUDE_CORPUS_DIR
// at a directory of session JSONL files (the private sanitized corpus, or
// a live ~/.claude/projects) and every line must:
//
//  1. be valid JSON that ParseEntryLenient accepts, and
//  2. contain no top-level or message-level keys outside the allowlists.
//
// Unset, the test skips — CI stays green without the corpus; the gated
// run is the drift alarm.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("CLAUDE_CORPUS_DIR")
	if dir == "" {
		t.Skip("CLAUDE_CORPUS_DIR not set; skipping gated corpus run")
	}

	unknownTop := map[string]string{} // key -> first file seen
	unknownMsg := map[string]string{}
	var files, lines int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		files++
		f, err := os.Open(path)
		if err != nil {
			t.Errorf("open %s: %v", path, err)
			return nil
		}
		defer f.Close()

		lineNo := 0
		scanErr := jsonl.ScanLines(f, func(line string) bool {
			lineNo++
			lines++
			if _, ok := transcript.ParseEntryLenient([]byte(line)); !ok {
				t.Errorf("%s:%d: line does not parse", path, lineNo)
				return true
			}
			var obj map[string]json.RawMessage
			if json.Unmarshal([]byte(line), &obj) != nil {
				return true // non-object JSONL line (never observed; parse already flagged)
			}
			for k := range obj {
				if !knownTopLevelKeys[k] {
					if _, seen := unknownTop[k]; !seen {
						unknownTop[k] = path
					}
				}
			}
			if raw, ok := obj["message"]; ok {
				var m map[string]json.RawMessage
				if json.Unmarshal(raw, &m) == nil {
					for k := range m {
						if !knownMessageKeys[k] {
							if _, seen := unknownMsg[k]; !seen {
								unknownMsg[k] = path
							}
						}
					}
				}
			}
			return true
		})
		if scanErr != nil {
			t.Errorf("scan %s: %v", path, scanErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	report := func(label string, unknown map[string]string) {
		if len(unknown) == 0 {
			return
		}
		keys := make([]string, 0, len(unknown))
		for k := range unknown {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Errorf("schema drift: unknown %s key %q (first seen in %s) — decide whether to model it, then extend the allowlist", label, k, unknown[k])
		}
	}
	report("top-level", unknownTop)
	report("message", unknownMsg)

	t.Logf("corpus: %d files, %d lines scanned", files, lines)
}
