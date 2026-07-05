// Package statusline models the JSON document Claude Code pipes to a
// statusline command's stdin on every tick.
//
// Canonical reference: https://code.claude.com/docs/en/statusline#available-data
// Fields are snake_case (like hook payloads, unlike the camelCase native
// transcript fields — never mix the conventions in one struct). Pointer
// fields are nil when the running Claude Code version omits them; the raw
// document is preserved for anything unmodeled.
//
// Token semantics worth restating: CurrentUsage fields are per-call
// snapshots, not session totals. With prompt caching, InputTokens is only
// the uncacheable tail after the last cache breakpoint; used_percentage =
// (input + cache_creation + cache_read) / context_window_size. Cumulative
// session totals live in TotalInputTokens / TotalOutputTokens.
package statusline

import (
	"encoding/json"
	"io"
)

// Payload is the statusline stdin document.
type Payload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	Version        string `json:"version"`

	Model *Model `json:"model"`

	Workspace *Workspace `json:"workspace"`

	ContextWindow *ContextWindow `json:"context_window"`

	// Cost is nil when Claude Code does not include cost data.
	Cost *Cost `json:"cost"`

	// ExceedsTwoHundredK reports whether combined tokens from the last API
	// response exceed 200k.
	ExceedsTwoHundredK bool `json:"exceeds_200k_tokens"`

	// OutputStyle is nil when output_style is absent.
	OutputStyle *OutputStyle `json:"output_style"`

	// Vim is nil unless vim mode is enabled.
	Vim *Vim `json:"vim"`

	// Agent is nil unless running with --agent.
	Agent *Agent `json:"agent"`

	// RateLimits is nil on older Claude Code versions or for API users.
	RateLimits *RateLimits `json:"rate_limits"`

	// Worktree is nil when not running inside a worktree.
	Worktree *Worktree `json:"worktree"`

	// Effort is nil on versions that do not report reasoning effort;
	// callers typically fall back to the CLAUDE_EFFORT environment variable.
	Effort *Effort `json:"effort"`

	// Raw is the complete JSON document as received. Populated by Decode.
	Raw json.RawMessage `json:"-"`
}

// Model identifies the active model.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Workspace holds the directory context.
type Workspace struct {
	// CurrentDir mirrors the top-level cwd; prefer this field.
	CurrentDir string `json:"current_dir"`
	// ProjectDir is the directory where Claude Code was launched.
	ProjectDir string `json:"project_dir"`
}

// ContextWindow holds context usage as reported by Claude Code.
type ContextWindow struct {
	Size              int      `json:"context_window_size"`
	UsedPercent       *float64 `json:"used_percentage"`
	RemainingPercent  *float64 `json:"remaining_percentage"`
	TotalInputTokens  int      `json:"total_input_tokens"`  // cumulative across the session
	TotalOutputTokens int      `json:"total_output_tokens"` // cumulative across the session
	CurrentUsage      *Usage   `json:"current_usage"`       // most recent API call; nil until first call
}

// Usage holds token counts from the most recent API call.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// Cost holds session accounting.
type Cost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms"`
	TotalLinesAdded    int     `json:"total_lines_added"`
	TotalLinesRemoved  int     `json:"total_lines_removed"`
}

// OutputStyle names the active output style.
type OutputStyle struct {
	Name string `json:"name"`
}

// Vim reports the vim editing mode ("NORMAL"/"INSERT") when enabled.
type Vim struct {
	Mode string `json:"mode"`
}

// Agent names the agent when running with --agent.
type Agent struct {
	Name string `json:"name"`
}

// RateLimits holds rate-limit windows provided via stdin, avoiding the
// need for OAuth API calls.
type RateLimits struct {
	FiveHour *RateWindow `json:"five_hour"`
	SevenDay *RateWindow `json:"seven_day"`
}

// RateWindow is one rate-limit window.
type RateWindow struct {
	UsedPercent *float64 `json:"used_percentage"`
	// ResetsAt is a Unix epoch in seconds (fractional allowed).
	ResetsAt *float64 `json:"resets_at"`
}

// Worktree holds metadata about the current worktree.
type Worktree struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Branch         string `json:"branch"`
	OriginalCwd    string `json:"original_cwd"`
	OriginalBranch string `json:"original_branch"`
}

// Effort holds the current reasoning effort level.
type Effort struct {
	Level string `json:"level"`
}

// Decode reads a statusline payload from r (conventionally os.Stdin),
// preserving the raw bytes in Payload.Raw.
func Decode(r io.Reader) (*Payload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	p.Raw = data
	return &p, nil
}
