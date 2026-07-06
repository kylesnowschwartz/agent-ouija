# agent-ouija

A standalone, stdlib-only Go library for reading AI-agent session state
off disk. Claude Code is the first (and currently only) provider; the
thin provider-neutral core exists so future providers (codex, pi, ...)
slot in without forcing Claude's fidelity through a lossy interface.

Consumers: tail-claude (TUI transcript reader), tail-claude-hud
(statusline), gearshifter (tmux control deck).

## Architecture: two tiers

**Tier 1 — neutral core** (root package `sessions`): `SessionRef`,
`Query`, `Provider`, `Registry`, and optional capability interfaces
discovered by type assertion (`LiveTracker`). Deliberately thin; it is
NOT a funnel for rich data.

**Tier 2 — lossless claude subtree** (`claude/...`): ordinary packages
that claude-specific consumers import directly. The `Entry → Classify →
BuildChunks` pipeline is never squeezed through an interface.

## Package map

| Package | Responsibility |
|---|---|
| `sessions` (root) | Provider-neutral core + capability interfaces |
| `sessionstest` | FakeProvider + provider conformance suite (`Run`) |
| `jsonl` | Neutral line IO: skip-oversized reader (64 MiB default, caller-supplied via `NewReaderSize`), offset tracking, `TailLines`/`ReverseScan` |
| `offsetstore` | Disk-persisted incremental-read offsets + opaque versioned snapshot (HUD's tick model) |
| `gitroot` | Git main-worktree root resolution without the git binary |
| `internal/pat` | Shared regexes/tags (teammate XML, command tags, system-output tags) |
| `claude` | `claude.Provider` adapter implementing the core interfaces; the ONE bridge across claude subpackages: `NameResolver`/`FindNameMatches` display-name arbitration (transcript title vs registry name) |
| `claude/claudedir` | `Root` type: path encoding (`EncodeProjectPath`), `ProjectDirFor`, `ListProjectDirs`, `SettingsPath`, `SessionsDir`, `DebugLogPath` |
| `claude/transcript` | Lossless pipeline: `Entry`/`ParseEntry`/`ParseEntryLenient`, `Classify`, `BuildChunks`, `ExtractContentBlocks`, `ReadSession(Incremental)`, tail scans (`LastAssistantModel`), ongoing heuristics. Invariants: see `doc.go` |
| `claude/discover` | Session discovery: `SessionInfo`, `DiscoverProjectSessions`, titles, cache, date grouping, `ProjectName` |
| `claude/agents` | Subagent/team/workflow discovery + 4-phase linking (`DiscoverSubagents`, `DiscoverTeamSessions`, `LinkSubagents`, `ReconstructTeams`) PLUS `ScanSubagentMeta` — the metadata-only scan for sub-second tick consumers. Never call the full-parse variant on a tick |
| `claude/tools` | Per-tool one-line summaries, taxonomy, truncation helpers |
| `claude/debuglog` | `~/.claude/debug/*.txt` parsing + incremental reads |
| `claude/hooks` | Hook stdin `Payload` (+ output shapes), `Raw` preserved |
| `claude/settings` | `Read` (model/effortLevel ONLY — file holds secrets, never log raw), MCP/hook introspection, `RegisterHooks` (exec-form, dual-form idempotent, atomic write) |
| `claude/registry` | Live-session process registry + pane→session `Resolve` (injectable `ProcessTree`) |
| `claude/statusline` | Full documented statusline stdin schema, `Raw` preserved |

## Hard rules

- **Zero external dependencies.** Stdlib only, enforced in CI. No
  exceptions — this is the whole point of the library.
- **`go 1.25.5` directive, pinned.** tail-claude and tail-claude-hud
  build at 1.25.5; do not bump without checking all three consumers.
- **Injected roots everywhere.** No `os.UserHomeDir()`/`os.Getwd()`
  inside library logic; paths flow from `claudedir.Root`
  (`claudedir.DefaultRoot()` is the one sanctioned convenience).
- **Two partial-last-line rules, on purpose.**
  `transcript.ReadSessionIncremental` keeps a parseable unterminated
  tail (tail-claude: resident watcher). `offsetstore` always defers it
  (HUD: fresh process per tick). Each has a regression test naming its
  dependent consumer. Never "unify" them.
- **`ClassifiedMsg` stays sealed.** The extension seam is the core
  `Provider`, not the message union; sealing preserves exhaustive
  type-switch safety in consumers.
- **camelCase vs snake_case never mix in one struct.** Native transcript
  fields are camelCase; hook/statusline payloads are snake_case.
- **Field docs live in `claude/transcript/doc.go`** — the codified
  tolerant-decoding rules. Read it before touching Entry or Classify.

## Consumer contract (the line)

The dependency is strictly one-way: this library reads Claude Code's
on-disk state and returns data; **consumers decide what the data means**.

- **Never model a consumer here.** No imports of consumer repos, no types
  or fields that exist only to serve one tool's presentation. Naming a
  consumer in a doc comment to explain a contract (see the
  partial-last-line rules) is fine; encoding its policy is not.
- **Policy that stays app-side, by prior decision** — reject upstreaming
  attempts: tail-claude's rendering/watchers; the HUD's color assignment,
  status/duration heuristics, display-name *presentation* (labels,
  truncation, coloring), `ContextPercent`, and snapshot `SchemaVersion`;
  gearshifter's settings-vs-transcript mtime arbitration. Display-name
  *source arbitration* (transcript title vs registry name, the flush
  re-stamp) is library-side: `claude.NameResolver` (issue #1).
- **What belongs here**: anything derived purely from Claude Code's format
  or filesystem layout. Format drift is ALWAYS fixed here with a fixture,
  never patched in a consumer.
- **Releases**: semver tags; consumers never pin `@main` outside an
  active migration. v1.0.0 shipped 2026-07-05 — the API is stable, and
  a breaking change now requires a /v2 module path. Batch bumps — no
  per-commit churn across consumer repos (Dependabot in each GitHub
  consumer announces new tags).

## Schema drift ("up to date in one place")

- `format_drift_test.go` + fixtures run in CI.
- `corpus_test.go` is gated on `CLAUDE_CORPUS_DIR`: point it at a
  transcript corpus or a live `~/.claude/projects`; every line must
  parse and every key must be in the maintained allowlists. New keys
  fail the gated run — extend the allowlist deliberately and decide
  whether the library must model the field.
- Escape hatches for unmodeled data: `jsonl` hands back raw lines;
  `hooks.Payload.Raw` / `statusline.Payload.Raw` keep the full document.

## Development

```sh
go build ./... && go vet ./...   # check
go test ./...                    # test
go test -race ./...              # race
gofmt -l .                       # must print nothing
go test -bench . -benchmem ./claude/transcript/ ./offsetstore/   # perf gates
CLAUDE_CORPUS_DIR=~/.claude/projects go test ./claude/transcript/ -run TestCorpus -v
```

Versioning: semver, stable since v1.0.0 (2026-07-05; gate evidence in
CHANGELOG.md). Additions bump the minor; a breaking change requires a
/v2 module path — prefer absorbing drift additively, as forkedFrom was.

## Provenance

Bootstrapped from tail-claude@e71144c `parser/` (plain copy, no history),
plus ports from tail-claude-hud@f6959f1 (offsetstore, hooks, settings,
statusline, ScanSubagentMeta) and gearshifter@e718c8e (registry,
LastAssistantModel contract). Attribution for originally-ported parsing
logic: see tail-claude's ATTRIBUTION.md lineage (claude-devtools,
agentsview).
