// Package transcript is the lossless parsing pipeline for Claude Code
// session JSONL files:
//
//	JSONL bytes -> ParseEntry -> Classify -> BuildChunks -> []Chunk
//
// Each stage is a pure function; file IO happens only in ReadSession,
// ReadSessionIncremental, and the tail scans. The package depends on
// nothing outside the standard library and this module.
//
// # Tolerant-decoding rules
//
// Claude Code's on-disk schema grows continuously. These rules are what
// keep old and new files parsing; violating any of them is a regression:
//
//   - toolUseResult is object-OR-array: a JSON object for regular tools,
//     a JSON array for MCP tools. Entry stores it as json.RawMessage and
//     ToolUseResultMap returns nil for the array shape.
//   - usage.iterations recurses: message.usage may carry an iterations
//     array (one element per inference cycle when the server collapses
//     several into one assistant entry, Claude Code 2.1.19x+). EntryUsage
//     nests recursively; the live context window is the LAST iteration's
//     snapshot (EntryUsage.ContextUsage), while top-level counts are a
//     merge that overstates the window.
//   - ParseTimestamp is multi-format: RFC3339Nano, RFC3339, and the
//     timezone-less variant Claude Code sometimes emits. Unknown formats
//     yield the zero time, never an error.
//   - Field-name conventions never mix: native transcript fields are
//     camelCase (isSidechain, toolUseResult); hook and statusline payloads
//     are snake_case (session_id, tool_name). One struct, one convention.
//   - uuid-less session-metadata records (custom-title, ai-title,
//     last-prompt, permission-mode, ... — re-appended on flush/resume,
//     last occurrence wins) are rejected by strict ParseEntry so they
//     cannot become phantom conversation turns, and accepted by
//     ParseEntryLenient for consumers that want them.
//   - type=summary entries use leafUuid instead of uuid (pre-2.1.18x
//     compaction boundaries); type=system with subtype=compact_boundary
//     is the modern signal and surfaces as CompactMsg.
//   - Attachment entries are discriminated, not blanket-dropped: only
//     attachment.type=="nested_memory" surfaces (MemoryLoadMsg); every
//     other subtype — including future ones — drops silently.
//   - Empty thinking blocks are counted, not emitted: Opus 4.7+/Claude 5
//     transcripts persist thinking as {"thinking":"","signature":"..."};
//     ThinkingCount still increments so the turn shows work happened.
//   - Oversized lines are skipped, never fatal: the jsonl reader's 64 MiB
//     default cap accommodates inline images and giant pre-2.1.19x tool
//     results; one huge line must never hide the rest of the session.
//   - forkedFrom (2.1.201+) stamps EVERY entry of a forked session with
//     {sessionId, messageUuid} lineage — user, assistant, system, and
//     attachment types alike. Entry.ForkedFrom is nil on non-forks.
//
// # Classify is destructive — ExtractContentBlocks is not
//
// Classify and SanitizeContent strip XML tags, drop noise entry types,
// and discard sidechain entries. Data any consumer needs beyond rendering
// (tool counts, agent launches, session metadata, team summaries) must be
// extracted at the Entry layer BEFORE Classify runs — via
// ExtractContentBlocks (lossless, filter-free, raw inputs preserved) or a
// raw scan. Never regex chunk text for data Classify strips.
//
// # Two partial-last-line rules, on purpose
//
// A JSONL file being appended to can end mid-line, and this module ships
// two different rules for it. ReadSessionIncremental KEEPS an
// unterminated final line that parses as complete JSON (a resident
// watcher may never get another file event — tail-claude's rule). The
// offsetstore package ALWAYS defers the unterminated tail to the next
// tick (its consumer is a fresh process ~300ms later — tail-claude-hud's
// rule). Swapping either for the other breaks that consumer; both have
// regression tests naming their dependent.
//
// # Schema drift
//
// corpus_test.go carries a maintained allowlist of every observed
// top-level and message key. Set CLAUDE_CORPUS_DIR to a transcript corpus
// (or a live ~/.claude/projects) to run the gated drift detector: a new
// key fails the run and is extended into the allowlist deliberately. For
// fields the library does not model, the jsonl package hands callers raw
// lines, and hooks/statusline payloads preserve Raw.
package transcript
