package transcript_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kylesnowschwartz/agent-ouija/claude/transcript"
)

func TestIsOngoing_EmptyChunks(t *testing.T) {
	if transcript.IsOngoing(nil) {
		t.Error("empty chunks should not be ongoing")
	}
}

func TestIsOngoing_LastItemIsTextOutput(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "thinking", Text: "Let me think..."},
				{Type: "text", Text: "Here is the answer."},
			},
		},
	})
	if transcript.IsOngoing(chunks) {
		t.Error("session ending with text output should not be ongoing")
	}
}

func TestIsOngoing_LastItemIsToolUseNoResult(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "Read"}},
			Blocks: []transcript.ContentBlock{
				{Type: "tool_use", ToolID: "c1", ToolName: "Read", ToolInput: json.RawMessage(`{"file_path":"x.go"}`)},
			},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("tool_use with no result should be ongoing")
	}
}

func TestIsOngoing_LastItemIsThinking(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "thinking", Text: "Hmm, let me consider..."},
			},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("thinking with no following output should be ongoing")
	}
}

func TestIsOngoing_ToolCallAfterTextOutput(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "Bash"}},
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Let me check that."},
				{Type: "tool_use", ToolID: "c1", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`)},
			},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("tool_use after text output should be ongoing (activity after ending event)")
	}
}

func TestIsOngoing_ExitPlanModeIsEndingEvent(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "ExitPlanMode"}},
			Blocks: []transcript.ContentBlock{
				{Type: "thinking", Text: "Planning..."},
				{Type: "tool_use", ToolID: "c1", ToolName: "ExitPlanMode", ToolInput: json.RawMessage(`{}`)},
			},
		},
	})
	if transcript.IsOngoing(chunks) {
		t.Error("ExitPlanMode should be an ending event")
	}
}

func TestIsOngoing_ToolUseWithResultThenTextIsComplete(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Second)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "Read"}},
			Blocks: []transcript.ContentBlock{
				{Type: "tool_use", ToolID: "c1", ToolName: "Read", ToolInput: json.RawMessage(`{"file_path":"x.go"}`)},
			},
		},
		transcript.AIMsg{
			Timestamp: t1,
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "c1", Content: "file contents"},
			},
		},
		transcript.AIMsg{
			Timestamp: t1.Add(1 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "The file looks good."},
			},
		},
	})
	if transcript.IsOngoing(chunks) {
		t.Error("tool_use with result followed by text output should not be ongoing")
	}
}

func TestIsOngoing_ShutdownResponseIsEndingEvent(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	shutdownInput := json.RawMessage(`{"type":"shutdown_response","approve":true,"request_id":"abc"}`)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "SendMessage"}},
			Blocks: []transcript.ContentBlock{
				{Type: "thinking", Text: "Shutting down..."},
				{Type: "tool_use", ToolID: "c1", ToolName: "SendMessage", ToolInput: shutdownInput},
			},
		},
	})
	if transcript.IsOngoing(chunks) {
		t.Error("SendMessage shutdown_response with approve:true should be an ending event")
	}
}

func TestIsOngoing_Fallback_LastAIChunkEndTurn(t *testing.T) {
	// Old-style chunks without structured items.
	chunks := []transcript.Chunk{
		{
			Type:       transcript.AIChunk,
			Model:      "claude-opus-4-6",
			Text:       "Done.",
			StopReason: "end_turn",
		},
	}
	if transcript.IsOngoing(chunks) {
		t.Error("AI chunk with stop_reason end_turn should not be ongoing (fallback)")
	}
}

func TestIsOngoing_Fallback_LastAIChunkNoStopReason(t *testing.T) {
	// Old-style chunk without stop reason = still streaming.
	chunks := []transcript.Chunk{
		{
			Type:  transcript.AIChunk,
			Model: "claude-opus-4-6",
			Text:  "Working...",
		},
	}
	if !transcript.IsOngoing(chunks) {
		t.Error("AI chunk with no stop_reason should be ongoing (fallback)")
	}
}

func TestIsOngoing_UserChunksOnly(t *testing.T) {
	// A session with only a user prompt means Claude is processing.
	// Dead-session case is handled by staleness thresholds in callers.
	chunks := []transcript.Chunk{
		{Type: transcript.UserChunk, UserText: "Hello"},
	}
	if !transcript.IsOngoing(chunks) {
		t.Error("trailing user prompt should be ongoing (Claude is processing)")
	}
}

func TestIsOngoing_UserPromptAfterCompletedTurn(t *testing.T) {
	// AI completes with text output, then user types a new prompt.
	// Should be ongoing — Claude is processing the new request.
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.UserMsg{Timestamp: t0, Text: "First question"},
		transcript.AIMsg{
			Timestamp:  t0.Add(1 * time.Second),
			Model:      "claude-opus-4-6",
			StopReason: "end_turn",
			Blocks:     []transcript.ContentBlock{{Type: "text", Text: "Here's the answer."}},
		},
		transcript.UserMsg{Timestamp: t0.Add(10 * time.Second), Text: "Follow-up question"},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("trailing user prompt after completed turn should be ongoing")
	}
}

func TestIsOngoing_MultipleChunks_OngoingInLast(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	// First turn: complete (text output)
	// Second turn: ongoing (tool_use, no result)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.UserMsg{Timestamp: t0, Text: "First question"},
		transcript.AIMsg{
			Timestamp: t0.Add(1 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks:    []transcript.ContentBlock{{Type: "text", Text: "First answer."}},
		},
		transcript.UserMsg{Timestamp: t0.Add(5 * time.Second), Text: "Second question"},
		transcript.AIMsg{
			Timestamp: t0.Add(6 * time.Second),
			Model:     "claude-opus-4-6",
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "Bash"}},
			Blocks: []transcript.ContentBlock{
				{Type: "tool_use", ToolID: "c1", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"make"}`)},
			},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("should be ongoing when last AI chunk has pending tool_use")
	}
}

