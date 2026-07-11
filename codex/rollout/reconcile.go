package rollout

// Claim is a caller-supplied Codex lifecycle-hook observation. The hook stream
// itself is the consumer's business (see codex/provider.go's LiveTracker note);
// this package only interprets what a claim means against rollout evidence.
type Claim uint8

const (
	// ClaimUnknown means the hook event is not recognized.
	ClaimUnknown Claim = iota
	// ClaimSessionStarted means a Codex session started.
	ClaimSessionStarted
	// ClaimTurnStarted means a user submitted a prompt.
	ClaimTurnStarted
	// ClaimWorking means Codex is performing or completing tool work.
	ClaimWorking
	// ClaimApprovalRequested means Codex requested permission.
	ClaimApprovalRequested
	// ClaimTurnStopped means Codex reported a turn boundary.
	ClaimTurnStopped
)

// ClaimForHookEvent maps a Codex lifecycle-hook event to a Claim.
func ClaimForHookEvent(event string) (Claim, bool) {
	switch event {
	case "SessionStart":
		return ClaimSessionStarted, true
	case "UserPromptSubmit":
		return ClaimTurnStarted, true
	case "PreToolUse", "PostToolUse":
		return ClaimWorking, true
	case "PermissionRequest":
		return ClaimApprovalRequested, true
	case "Stop":
		return ClaimTurnStopped, true
	default:
		return ClaimUnknown, false
	}
}

// NeedsEvidence reports whether reconciling the claim can change its verdict.
func (c Claim) NeedsEvidence() bool {
	return c == ClaimApprovalRequested || c == ClaimTurnStopped
}

// Verdict is the reconciled thread status. It is distinct from Status because
// a rollout stream alone can never show blocked-on-approval, so Waiting exists
// only here.
type Verdict int

const (
	// VerdictIdle means the thread is resting outside a turn.
	VerdictIdle Verdict = iota
	// VerdictRunning means the thread is working on a turn.
	VerdictRunning
	// VerdictWaiting means the thread is blocked on user approval.
	VerdictWaiting
	// VerdictDone means the turn completed successfully.
	VerdictDone
	// VerdictInterrupted means the turn was aborted by the user.
	VerdictInterrupted
	// VerdictError means the turn ended with an error.
	VerdictError
)

// String returns the lowercase verdict name.
func (v Verdict) String() string {
	switch v {
	case VerdictIdle:
		return "idle"
	case VerdictRunning:
		return "running"
	case VerdictWaiting:
		return "waiting"
	case VerdictDone:
		return "done"
	case VerdictInterrupted:
		return "interrupted"
	case VerdictError:
		return "error"
	default:
		return "unknown"
	}
}

// AutoReviewReviewer identifies an external reviewer that resolves approvals
// without user input.
const AutoReviewReviewer = "auto_review"

// Reconcile combines a lifecycle-hook claim with rollout evidence. A zero State
// (missing or unreadable rollout) degrades to the claim's own verdict; callers
// pass the zero value on read failure.
func Reconcile(claim Claim, evidence State) Verdict {
	switch claim {
	case ClaimTurnStopped:
		switch evidence.Status {
		case Done:
			return VerdictDone
		case Interrupted:
			return VerdictInterrupted
		case Error:
			return VerdictError
		default:
			return VerdictIdle
		}
	case ClaimApprovalRequested:
		if evidence.ApprovalsReviewer == AutoReviewReviewer {
			return VerdictRunning
		}
		return VerdictWaiting
	case ClaimSessionStarted:
		return VerdictIdle
	case ClaimTurnStarted, ClaimWorking:
		return VerdictRunning
	default:
		return VerdictIdle
	}
}
