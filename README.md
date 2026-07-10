# agent-ouija

Read AI-agent session state off disk. A standalone, dependency-free Go
library that knows where Claude Code keeps everything — transcripts,
subagents, teams, debug logs, hook payloads, settings, the live-session
registry, the statusline stdin schema — and parses all of it losslessly.

Every Claude Code format change lands here once, instead of once per
tool.

```
go get github.com/kylesnowschwartz/agent-ouija
```

## Quick start

```go
import (
    "github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
    "github.com/kylesnowschwartz/agent-ouija/claude/discover"
    "github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

root, _ := claudedir.DefaultRoot()                     // ~/.claude

// Which sessions exist for this project?
sessions, _ := discover.DiscoverProjectSessions(root.ProjectDirFor("/path/to/project"))

// Read one, fully processed for display.
chunks, _ := transcript.ReadSession(sessions[0].Path)

// Or tail it live: incremental reads + chunk rebuilds.
msgs, offset, _ := transcript.ReadSessionIncremental(sessions[0].Path, 0)
_ = transcript.BuildChunks(msgs)
_ = offset // resume from here on the next file event
```

Everything returns plain Go structs. JSON exists only at the boundaries
the library reads.

## Two tiers

**Provider-neutral core** (package `sessions`, module root) — for
consumers that enumerate sessions across agent products: `Provider`,
`SessionRef`, `Query`, `Registry`, and optional capabilities probed by
type assertion (`LiveTracker`). Claude Code and Codex CLI are its two
providers today; more slot in without forcing either one's fidelity
through the neutral interface.

**Lossless Claude subtree** (`claude/...`) — for consumers that know
they're reading Claude Code state and want full fidelity. The parsing
pipeline is never squeezed through an interface.

| You need | Import |
|---|---|
| Parse transcripts (entries → chunks) | `claude/transcript` |
| Find sessions, titles, previews | `claude/discover` |
| Subagents, teams, workflows | `claude/agents` |
| `~/.claude` path conventions | `claude/claudedir` |
| Hook stdin payloads | `claude/hooks` |
| settings.json (read + hook registration) | `claude/settings` |
| Live pane → session resolution | `claude/registry` |
| Statusline stdin schema | `claude/statusline` |
| Debug logs | `claude/debuglog` |
| Incremental offsets across process restarts | `offsetstore` |
| Raw JSONL primitives, bounded tail reads | `jsonl` |
| Git main-worktree resolution | `gitroot` |

**Codex subtree** (`codex/...`) — the Codex CLI counterpart, for
consumers reading `$CODEX_HOME` rollout transcripts.

| You need | Import |
|---|---|
| Rollout entries + trailing status/cwd fold | `codex/rollout` |
| Find rollout files, resolve thread names | `codex/discover` |
| `$CODEX_HOME` path conventions | `codex/codexdir` |
| `sessions.Provider` adapter | `codex` |

## Design commitments

- **Stdlib only.** No dependencies, ever. Enforced in CI.
- **Injected roots.** Nothing calls `os.UserHomeDir()` behind your back.
- **Lossless by default, tolerant by design.** Unknown entry types,
  block types, and attachment subtypes degrade silently; raw bytes stay
  reachable (`jsonl`, `Payload.Raw`) so new fields are readable without
  a release.
- **Drift alarm.** A gated corpus test (`CLAUDE_CORPUS_DIR`) asserts
  every line of a real transcript corpus parses and every key is known.
- **Two partial-write rules, both pinned.** Resident watchers keep a
  parseable unterminated tail; per-tick processes defer it. Regression
  tests name the consumer each rule protects.

Used by [tail-claude](https://github.com/kylesnowschwartz/tail-claude),
[tail-claude-hud](https://github.com/kylesnowschwartz/tail-claude-hud),
and gearshifter.

## Status

v0.x: the API moves when the consumers need it to; breaking changes bump
the minor version and are listed in [CHANGELOG.md](CHANGELOG.md).

## License

MIT — see [LICENSE](LICENSE). Parsing-logic lineage: tail-claude, which
ports ideas from claude-devtools and agentsview (see that repo's
ATTRIBUTION.md).