func TestIsOngoing_SubagentSpawnIsActivity(t *testing.T) {
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	taskInput := json.RawMessage(`{"subagent_type":"Explore","description":"Find stuff"}`)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Let me search."},
				{Type: "tool_use", ToolID: "c1", ToolName: "Task", ToolInput: taskInput},
			},
			ToolCalls: []transcript.ToolCall{{ID: "c1", Name: "Task"}},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("subagent spawn (Task tool) after text should be ongoing")
	}
}

func TestIsOngoing_PendingTaskMaskedByTextOutput(t *testing.T) {
	// Simulates a team session where Agent A completed and Agent B is still
	// running. The parent wrote text output after processing Agent A's result,
	// which the activity-based check sees as the last ending event with no
	// AI activity after it. But Agent B's Task tool call has no result yet.
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	taskInputA := json.RawMessage(`{"subagent_type":"general-purpose","description":"Agent A"}`)
	taskInputB := json.RawMessage(`{"subagent_type":"general-purpose","description":"Agent B"}`)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		// Initial AI turn: spawn both agents.
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Spawning the team."},
				{Type: "tool_use", ToolID: "taskA", ToolName: "Task", ToolInput: taskInputA},
				{Type: "tool_use", ToolID: "taskB", ToolName: "Task", ToolInput: taskInputB},
			},
			ToolCalls: []transcript.ToolCall{
				{ID: "taskA", Name: "Task"},
				{ID: "taskB", Name: "Task"},
			},
		},
		// Agent A completes — tool_result for taskA.
		transcript.AIMsg{
			Timestamp: t0.Add(30 * time.Second),
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "taskA", Content: "Agent A finished."},
			},
		},
		// Parent processes the result and writes text output.
		transcript.AIMsg{
			Timestamp: t0.Add(31 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Agent A completed. Waiting for Agent B."},
			},
		},
		// Agent B is still running — no tool_result for taskB.
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("should be ongoing: taskB has no result, Agent B is still running")
	}
}

