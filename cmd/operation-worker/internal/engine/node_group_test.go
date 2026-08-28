package engine

import (
	"testing"
)

func TestBatchConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  BatchConfig
		wantErr bool
	}{
		{
			name: "valid three batches",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 1, Percent: 10},
					{Order: 2, Percent: 30},
					{Order: 3, Percent: 60},
				},
			},
			wantErr: false,
		},
		{
			name: "single batch",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 1, Percent: 100},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty batches",
			config:  BatchConfig{},
			wantErr: true,
		},
		{
			name: "percent sums to 90",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 1, Percent: 50},
					{Order: 2, Percent: 40},
				},
			},
			wantErr: true,
		},
		{
			name: "percent sums to 110",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 1, Percent: 60},
					{Order: 2, Percent: 50},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate order",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 1, Percent: 50},
					{Order: 1, Percent: 50},
				},
			},
			wantErr: true,
		},
		{
			name: "non-sequential order",
			config: BatchConfig{
				Batches: []BatchDefinition{
					{Order: 3, Percent: 30},
					{Order: 1, Percent: 30},
					{Order: 2, Percent: 40},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewBatchManager(t *testing.T) {
	config := BatchConfig{
		Batches: []BatchDefinition{
			{Order: 1, Percent: 30},
			{Order: 2, Percent: 70},
		},
	}
	bm := NewBatchManager(config)
	if len(bm.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(bm.States))
	}
	if bm.States[0].Status != BatchInProgress {
		t.Errorf("first batch should be in progress, got %s", bm.States[0].Status)
	}
	if bm.States[1].Status != BatchPending {
		t.Errorf("second batch should be pending, got %s", bm.States[1].Status)
	}
}

func TestBatchManagerCompleteBatch(t *testing.T) {
	config := BatchConfig{
		Batches: []BatchDefinition{
			{Order: 1, Percent: 30},
			{Order: 2, Percent: 70},
		},
	}
	bm := NewBatchManager(config)

	result := bm.CompleteBatch(1, 0)
	if !result.Passed {
		t.Error("health gate should pass")
	}
	if bm.States[0].Status != BatchSucceeded {
		t.Errorf("batch 1 should be succeeded, got %s", bm.States[0].Status)
	}
	if bm.States[1].Status != BatchInProgress {
		t.Errorf("batch 2 should be in progress, got %s", bm.States[1].Status)
	}

	bm.CompleteBatch(2, 0)
	if !bm.AllSucceeded() {
		t.Error("all batches should be succeeded")
	}
}

func TestBatchManagerFailBatch(t *testing.T) {
	config := BatchConfig{
		Batches: []BatchDefinition{
			{Order: 1, Percent: 30},
			{Order: 2, Percent: 70},
		},
	}
	bm := NewBatchManager(config)

	bm.FailBatch(1)
	if !bm.HasFailed() {
		t.Error("should have failed batch")
	}
	if !bm.Paused {
		t.Error("manager should be paused after failure")
	}
	if bm.States[1].Status != BatchPaused {
		t.Errorf("subsequent batches should be paused, got %s", bm.States[1].Status)
	}
}

func TestBatchManagerPauseResume(t *testing.T) {
	config := BatchConfig{
		Batches: []BatchDefinition{
			{Order: 1, Percent: 100},
		},
	}
	bm := NewBatchManager(config)

	bm.Pause()
	if !bm.Paused {
		t.Error("should be paused")
	}
	if bm.States[0].Status != BatchPaused {
		t.Errorf("current batch should be paused, got %s", bm.States[0].Status)
	}

	bm.Resume()
	if bm.Paused {
		t.Error("should not be paused after resume")
	}
	if bm.States[0].Status != BatchInProgress {
		t.Errorf("current batch should be in progress, got %s", bm.States[0].Status)
	}
}

func TestBatchManagerCurrentBatch(t *testing.T) {
	config := BatchConfig{
		Batches: []BatchDefinition{
			{Order: 1, Percent: 50},
			{Order: 2, Percent: 50},
		},
	}
	bm := NewBatchManager(config)

	current := bm.CurrentBatch()
	if current == nil || current.Batch.Order != 1 {
		t.Fatalf("expected batch 1, got %v", current)
	}

	bm.CompleteBatch(1, 0)
	current = bm.CurrentBatch()
	if current == nil || current.Batch.Order != 2 {
		t.Fatalf("expected batch 2, got %v", current)
	}

	bm.CompleteBatch(2, 0)
	if bm.CurrentBatch() != nil {
		t.Error("no current batch when all done")
	}
}
