package store

import (
	"context"
	"log"
	"time"
)

// OrphanedStepSweeper re-drives step requests that were lost with their consumer.
type OrphanedStepSweeper struct {
	store    *OperationStore
	idleFor  time.Duration
	interval time.Duration
	limit    int
}

func NewOrphanedStepSweeper(store *OperationStore, idleFor, interval time.Duration, limit int) *OrphanedStepSweeper {
	return &OrphanedStepSweeper{store: store, idleFor: idleFor, interval: interval, limit: limit}
}

func (s *OrphanedStepSweeper) Start(ctx context.Context) {
	log.Printf("[redrive] starting orphaned step sweeper (idle=%s interval=%s limit=%d)", s.idleFor, s.interval, s.limit)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[redrive] stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *OrphanedStepSweeper) sweep(ctx context.Context) {
	result, err := s.store.RequeueOrphanedSteps(ctx, s.idleFor, s.limit)
	if err != nil {
		log.Printf("[redrive] sweep failed: %v", err)
		return
	}
	if len(result.Requeued) > 0 {
		log.Printf("[redrive] requeued %d orphaned step request(s): %v", len(result.Requeued), result.Requeued)
	}
	// Exhausted steps stay pending on purpose: failing them needs the same lease
	// guarded transition as a real execution, so an operator must decide instead of
	// the worker silently terminating an Operation.
	for _, stepID := range result.Exhausted {
		log.Printf("[redrive] step %s exhausted its delivery budget and still has no worker; manual intervention required", stepID)
	}
}
