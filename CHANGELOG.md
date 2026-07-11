# Changelog

Versioning: semver, module-aware. From v1.0.0 the API is stable —
breaking changes require a /v2 module path. The v1 gate (all consumers
migrated + one real Anthropic format-drift cycle absorbed without API
breakage) was satisfied 2026-07-05.

## v1.6.0 — 2026-07-11

Additive only.

- `codex/rollout`: `Claim`, `ClaimForHookEvent`, `NeedsEvidence`,
  `Verdict`, and `Reconcile` reconcile caller-owned Codex lifecycle-hook
  observations with rollout evidence. Terminal rollout states override a
  `Stop` claim, while an `auto_review` turn context keeps an approval claim
  running instead of waiting. `TrailingState` now includes the newest turn
  context's `ApprovalsReviewer` value without another rollout read.

## v1.5.0 — 2026-07-11

Additive only.

- `codex/rollout`: `FinalMessage` and `FinalAssistantMessage` return the
  newest final-phase assistant output from either Codex message encoding,
  with its timestamp and source entry.
- `codex/rollout`: `Snapshot` and `SessionSnapshot` combine session metadata,
  trailing state, and final assistant output for one rollout path.

## v1.4.0 — 2026-07-11

Additive only. Closes the three shims tail-claude-mux's codexwatch kept
after adopting v1.3.0 (each marked "TODO: move into agent-ouija").

- `codex/rollout`: `Payload.Source` models session_meta's polymorphic
  "source" field — string form in `Source.Kind` ("cli", "exec",
  "vscode", "mcp" observed live), object form (subagent/derived
  sessions) preserved verbatim in `Source.Raw` (a string, not
  json.RawMessage, so `Entry` stays comparable). `Payload.Cwd` is now
  documented as also set on session_meta.
- `codex/rollout.SessionMeta` — reads a rollout stream's first
  parseable entry and returns it if it is the session_meta header;
  stops at that entry rather than reading the whole file. Verified
  against all 493 live rollouts on disk (header on line 1 in every one).
- `codex/discover.SessionIDFromPath` — extracts a trailing lowercase
  UUID from an extension-stripped base name for a single already-known
  rollout path (e.g. from lsof), using the same rule as
  `DiscoverRollouts`.

## v1.3.0 — 2026-07-11

Additive only.

- `codex/` subtree — Codex CLI on-disk state parsing, mirroring
  `claude/`'s role, so consumers (tail-claude-mux's codexwatch,
  tail-claude, tail-claude-hud) stop reimplementing it:
  - `codex/rollout`: `Entry`/`Payload` + lenient `ParseEntry`;
    `TrailingState` folds a rollout stream into a `{Status, Cwd}`
    snapshot using an Idle/Running/Done/Interrupted/Error vocabulary
    owned by this package (no dependency on any consumer's wire
    format). `Ongoing` is gated on rollout file mtime
    (`OngoingStalenessThreshold`, 2 minutes) so a killed or crashed
    Codex, which never appends a terminal event, doesn't read Running
    forever.
  - `codex/codexdir`: `Root` + `DefaultRoot` (`$CODEX_HOME` /
    `$HOME/.codex`), `SessionsDir`, `SessionIndexPath` — mirrors
    `claude/claudedir`.
  - `codex/discover`: `DiscoverRollouts` (nested YYYY/MM/DD walk,
    trailing-UUID filename extraction) and `ThreadNames`
    (`session_index.jsonl` reader, last-entry-per-id wins).
  - `codex.Provider` implementing `sessions.Provider`, passing the
    same sessionstest conformance suite as `claude.Provider`.
    Deliberately no LiveTracker — Codex CLI keeps no on-disk
    live-process registry to back it.

## v1.2.0 — 2026-07-06

Additive only.

- `claude.NameResolver` (`NewNameResolver`, `DisplayName`, `Apply`) —
  display-name arbitration between the transcript title and the
  live-session registry name: custom title > AI title > registry name >
  stamped title > "". Why: launcher-started sessions get a custom-title
  record re-stamped with the project directory name on every transcript
  flush (verified against Claude Code 2.1.195), so under
  last-occurrence-wins a /rename survives only in the registry; and
  auto-named sessions (registry `{dir}-{2hex}`, 2.1.196+) carry no
  transcript title records at all. Entries are deliberately NOT
  liveness-filtered — a lingering registry file is the only surviving
  record of a /rename (regression-tested). Fixture:
  `claude/testdata/restamped_title.jsonl`. Ported from tail-claude's
  `registry_names.go`. Issue #1.
- `claude.FindNameMatches(query, root, entries)` — the registry-name
  counterpart to `discover.FindTitleMatches` (case-insensitive, exact
  beats substring, newest-first, missing transcripts skipped, stale
  duplicate entries resolved before matching).
- `discover.MergeTitleRefs` / `discover.PreferExact` — combine match
  results across the two sources and restore exact-beats-substring
  across the merge.

## v1.1.0 — 2026-07-05

Additive only.

- `transcript.LastAssistantModelAt(path)` — LastAssistantModel
  arbitrating by ENTRY time: returns the matched assistant entry's own
  timestamp (file-mtime fallback when the entry has none).
  `LastAssistantModel`'s pinned file-mtime contract is unchanged. Why:
  a transcript's file mtime moves on every appended entry — user
  prompts, local slash commands — so "transcript newer than
  settings.json" does not mean the model fact is newer; a live
  session's /model change was losing the freshness arbitration until
  the next assistant reply (found live in gearshifter's strip,
  2026-07-05). Consumer: gearshifter session-state arbitration.

## v1.0.0 — 2026-07-05

The API as of v0.4.2, frozen. No code changes.

Gate evidence: four consumers live on the module (tail-claude v0.16.0,
tail-claude-hud v0.7.0, gearshifter, tail-claude-mux's Go backend);
two real Anthropic drift events absorbed — startedAt string→epoch-ms
(v0.3.0, broke Live.StartedAt, gate stayed open) and forkedFrom
lineage (v0.4.2, additive, gate closed).

## v0.4.2 — 2026-07-05

Additive only — the second real Anthropic format-drift event, absorbed
without API breakage. This satisfies the v1.0.0 gate's drift criterion.

- `transcript.Entry` gains `ForkedFrom *ForkedFrom` ({sessionId,
  messageUuid}): Claude Code 2.1.201 stamps every entry of a forked
  session with fork lineage, on user/assistant/system/attachment types
  alike. Nil on non-forks. Found by the gated corpus run; fixture in
  format_drift_test.go; corpus allowlist extended.

## v0.4.1 — 2026-07-05

Additive only.

- `settings.HookCommand` gains `Async` — writes Claude Code's
  fire-and-forget hook flag (`"async": true`) in the registered entry.
  Identity matching is unchanged: async does not distinguish otherwise
  equal commands. Consumer: tail-claude-mux `tcm-server -register-hooks`
  (its hooks must never block the agent).

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
