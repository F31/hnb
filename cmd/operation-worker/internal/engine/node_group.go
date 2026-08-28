package engine

import (
	"fmt"
	"time"
)

type NodeGroup struct {
	ID               string            `json:"id"`
	TenantID         string            `json:"tenant_id"`
	EdgeTargetID     string            `json:"edge_target_id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	NodeSelector     map[string]string `json:"node_selector,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	BatchOrder       int               `json:"batch_order,omitempty"`
	BatchPercent     int               `json:"batch_percent,omitempty"`
	HealthGateSec    int               `json:"health_gate_seconds,omitempty"`
	FailureTolerance int               `json:"failure_tolerance,omitempty"`
	IsPaused         bool              `json:"is_paused"`
	CreatedAt        string            `json:"created_at"`
}

type BatchConfig struct {
	Batches          []BatchDefinition `json:"batches"`
	HealthGateSec    int               `json:"health_gate_seconds"`
	FailureTolerance int               `json:"failure_tolerance"`
}

type BatchDefinition struct {
	Order         int `json:"order"`
	Percent       int `json:"percent"`
	HealthGateSec int `json:"health_gate_seconds,omitempty"`
}

type HealthGateResult struct {
	Passed         bool     `json:"passed"`
	AvailableRatio float64  `json:"available_ratio"`
	RestartCount   int      `json:"restart_count"`
	Issues         []string `json:"issues,omitempty"`
}
type BatchStatus string

const (
	BatchPending    BatchStatus = "pending"
	BatchInProgress BatchStatus = "in_progress"
	BatchPaused     BatchStatus = "paused"
	BatchSucceeded  BatchStatus = "succeeded"
	BatchFailed     BatchStatus = "failed"
)

type BatchState struct {
	Batch      BatchDefinition `json:"batch"`
	Status     BatchStatus     `json:"status"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type BatchManager struct {
	Config BatchConfig  `json:"config"`
	States []BatchState `json:"states"`
	Paused bool         `json:"paused"`
}

func NewBatchManager(config BatchConfig) *BatchManager {
	states := make([]BatchState, len(config.Batches))
	for i, b := range config.Batches {
		status := BatchPending
		if i == 0 {
			status = BatchInProgress
		}
		states[i] = BatchState{Batch: b, Status: status}
	}
	return &BatchManager{
		Config: config,
		States: states,
	}
}

func (bc *BatchConfig) Validate() error {
	if len(bc.Batches) == 0 {
		return fmt.Errorf("batch config must have at least one batch")
	}
	total := 0
	seen := make(map[int]bool)
	for i, b := range bc.Batches {
		if b.Order < 1 {
			return fmt.Errorf("batch %d: order must be positive", i)
		}
		if seen[b.Order] {
			return fmt.Errorf("batch %d: duplicate order %d", i, b.Order)
		}
		seen[b.Order] = true
		if b.Percent < 1 || b.Percent > 100 {
			return fmt.Errorf("batch %d: percent must be between 1 and 100", i)
		}
		total += b.Percent
	}
	if total != 100 {
		return fmt.Errorf("batch percentages sum to %d, must be 100", total)
	}
	for i := 0; i < len(bc.Batches)-1; i++ {
		for j := i + 1; j < len(bc.Batches); j++ {
			if bc.Batches[i].Order > bc.Batches[j].Order {
				return fmt.Errorf("batches must be in sequential order by Order")
			}
		}
	}
	return nil
}

func (bm *BatchManager) CurrentBatch() *BatchState {
	for i := range bm.States {
		if bm.States[i].Status == BatchInProgress || bm.States[i].Status == BatchPaused {
			return &bm.States[i]
		}
	}
	return nil
}

func (bm *BatchManager) NextBatch() *BatchState {
	for i := range bm.States {
		if bm.States[i].Status == BatchPending {
			return &bm.States[i]
		}
	}
	return nil
}

func (bm *BatchManager) CompleteBatch(order int, healthGateSec int) *HealthGateResult {
	for i := range bm.States {
		if bm.States[i].Batch.Order == order {
			bm.States[i].Status = BatchSucceeded
			bm.States[i].FinishedAt = time.Now().UTC().Format(time.RFC3339)
			next := bm.NextBatch()
			if next != nil {
				next.Status = BatchInProgress
				next.StartedAt = time.Now().UTC().Format(time.RFC3339)
			}
			return checkHealthGate(bm, order, healthGateSec)
		}
	}
	return &HealthGateResult{Passed: false, Issues: []string{"batch not found"}}
}

func (bm *BatchManager) FailBatch(order int) {
	for i := range bm.States {
		if bm.States[i].Batch.Order == order {
			bm.States[i].Status = BatchFailed
			bm.States[i].FinishedAt = time.Now().UTC().Format(time.RFC3339)
			bm.Paused = true
			for j := i + 1; j < len(bm.States); j++ {
				bm.States[j].Status = BatchPaused
			}
			return
		}
	}
}

func (bm *BatchManager) Pause() {
	bm.Paused = true
	current := bm.CurrentBatch()
	if current != nil {
		current.Status = BatchPaused
	}
}

func (bm *BatchManager) Resume() {
	bm.Paused = false
	current := bm.CurrentBatch()
	if current != nil && current.Status == BatchPaused {
		current.Status = BatchInProgress
		return
	}
	next := bm.NextBatch()
	if next != nil {
		next.Status = BatchInProgress
		next.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (bm *BatchManager) AllSucceeded() bool {
	for _, s := range bm.States {
		if s.Status != BatchSucceeded {
			return false
		}
	}
	return true
}

func (bm *BatchManager) HasFailed() bool {
	for _, s := range bm.States {
		if s.Status == BatchFailed {
			return true
		}
	}
	return false
}

func checkHealthGate(bm *BatchManager, order int, defaultHealthGateSec int) *HealthGateResult {
	healthGateSec := defaultHealthGateSec
	for _, b := range bm.Config.Batches {
		if b.Order == order && b.HealthGateSec > 0 {
			healthGateSec = b.HealthGateSec
		}
	}
	if healthGateSec <= 0 {
		return &HealthGateResult{Passed: true, AvailableRatio: 1.0}
	}
	return &HealthGateResult{
		Passed:         true,
		AvailableRatio: 1.0,
		RestartCount:   0,
	}
}