func TestIsOngoing_AllTasksCompleted(t *testing.T) {
	// Same structure as above but both agents have completed.
	// Should NOT be ongoing.
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	taskInputA := json.RawMessage(`{"subagent_type":"general-purpose","description":"Agent A"}`)
	taskInputB := json.RawMessage(`{"subagent_type":"general-purpose","description":"Agent B"}`)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Spawning the team."},
				{Type: "tool_use", ToolID: "taskA", ToolName: "Task", ToolInput: taskInputA},
				{Type: "tool_use", ToolID: "taskB", ToolName: "Task", ToolInput: taskInputB},
			},
			ToolCalls: []transcript.ToolCall{
				{ID: "taskA", Name: "Task"},
				{ID: "taskB", Name: "Task"},
			},
		},
		transcript.AIMsg{
			Timestamp: t0.Add(30 * time.Second),
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "taskA", Content: "Agent A finished."},
			},
		},
		transcript.AIMsg{
			Timestamp: t0.Add(60 * time.Second),
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "taskB", Content: "Agent B finished."},
			},
		},
		transcript.AIMsg{
			Timestamp: t0.Add(61 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Both agents completed successfully."},
			},
		},
	})
	if transcript.IsOngoing(chunks) {
		t.Error("should not be ongoing: all tasks have results and session ends with text")
	}
}

func TestIsOngoing_PendingRegularToolCall(t *testing.T) {
	// A regular tool call (not Task/Agent) without a result is NOT ongoing.
	// Missing results on regular tools mean the session was interrupted or
	// the JSONL is incomplete — not evidence of active work.
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "tool_use", ToolID: "c1", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"make test"}`)},
				{Type: "tool_use", ToolID: "c2", ToolName: "Read", ToolInput: json.RawMessage(`{"file_path":"x.go"}`)},
			},
			ToolCalls: []transcript.ToolCall{
				{ID: "c1", Name: "Bash"},
				{ID: "c2", Name: "Read"},
			},
		},
		transcript.AIMsg{
			Timestamp: t0.Add(1 * time.Second),
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "c1", Content: "ok"},
			},
		},
		transcript.AIMsg{
			Timestamp: t0.Add(2 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Bash finished."},
			},
		},
		// c2 (Read) still has no result — but it's a regular tool, not an agent.
	})
	if transcript.IsOngoing(chunks) {
		t.Error("should not be ongoing: pending Read tool call is not an agent")
	}
}

func TestIsOngoing_PendingAgentToolCall(t *testing.T) {
	// An Agent tool call (not Task) without a result IS ongoing.
	// Both "Task" and "Agent" are subagent spawners that can run for minutes.
	t0 := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	agentInput := json.RawMessage(`{"subagent_type":"Explore","description":"Find stuff","prompt":"search"}`)
	chunks := transcript.BuildChunks([]transcript.ClassifiedMsg{
		transcript.AIMsg{
			Timestamp: t0,
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Let me search for that."},
				{Type: "tool_use", ToolID: "a1", ToolName: "Agent", ToolInput: agentInput},
			},
			ToolCalls: []transcript.ToolCall{{ID: "a1", Name: "Agent"}},
		},
		// Agent result arrives for first spawn.
		transcript.AIMsg{
			Timestamp: t0.Add(10 * time.Second),
			IsMeta:    true,
			Blocks: []transcript.ContentBlock{
				{Type: "tool_result", ToolID: "a1", Content: "Found it."},
			},
		},
		// Parent spawns another Agent, still pending.
		transcript.AIMsg{
			Timestamp: t0.Add(11 * time.Second),
			Model:     "claude-opus-4-6",
			Blocks: []transcript.ContentBlock{
				{Type: "text", Text: "Let me also check this."},
				{Type: "tool_use", ToolID: "a2", ToolName: "Agent", ToolInput: agentInput},
			},
			ToolCalls: []transcript.ToolCall{{ID: "a2", Name: "Agent"}},
		},
	})
	if !transcript.IsOngoing(chunks) {
		t.Error("should be ongoing: Agent tool call a2 has no result")
	}
}
