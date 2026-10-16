package cli

import "fmt"

// Structured exit codes for hnbctl. Plaintext script and the portal wizard read
// these to distinguish success from unsupported/blocked/partial outcomes.
const (
	// ExitSuccess means the requested command reached its normal terminal state.
	ExitSuccess = 0
	// ExitError is a generic execution failure.
	ExitError = 1
	// ExitUsage means unknown command, bad flags or invalid input.
	ExitUsage = 2
	// ExitBlocked means a capability is not certified/supported yet.
	ExitBlocked = 3
	// ExitNotVerified means a lower layer succeeded but a required higher layer
	// (for example platform login or a real managed operation) did not.
	ExitNotVerified = 4
	// ExitConnectionFailed means the managed API was unreachable and no local
	// fallback was allowed.
	ExitConnectionFailed = 5
)

// ExitReason is a stable machine-readable failure code.
type ExitReason string

const (
	ReasonInvalidConfig       ExitReason = "INVALID_CONFIG"
	ReasonTargetActionMissing ExitReason = "TARGET_ACTION_UNSUPPORTED"
	ReasonProfileBlocked      ExitReason = "PROFILE_NOT_CERTIFIED"
	ReasonPreflightBlocked    ExitReason = "PREFLIGHT_BLOCKED"
	ReasonPlatformNotVerified ExitReason = "PLATFORM_NOT_VERIFIED"
	ReasonAPIUnreachable      ExitReason = "API_UNREACHABLE"
	ReasonOutputFailed        ExitReason = "OUTPUT_FAILED"
)

func exitCodeFor(reason ExitReason) int {
	switch reason {
	case ReasonInvalidConfig:
		return ExitUsage
	case ReasonTargetActionMissing, ReasonProfileBlocked, ReasonPreflightBlocked:
		return ExitBlocked
	case ReasonPlatformNotVerified:
		return ExitNotVerified
	case ReasonAPIUnreachable:
		return ExitConnectionFailed
	default:
		return ExitError
	}
}

// Result is the structured command outcome.
type Result struct {
	ExitCode int
	Reason   ExitReason
	Message  string
}

func success(message string) Result {
	return Result{ExitCode: ExitSuccess, Message: message}
}

func fail(reason ExitReason, message string) Result {
	return Result{ExitCode: exitCodeFor(reason), Reason: reason, Message: message}
}

func failf(reason ExitReason, format string, args ...any) Result {
	return fail(reason, fmt.Sprintf(format, args...))
}
