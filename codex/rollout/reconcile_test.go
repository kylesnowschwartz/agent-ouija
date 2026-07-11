package rollout_test

import (
	"os"
	"testing"

	"github.com/kylesnowschwartz/agent-ouija/codex/rollout"
)

func TestClaimForHookEvent(t *testing.T) {
	tests := []struct {
		event string
		want  rollout.Claim
		ok    bool
	}{
		{"SessionStart", rollout.ClaimSessionStarted, true},
		{"UserPromptSubmit", rollout.ClaimTurnStarted, true},
		{"PreToolUse", rollout.ClaimWorking, true},
		{"PostToolUse", rollout.ClaimWorking, true},
		{"PermissionRequest", rollout.ClaimApprovalRequested, true},
		{"Stop", rollout.ClaimTurnStopped, true},
		{"Unknown", rollout.ClaimUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			got, ok := rollout.ClaimForHookEvent(tt.event)
			if got != tt.want || ok != tt.ok {
				t.Errorf("ClaimForHookEvent(%q) = (%d, %t), want (%d, %t)", tt.event, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestClaimNeedsEvidence(t *testing.T) {
	tests := []struct {
		claim rollout.Claim
		want  bool
	}{
		{rollout.ClaimUnknown, false},
		{rollout.ClaimSessionStarted, false},
		{rollout.ClaimTurnStarted, false},
		{rollout.ClaimWorking, false},
		{rollout.ClaimApprovalRequested, true},
		{rollout.ClaimTurnStopped, true},
	}
	for _, tt := range tests {
		if got := tt.claim.NeedsEvidence(); got != tt.want {
			t.Errorf("Claim(%d).NeedsEvidence() = %t, want %t", tt.claim, got, tt.want)
		}
	}
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name     string
		claim    rollout.Claim
		evidence rollout.State
		want     rollout.Verdict
	}{
		{"stopped done", rollout.ClaimTurnStopped, rollout.State{Status: rollout.Done}, rollout.VerdictDone},
		{"stopped interrupted", rollout.ClaimTurnStopped, rollout.State{Status: rollout.Interrupted}, rollout.VerdictInterrupted},
		{"stopped error", rollout.ClaimTurnStopped, rollout.State{Status: rollout.Error}, rollout.VerdictError},
		{"stopped running", rollout.ClaimTurnStopped, rollout.State{Status: rollout.Running}, rollout.VerdictIdle},
		{"stopped idle", rollout.ClaimTurnStopped, rollout.State{Status: rollout.Idle, Cwd: "/work/proj"}, rollout.VerdictIdle},
		{"stopped zero state", rollout.ClaimTurnStopped, rollout.State{}, rollout.VerdictIdle},
		{"approval auto reviewed", rollout.ClaimApprovalRequested, rollout.State{ApprovalsReviewer: rollout.AutoReviewReviewer}, rollout.VerdictRunning},
		{"approval empty reviewer", rollout.ClaimApprovalRequested, rollout.State{ApprovalsReviewer: ""}, rollout.VerdictWaiting},
		{"approval zero state", rollout.ClaimApprovalRequested, rollout.State{}, rollout.VerdictWaiting},
		{"session started", rollout.ClaimSessionStarted, rollout.State{}, rollout.VerdictIdle},
		{"turn started", rollout.ClaimTurnStarted, rollout.State{}, rollout.VerdictRunning},
		{"working", rollout.ClaimWorking, rollout.State{}, rollout.VerdictRunning},
		{"unknown", rollout.ClaimUnknown, rollout.State{}, rollout.VerdictIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rollout.Reconcile(tt.claim, tt.evidence); got != tt.want {
				t.Errorf("Reconcile(%d, %+v) = %s, want %s", tt.claim, tt.evidence, got, tt.want)
			}
		})
	}
}

func TestReconcileApprovalFixtures(t *testing.T) {
	tests := []struct {
		path string
		want rollout.Verdict
	}{
		{"testdata/auto-reviewed-approval.jsonl", rollout.VerdictRunning},
		{"testdata/malformed-cwd-auto-review.jsonl", rollout.VerdictRunning},
		{"testdata/manual-approval.jsonl", rollout.VerdictWaiting},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			f, err := os.Open(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			state, err := rollout.TrailingState(f)
			if err != nil {
				t.Fatal(err)
			}
			if got := rollout.Reconcile(rollout.ClaimApprovalRequested, state); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestVerdictString(t *testing.T) {
	tests := []struct {
		verdict rollout.Verdict
		want    string
	}{
		{rollout.VerdictIdle, "idle"},
		{rollout.VerdictRunning, "running"},
		{rollout.VerdictWaiting, "waiting"},
		{rollout.VerdictDone, "done"},
		{rollout.VerdictInterrupted, "interrupted"},
		{rollout.VerdictError, "error"},
		{rollout.Verdict(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.verdict.String(); got != tt.want {
			t.Errorf("Verdict(%d).String() = %q, want %q", tt.verdict, got, tt.want)
		}
	}
}
