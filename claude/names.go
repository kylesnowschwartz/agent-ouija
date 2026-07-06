package claude

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kylesnowschwartz/agent-ouija/claude/claudedir"
	"github.com/kylesnowschwartz/agent-ouija/claude/discover"
	"github.com/kylesnowschwartz/agent-ouija/claude/registry"
	"github.com/kylesnowschwartz/agent-ouija/gitroot"
)

// Display-name arbitration between Claude Code's two title sources, best
// name first:
//
//	custom title > AI title > registry name > stamped title > ""
//
// The first two are resolved into discover.SessionInfo.Title (custom wins,
// last occurrence wins). The registry name (~/.claude/sessions/{pid}.json)
// fills two gaps the transcript can't (verified against Claude Code
// 2.1.195/2.1.196):
//
//   - Sessions launched through an agent launcher get a custom-title record
//     stamped with the project directory name on every transcript flush. A
//     /rename writes the new title once, then the next flush overwrites it,
//     so under last-occurrence-wins the stamp is what SessionInfo.Title
//     reports and the rename survives only in the registry. Claude Code
//     also skips ai-title generation when a custom title exists, so stamped
//     sessions have no AI-title fallback either.
//   - Auto-named sessions (registry "{dir}-{2hex}" names, 2.1.196+) carry
//     no transcript title records at all.
//
// A stored title that merely repeats the project directory name is that
// stamp, not a user choice — rank it below the registry name. Detection is
// exact-match against the cwd basename and the git main-worktree basename;
// launcher-composed titles that embed but don't equal the directory name
// are treated as genuine.

// NameResolver holds the winning registry name per session, ready to
// arbitrate against transcript titles. Build one with NewNameResolver;
// the zero value resolves every session to its transcript title.
type NameResolver struct {
	names map[string]string // sessionID → winning registry name
}

// NewNameResolver picks one registry name per session from raw registry
// entries. Files linger after exit and a resumed session appears under a
// new pid; when several entries claim the same session, newerEntry picks
// the winner.
//
// Lingering entries from exited sessions are wanted here — they are the
// only surviving record of a /rename once the transcript flush re-stamps
// the title (tail-claude's picker depends on this) — so entries are NOT
// filtered with Live.Alive.
func NewNameResolver(entries []registry.Live) NameResolver {
	winners := winnersBySession(entries)
	names := make(map[string]string, len(winners))
	for id, e := range winners {
		names[id] = e.Name
	}
	return NameResolver{names: names}
}

// winnersBySession picks one named entry per session from raw registry
// entries using the newerEntry tie-break. Nameless entries can never win
// a name arbitration and are dropped.
func winnersBySession(entries []registry.Live) map[string]registry.Live {
	winners := make(map[string]registry.Live, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		if cur, seen := winners[e.SessionID]; !seen || newerEntry(e, cur) {
			winners[e.SessionID] = e
		}
	}
	return winners
}

// DisplayName returns the best display name for a session. Genuine
// custom/AI titles stay untouched; only untitled sessions and directory-
// name stamps take the registry name. A stamped title with no registry
// alternative is returned as-is — a stamp still beats a blank line.
// Returns "" only when the session has no title and no registry name.
//
// Edge case, accepted: a user who genuinely renames a session to the
// project directory name is indistinguishable from the stamp, so a
// registry name (including an auto-generated one) replaces that title.
func (r NameResolver) DisplayName(info discover.SessionInfo) string {
	name := r.names[info.SessionID]
	if name == "" {
		return info.Title
	}
	if info.Title == "" || isStampedTitle(info.Title, info.Cwd) {
		return name
	}
	return info.Title
}

// Apply overlays resolved display names onto discovered sessions in
// place, rewriting each Title to DisplayName's answer. Single entry point
// for slice consumers (pickers, session lists) so initial loads and
// watcher rescans cannot diverge.
func (r NameResolver) Apply(sessions []discover.SessionInfo) {
	if len(r.names) == 0 {
		return
	}
	for i := range sessions {
		sessions[i].Title = r.DisplayName(sessions[i])
	}
}

// newerEntry orders same-session registry entries: freshest UpdatedAt
// wins, then StartedAt, then PID — deterministic even when format drift
// decodes the timestamps to zero.
func newerEntry(a, b registry.Live) bool {
	if a.UpdatedAt != b.UpdatedAt {
		return a.UpdatedAt > b.UpdatedAt
	}
	if a.StartedAt != b.StartedAt {
		return a.StartedAt > b.StartedAt
	}
	return a.PID > b.PID
}

// isStampedTitle reports whether a stored title merely repeats the
// session's directory name — the launcher stamp, not a user choice. The
// stamp carries the project's name while the session may run in a
// subdirectory or worktree, so the git main-worktree root's name counts
// too (one .git lookup, only on the rare non-matching path).
func isStampedTitle(title, cwd string) bool {
	if cwd == "" {
		return false
	}
	return title == filepath.Base(cwd) || title == filepath.Base(gitroot.ResolveGitRoot(cwd))
}

// FindNameMatches resolves a name query against registry session names,
// mirroring discover.FindTitleMatches semantics: case-insensitive, exact
// matches beat substring matches, newest-first within a tier. Sessions
// whose transcript file does not exist (yet) are skipped — there is
// nothing to open. Duplicate same-session entries are resolved with the
// NewNameResolver tie-break before matching, so a stale pre-rename name
// cannot match.
//
// Registry names are what pickers display for stamped and auto-named
// sessions, so they must resolve as name arguments too. Combine with
// discover.FindTitleMatches via discover.MergeTitleRefs, then restore the
// cross-source exact-beats-substring rule with discover.PreferExact.
func FindNameMatches(query string, root claudedir.Root, entries []registry.Live) []discover.SessionTitleRef {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lower := strings.ToLower(query)
	var exact, partial []discover.SessionTitleRef
	for _, e := range winnersBySession(entries) {
		if e.Cwd == "" {
			continue // no cwd, no transcript path to offer
		}
		t := strings.ToLower(e.Name)
		isExact := t == lower
		if !isExact && !strings.Contains(t, lower) {
			continue
		}
		path := root.SessionTranscriptPath(e.Cwd, e.SessionID)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		ref := discover.SessionTitleRef{Path: path, SessionID: e.SessionID, Title: e.Name, ModTime: info.ModTime()}
		if isExact {
			exact = append(exact, ref)
		} else {
			partial = append(partial, ref)
		}
	}
	refs := partial
	if len(exact) > 0 {
		refs = exact
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].ModTime.After(refs[j].ModTime)
	})
	return refs
}
