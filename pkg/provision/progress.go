package provision

import "time"

// StageEvent contains observations only, never a script, stdout, credentials,
// join token or kubeconfig. It is not an Operation or a resumable receipt.
type StageEvent struct {
	Stage      string    `json:"stage"`
	NodeID     string    `json:"nodeId"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	State      string    `json:"state"` // running | succeeded | failed
}

// ProgressObserver is caller-owned: CLI receipts and Provider progress can
// consume the same stage events without the executor acquiring store ownership.
// Observe is synchronous; observers must not block on remote execution.
type ProgressObserver interface{ Observe(StageEvent) }

func WithProgressObserver(observer ProgressObserver) Option {
	return func(p *Provisioner) { p.observer = observer }
}
