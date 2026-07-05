package statusline

import (
	"strings"
	"testing"
)

const sample = `{
  "session_id": "abc",
  "transcript_path": "/home/u/.claude/projects/-p/abc.jsonl",
  "cwd": "/p",
  "version": "2.1.201",
  "model": {"id": "claude-opus-4-8", "display_name": "Opus 4.8"},
  "workspace": {"current_dir": "/p", "project_dir": "/p"},
  "context_window": {
    "context_window_size": 200000,
    "used_percentage": 42.5,
    "remaining_percentage": 57.5,
    "total_input_tokens": 900000,
    "total_output_tokens": 12000,
    "current_usage": {
      "input_tokens": 1200,
      "output_tokens": 300,
      "cache_creation_input_tokens": 500,
      "cache_read_input_tokens": 80000
    }
  },
  "cost": {"total_cost_usd": 1.25, "total_duration_ms": 60000, "total_api_duration_ms": 20000, "total_lines_added": 10, "total_lines_removed": 3},
  "exceeds_200k_tokens": false,
  "output_style": {"name": "bottom-line"},
  "vim": {"mode": "NORMAL"},
  "rate_limits": {"five_hour": {"used_percentage": 12.0, "resets_at": 1780000000.5}},
  "worktree": {"name": "wt", "path": "/p/.claude/worktrees/wt", "branch": "feat", "original_cwd": "/p", "original_branch": "main"},
  "effort": {"level": "high"},
  "unmodeled_future": {"x": 1}
}`

func TestDecode(t *testing.T) {
	p, err := Decode(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if p.SessionID != "abc" || p.Version != "2.1.201" {
		t.Errorf("top-level fields: %+v", p)
	}
	if p.Model == nil || p.Model.ID != "claude-opus-4-8" {
		t.Errorf("Model = %+v", p.Model)
	}
	cw := p.ContextWindow
	if cw == nil || cw.Size != 200000 || cw.UsedPercent == nil || *cw.UsedPercent != 42.5 {
		t.Fatalf("ContextWindow = %+v", cw)
	}
	if cw.CurrentUsage == nil || cw.CurrentUsage.CacheReadInputTokens != 80000 {
		t.Errorf("CurrentUsage = %+v", cw.CurrentUsage)
	}
	if cw.TotalInputTokens != 900000 {
		t.Errorf("TotalInputTokens = %d", cw.TotalInputTokens)
	}
	if p.Cost == nil || p.Cost.TotalCostUSD != 1.25 {
		t.Errorf("Cost = %+v", p.Cost)
	}
	if p.OutputStyle == nil || p.OutputStyle.Name != "bottom-line" {
		t.Errorf("OutputStyle = %+v", p.OutputStyle)
	}
	if p.Vim == nil || p.Vim.Mode != "NORMAL" {
		t.Errorf("Vim = %+v", p.Vim)
	}
	if p.RateLimits == nil || p.RateLimits.FiveHour == nil || *p.RateLimits.FiveHour.UsedPercent != 12.0 {
		t.Errorf("RateLimits = %+v", p.RateLimits)
	}
	if p.Worktree == nil || p.Worktree.Branch != "feat" {
		t.Errorf("Worktree = %+v", p.Worktree)
	}
	if p.Effort == nil || p.Effort.Level != "high" {
		t.Errorf("Effort = %+v", p.Effort)
	}
	if !strings.Contains(string(p.Raw), "unmodeled_future") {
		t.Error("Raw does not preserve unmodeled fields")
	}
}

func TestDecode_MinimalPayload(t *testing.T) {
	p, err := Decode(strings.NewReader(`{"session_id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != nil || p.Cost != nil || p.ContextWindow != nil || p.RateLimits != nil {
		t.Error("absent sections must decode to nil pointers")
	}
}
