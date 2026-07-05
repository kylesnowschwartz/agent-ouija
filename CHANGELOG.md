# Changelog

Versioning: v0.x — breaking changes bump the MINOR version and are
listed here. v1.0.0 comes only after all three consumers (tail-claude,
tail-claude-hud, gearshifter) have migrated and one real Anthropic
format-drift cycle has been absorbed without API breakage.

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
