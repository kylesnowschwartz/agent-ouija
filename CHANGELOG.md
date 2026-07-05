# Changelog

Versioning: v0.x — breaking changes bump the MINOR version and are
listed here. v1.0.0 comes only after all three consumers (tail-claude,
tail-claude-hud, gearshifter) have migrated and one real Anthropic
format-drift cycle has been absorbed without API breakage.

## v0.4.0 — 2026-07-05

Additive only.

- `registry.Live` gains `Agent` — the active subagent name while a
  Task tool call runs (the `agent` key in sessions/*.json). Consumer:
  tail-claude-mux Go backend (AgentEvent.subagent).

## v0.3.0 — 2026-07-05

First real Anthropic format-drift event, absorbed library-side with a
fixture (the v1.0.0 gate asks for a drift cycle absorbed *without API
breakage* — this one forced a field type change, so the gate stays
open).

- **Breaking**: `registry.Live.StartedAt` is now `registry.EpochMS`
  (int64 milliseconds) instead of `string`. Current Claude Code writes
  `startedAt` as an epoch-ms JSON number; the strict string decode
  failed every unmarshal and `registry.Read` returned zero entries on
  2.1.19x. `EpochMS` tolerates the number, numeric-string, and RFC3339
  encodings; unknown shapes decode to 0 rather than dropping the entry.
  No known consumer read the field directly.
- `registry.Live` gains the liveness map and identity fields consumers
  need: `Status` (busy/idle/waiting; may be absent for sdk-cli),
  `UpdatedAt`, `StatusUpdatedAt`, `Name`, `Kind`. First consumer:
  tail-claude-mux's Go backend (liveness probe, thread names).
- `settings.Introspect(path)` — State + MCP server names + non-empty
  hook count from a single read; the single-purpose readers remain.

## v0.2.0 — 2026-07-05

Additive only; no breaking changes.

- `claudedir.DefaultRoot` honors `CLAUDE_CONFIG_DIR` before falling back
  to `$HOME/.claude`, matching Claude Code's own config-root resolver
  (verified against the 2.1.201 bundle: projects/, sessions/, and
  settings.json all derive from it). Consumers reading through
  DefaultRoot now follow the override automatically.
- `Root.SessionTranscriptPath(cwd, sessionID)` — forward constructor for
  `{root}/projects/{encoded-cwd}/{id}.jsonl`; completes the (Cwd,
  SessionID) pair `registry.Resolve` returns.
- `claudedir.NewestTranscript(projectDir)` — most recently modified
  transcript in a project dir (tail-claude-hud's current-session
  lookup, moved library-side).

## v0.1.0 — 2026-07-05

Initial extraction. Bootstrapped from tail-claude@e71144c `parser/`
(plain copy, restructured into the two-tier package tree), with ports
from tail-claude-hud@f6959f1 (offsetstore, hooks, settings, statusline,
`agents.ScanSubagentMeta`, Entry Slug/CustomTitle superset) and
gearshifter@e718c8e (registry, `transcript.LastAssistantModel`).

Defects found and fixed during the consumer migrations, before this tag:

- `settings.RegisterHooks` atomic write now preserves the existing file
  mode (a hardened 0600 no longer widens to 0644), writes through
  symlinks instead of replacing them, and cleans its temp file on rename
  failure.
- `registry.Resolve` restores gearshifter's empty-`startedAt` guard: a
  lone live cwd match with no `startedAt` is selected again.
- `settings.State` gains `Style` (the `outputStyle` key).
- `transcript.LastAssistantModel` pinned to gearshifter's exact
  semantics: one Stat feeds both the scan window and the returned mtime,
  and lines decode through a minimal {type, message.model} struct so
  format drift in unrelated fields never drops the model.

- `sessions` core: `Provider`, `SessionRef`, `Query`, `Registry`,
  `LiveTracker` capability; `claude.Provider` adapter; `sessionstest`
  conformance suite.
- `claude/transcript`: lossless pipeline plus `ParseEntryLenient`,
  `ExtractContentBlocks`, `ScanTailEntries`, `LastAssistantModel`.
- `jsonl`: caller-supplied max line size (default 64 MiB, skip-oversized
  verbatim), `TailLines`, `ReverseScan`.
- `offsetstore`: caller-supplied schema version + opaque snapshot.
- `claude/settings.RegisterHooks`: exec-form `HookCommand`, dual-form
  idempotency, atomic writes.
