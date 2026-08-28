package nats

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/operation-worker/internal/driver"
	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
	"github.com/F31/hnb/cmd/operation-worker/internal/store"
)

const (
	commandStream         = "commands"
	domainEventStream     = "domain-events"
	failedMessageStream   = "failed-messages"
	stepRequestedSubject  = "hnb.command.operation.step-requested.v1"
	stepCompletedSubject  = "hnb.event.operation.step-completed.v1"
	operationConsumer     = "operation-worker"
	maxDeliveries         = 10
	consumerMaxDeliveries = maxDeliveries + 1
)

var operationBackoff = []time.Duration{
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
	2 * time.Hour,
}

type Worker struct {
	js        jetstream.JetStream
	opStore   *store.OperationStore
	planStore *store.PlanStore
	runner    StepRunner
	leaseDur  time.Duration
	workerID  string
}

type StepRunner interface {
	Execute(context.Context, engine.ExecutionContext) (map[string]string, string, error)
}

func NewWorker(db *sql.DB, nc *natslib.Conn, leaseDuration time.Duration, runner StepRunner) (*Worker, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	id := uuid.NewString()
	return &Worker{
		js:        js,
		opStore:   store.NewOperationStore(db),
		planStore: store.NewPlanStore(db),
		runner:    runner,
		leaseDur:  leaseDuration,
		workerID:  fmt.Sprintf("worker-%s", id[:8]),
	}, nil
}

func (w *Worker) Start(ctx context.Context) error {
	log.Printf("[%s] starting worker", w.workerID)
	if err := w.ensureStreams(ctx); err != nil {
		return err
	}

	consumer, err := w.ensureConsumer(ctx)
	if err != nil {
		return err
	}

	consumeContext, err := consumer.Consume(func(msg jetstream.Msg) {
		w.handleMessage(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	log.Printf("[%s] listening on shared consumer %s", w.workerID, operationConsumer)
	<-ctx.Done()
	consumeContext.Stop()
	return nil
}

func (w *Worker) ensureConsumer(ctx context.Context) (jetstream.Consumer, error) {
	consumer, err := w.js.CreateOrUpdateConsumer(ctx, commandStream, jetstream.ConsumerConfig{
		Name:          operationConsumer,
		Durable:       operationConsumer,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       60 * time.Second,
		MaxDeliver:    consumerMaxDeliveries,
		BackOff:       operationBackoff,
		MaxAckPending: 16,
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
		FilterSubject: stepRequestedSubject,
	})
	if err != nil {
		return nil, fmt.Errorf("consumer: %w", err)
	}
	return consumer, nil
}

func (w *Worker) ensureStreams(ctx context.Context) error {
	_, err := w.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       commandStream,
		Subjects:   []string{"hnb.command.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.WorkQueuePolicy,
		MaxAge:     7 * 24 * time.Hour,
		MaxBytes:   1 << 30,
		MaxMsgSize: 2 << 20,
		Discard:    jetstream.DiscardOld,
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("command stream: %w", err)
	}
	_, err = w.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       failedMessageStream,
		Subjects:   []string{"hnb.failed.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   1 << 30,
		MaxMsgSize: 2 << 20,
		Discard:    jetstream.DiscardOld,
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("failed-message stream: %w", err)
	}
	return nil
}

type eventEnvelope struct {
	MessageID        string          `json:"messageId"`
	MessageType      string          `json:"messageType"`
	SchemaVersion    string          `json:"schemaVersion"`
	OccurredAt       time.Time       `json:"occurredAt"`
	TenantID         string          `json:"tenantId"`
	ProjectID        string          `json:"projectId,omitempty"`
	EnvironmentID    string          `json:"environmentId,omitempty"`
	ActorID          string          `json:"actorId,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	CausationID      string          `json:"causationId,omitempty"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	AggregateID      string          `json:"aggregateId,omitempty"`
	AggregateVersion int64           `json:"aggregateVersion,omitempty"`
	OperationID      string          `json:"operationId,omitempty"`
	StepID           string          `json:"stepId,omitempty"`
	ExpectedVersion  int64           `json:"expectedVersion,omitempty"`
	Payload          json.RawMessage `json:"payload"`
}

type stepRequestMessage struct {
	OperationID      string `json:"operationId"`
	StepID           string `json:"stepId"`
	StepType         string `json:"stepType"`
	IdempotencyKey   string `json:"idempotencyKey"`
	ExpectedVersion  int64  `json:"expectedVersion"`
	ExecutionContext string `json:"executionContext,omitempty"`
}

func (w *Worker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var envelope eventEnvelope
	if err := json.Unmarshal(msg.Data(), &envelope); err != nil {
		w.failPermanently(ctx, msg, fmt.Sprintf("invalid envelope JSON: %v", err))
		return
	}
	var request stepRequestMessage
	if err := json.Unmarshal(envelope.Payload, &request); err != nil {
		w.failPermanently(ctx, msg, fmt.Sprintf("invalid StepRequested payload: %v", err))
		return
	}
	if err := validateStepRequest(envelope, request); err != nil {
		w.failPermanently(ctx, msg, err.Error())
		return
	}

	op, err := w.opStore.GetOperationContext(ctx, request.OperationID)
	if err != nil {
		w.retryOrFail(ctx, msg, fmt.Errorf("load operation: %w", err))
		return
	}
	if engine.IsTerminal(op.Status) {
		log.Printf("[%s] operation %s is terminal (%s); acknowledging stale command", w.workerID, op.ID, op.Status)
		w.ack(msg)
		return
	}
	if op.Status != engine.StatusQueued && op.Status != engine.StatusInProgress {
		w.retryOrFail(ctx, msg, fmt.Errorf("operation %s is not runnable in state %s", op.ID, op.Status))
		return
	}
	if envelope.TenantID != op.TenantID ||
		(envelope.ProjectID != "" && envelope.ProjectID != op.ProjectID) ||
		(envelope.EnvironmentID != "" && envelope.EnvironmentID != op.EnvironmentID) {
		w.failPermanently(ctx, msg, "message scope does not match authoritative operation scope")
		return
	}

	step, err := w.opStore.GetStepState(ctx, request.OperationID, request.StepID)
	if err != nil {
		w.retryOrFail(ctx, msg, fmt.Errorf("load step: %w", err))
		return
	}
	if step.Status == engine.StepSucceeded {
		log.Printf("[%s] step %s already succeeded; acknowledging duplicate command", w.workerID, request.StepID)
		w.ack(msg)
		return
	}
	if step.IdempotencyKey != request.IdempotencyKey || step.Version != request.ExpectedVersion {
		log.Printf("[%s] stale step command %s: expected version %d, current version %d", w.workerID, request.StepID, request.ExpectedVersion, step.Version)
		w.ack(msg)
		return
	}
	if step.StepType != request.StepType {
		w.failPermanently(ctx, msg, "message stepType does not match authoritative step type")
		return
	}
	dependenciesSatisfied, err := w.opStore.DependenciesSatisfied(ctx, request.OperationID, step.DependsOn)
	if err != nil {
		w.retryOrFail(ctx, msg, err)
		return
	}

	lease, err := w.opStore.AcquireLease(
		ctx, request.OperationID, request.StepID, w.workerID, request.ExpectedVersion, w.leaseDur,
	)
	if errors.Is(err, store.ErrLeaseNotAcquired) {
		w.retryOrFailAfter(ctx, msg, err, w.leaseDur)
		return
	}
	if err != nil {
		w.retryOrFail(ctx, msg, err)
		return
	}
	now := time.Now().UTC()
	if !dependenciesSatisfied {
		w.failStepExecution(
			ctx, msg, op, step, envelope, request, lease, now,
			nil, "", errors.New("step was dispatched before its prerequisites succeeded"), true,
		)
		return
	}
	if w.runner == nil {
		w.failStepExecution(
			ctx, msg, op, step, envelope, request, lease, now,
			nil, "", errors.New("no runtime step runner is configured"), true,
		)
		return
	}

	outputs, checkpoint, err := w.executeStep(
		ctx, msg, request.StepID, lease,
		time.Duration(step.TimeoutSeconds)*time.Second,
		engine.ExecutionContext{
			StepID:             request.StepID,
			OperationID:        request.OperationID,
			TenantID:           op.TenantID,
			ProjectID:          op.ProjectID,
			EnvironmentID:      op.EnvironmentID,
			StepType:           step.StepType,
			Inputs:             step.Inputs,
			ProviderID:         step.ProviderID,
			ProviderVersion:    step.ProviderVersion,
			ProviderDigest:     step.ProviderDigest,
			Checkpoint:         step.Checkpoint,
			IdempotencyKey:     request.IdempotencyKey,
			ExecutionAttemptID: lease.ID,
			FencingGeneration:  lease.Generation,
			NodeGroupAffinity:  w.loadNodeGroupAffinity(ctx, op.PlanID),
			TargetClusterIDs:   op.TargetClusterIDs,
		},
	)
	if err != nil {
		permanent, fenced := classifyExecutionError(err)
		if fenced {
			w.retryOrFailAfter(ctx, msg, err, w.leaseDur)
			return
		}
		w.failStepExecution(ctx, msg, op, step, envelope, request, lease, now, outputs, checkpoint, err, permanent)
		return
	}
	completedAt := time.Now().UTC()
	result := engine.StepResult{
		StepID:      request.StepID,
		OperationID: request.OperationID,
		Status:      engine.StepSucceeded,
		StartedAt:   now,
		CompletedAt: completedAt,
		Outputs:     stringMapToAny(outputs),
		Checkpoint:  checkpoint,
	}
	eventID := uuid.NewString()
	err = w.opStore.CommitStepSuccess(
		ctx,
		request.OperationID,
		request.StepID,
		request.IdempotencyKey,
		w.workerID,
		lease,
		request.ExpectedVersion,
		result,
		store.OutboxEvent{
			MessageID:       eventID,
			MessageType:     stepCompletedSubject,
			SchemaVersion:   "1.0.0",
			Subject:         stepCompletedSubject,
			OccurredAt:      completedAt,
			TenantID:        op.TenantID,
			ProjectID:       op.ProjectID,
			EnvironmentID:   op.EnvironmentID,
			ActorID:         w.workerID,
			CorrelationID:   envelope.CorrelationID,
			CausationID:     envelope.MessageID,
			IdempotencyKey:  "step-completed:" + request.StepID,
			AggregateID:     request.OperationID,
			OperationID:     request.OperationID,
			StepID:          request.StepID,
			ExpectedVersion: request.ExpectedVersion + 1,
			Payload: map[string]any{
				"operationId": request.OperationID,
				"stepId":      request.StepID,
				"status":      string(engine.StepSucceeded),
			},
		},
	)
	if errors.Is(err, store.ErrStepAlreadyCompleted) || errors.Is(err, store.ErrStepVersionMismatch) {
		w.ack(msg)
		return
	}
	if err != nil {
		w.retryOrFail(ctx, msg, err)
		return
	}

	w.ack(msg)
	log.Printf("[%s] step %s committed with outbox event %s", w.workerID, request.StepID, eventID)
}

func (w *Worker) failStepExecution(
	ctx context.Context,
	msg jetstream.Msg,
	op *engine.Operation,
	step store.StepState,
	envelope eventEnvelope,
	request stepRequestMessage,
	lease store.Lease,
	startedAt time.Time,
	outputs map[string]string,
	checkpoint string,
	cause error,
	permanent bool,
) {
	failedMessageID, deliveries := failedMessageIdentity(msg)
	completedAt := time.Now().UTC()
	failedSubject := "hnb.failed." + strings.TrimPrefix(msg.Subject(), "hnb.")
	failurePayload := map[string]any{
		"originalSubject":  msg.Subject(),
		"failedAt":         completedAt.Format(time.RFC3339Nano),
		"failureReason":    cause.Error(),
		"deliveryAttempts": deliveries,
		"messageBase64":    base64.StdEncoding.EncodeToString(msg.Data()),
	}
	result := engine.StepResult{
		StepID:       request.StepID,
		OperationID:  request.OperationID,
		Status:       engine.StepFailed,
		ErrorMessage: cause.Error(),
		Outputs:      stringMapToAny(outputs),
		Checkpoint:   checkpoint,
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
	}
	if !permanent && step.RetryCount < step.MaxRetries && deliveries < maxDeliveries {
		if err := w.opStore.SaveStepRetry(
			ctx, request.OperationID, request.StepID, w.workerID, lease,
			request.ExpectedVersion, result,
		); err != nil {
			w.retryOrFailAfter(ctx, msg, fmt.Errorf("persist step retry: %w", err), w.leaseDur)
			return
		}
		log.Printf(
			"[%s] step %s retry %d/%d checkpoint persisted",
			w.workerID, request.StepID, step.RetryCount+1, step.MaxRetries,
		)
		_ = msg.Nak()
		return
	}
	eventID := uuid.NewString()
	err := w.opStore.CommitStepFailure(
		ctx,
		request.OperationID,
		request.StepID,
		request.IdempotencyKey,
		w.workerID,
		lease,
		request.ExpectedVersion,
		result,
		store.OutboxEvent{
			MessageID:       eventID,
			MessageType:     stepCompletedSubject,
			SchemaVersion:   "1.0.0",
			Subject:         stepCompletedSubject,
			OccurredAt:      completedAt,
			TenantID:        op.TenantID,
			ProjectID:       op.ProjectID,
			EnvironmentID:   op.EnvironmentID,
			ActorID:         w.workerID,
			CorrelationID:   envelope.CorrelationID,
			CausationID:     envelope.MessageID,
			IdempotencyKey:  "step-failed:" + request.StepID,
			AggregateID:     request.OperationID,
			OperationID:     request.OperationID,
			StepID:          request.StepID,
			ExpectedVersion: request.ExpectedVersion + 1,
			Payload: map[string]any{
				"operationId": request.OperationID,
				"stepId":      request.StepID,
				"status":      string(engine.StepFailed),
				"error":       cause.Error(),
			},
		},
		store.OutboxEvent{
			MessageID:       failedMessageID,
			MessageType:     failedSubject,
			SchemaVersion:   "1.0.0",
			Subject:         failedSubject,
			OccurredAt:      completedAt,
			TenantID:        op.TenantID,
			ProjectID:       op.ProjectID,
			EnvironmentID:   op.EnvironmentID,
			ActorID:         w.workerID,
			CorrelationID:   envelope.CorrelationID,
			CausationID:     envelope.MessageID,
			IdempotencyKey:  "failed-message:" + failedMessageID,
			AggregateID:     request.OperationID,
			OperationID:     request.OperationID,
			StepID:          request.StepID,
			ExpectedVersion: request.ExpectedVersion + 1,
			Payload:         failurePayload,
		},
	)
	if errors.Is(err, store.ErrStepAlreadyCompleted) {
		w.ack(msg)
		return
	}
	if err != nil {
		if deliveries >= consumerMaxDeliveries {
			w.failPermanently(ctx, msg, fmt.Sprintf("persist terminal step failure: %v", err))
			return
		}
		log.Printf("[%s] persist terminal step failure: %v", w.workerID, err)
		_ = msg.NakWithDelay(w.leaseDur)
		return
	}
	log.Printf("[%s] terminal step failure persisted with failed-message outbox event %s", w.workerID, failedMessageID)
	if err := msg.TermWithReason("terminal failure persisted to outbox"); err != nil {
		log.Printf("[%s] terminate failed step command: %v", w.workerID, err)
	}
}

func validateStepRequest(envelope eventEnvelope, request stepRequestMessage) error {
	if envelope.MessageType != stepRequestedSubject {
		return fmt.Errorf("unexpected message type %q", envelope.MessageType)
	}
	if envelope.SchemaVersion != "1.0.0" {
		return fmt.Errorf("unsupported schema version %q", envelope.SchemaVersion)
	}
	for name, value := range map[string]string{
		"messageId":     envelope.MessageID,
		"correlationId": envelope.CorrelationID,
		"operationId":   request.OperationID,
		"stepId":        request.StepID,
	} {
		if _, err := uuid.Parse(value); err != nil {
			return fmt.Errorf("%s must be a UUID", name)
		}
	}
	if envelope.TenantID == "" || request.StepType == "" || request.IdempotencyKey == "" {
		return errors.New("tenantId, stepType, and idempotencyKey are required")
	}
	if envelope.OccurredAt.IsZero() {
		return errors.New("occurredAt is required")
	}
	if len(request.IdempotencyKey) > 128 {
		return errors.New("idempotencyKey exceeds 128 characters")
	}
	if request.ExpectedVersion < 0 {
		return errors.New("expectedVersion cannot be negative")
	}
	if envelope.OperationID != "" && envelope.OperationID != request.OperationID {
		return errors.New("envelope and payload operationId differ")
	}
	if envelope.StepID != "" && envelope.StepID != request.StepID {
		return errors.New("envelope and payload stepId differ")
	}
	if envelope.IdempotencyKey != request.IdempotencyKey {
		return errors.New("envelope and payload idempotencyKey differ")
	}
	return nil
}

func (w *Worker) retryOrFail(ctx context.Context, msg jetstream.Msg, cause error) {
	w.retryOrFailAfter(ctx, msg, cause, 0)
}

func (w *Worker) retryOrFailAfter(
	ctx context.Context,
	msg jetstream.Msg,
	cause error,
	delay time.Duration,
) {
	metadata, err := msg.Metadata()
	if err == nil && metadata.NumDelivered >= maxDeliveries {
		w.failPermanently(ctx, msg, cause.Error())
		return
	}
	log.Printf("[%s] transient processing error: %v", w.workerID, cause)
	if delay > 0 {
		_ = msg.NakWithDelay(delay)
		return
	}
	_ = msg.Nak()
}

func stringMapToAny(in map[string]string) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (w *Worker) executeStep(
	ctx context.Context,
	msg jetstream.Msg,
	stepID string,
	lease store.Lease,
	timeout time.Duration,
	execution engine.ExecutionContext,
) (map[string]string, string, error) {
	if err := msg.InProgress(); err != nil {
		return nil, "", fmt.Errorf("extend initial message acknowledgement deadline: %w", err)
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	heartbeatResult := make(chan error, 1)
	go w.maintainLease(runCtx, cancel, msg, stepID, lease, stopHeartbeat, heartbeatResult)

	outputs, checkpoint, runErr := w.runner.Execute(runCtx, execution)
	close(stopHeartbeat)
	heartbeatErr := <-heartbeatResult
	if heartbeatErr != nil {
		return outputs, checkpoint, heartbeatErr
	}
	return outputs, checkpoint, runErr
}

func (w *Worker) maintainLease(
	ctx context.Context,
	cancel context.CancelFunc,
	msg jetstream.Msg,
	stepID string,
	lease store.Lease,
	stop <-chan struct{},
	result chan<- error,
) {
	interval := w.leaseDur / 3
	maxInterval := operationBackoff[0] / 3
	if interval <= 0 || interval > maxInterval {
		interval = maxInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			result <- nil
			return
		case <-ctx.Done():
			result <- ctx.Err()
			return
		case <-ticker.C:
			if err := w.opStore.RenewLease(ctx, stepID, w.workerID, lease, w.leaseDur); err != nil {
				cancel()
				result <- err
				return
			}
			if err := msg.InProgress(); err != nil {
				cancel()
				result <- fmt.Errorf("refresh message acknowledgement deadline: %w", err)
				return
			}
		}
	}
}

func classifyExecutionError(err error) (permanent, fenced bool) {
	if errors.Is(err, store.ErrLeaseLost) {
		return false, true
	}
	var providerErr *driver.ProviderError
	if !errors.As(err, &providerErr) {
		return false, false
	}
	if providerErr.Code == driver.ErrorFenced {
		return false, true
	}
	return !providerErr.Retryable, false
}

func (w *Worker) failPermanently(ctx context.Context, msg jetstream.Msg, reason string) {
	originalSubject := msg.Subject()
	failedSubject := "hnb.failed." + strings.TrimPrefix(originalSubject, "hnb.")
	failedMessageID, deliveries := failedMessageIdentity(msg)
	payload, err := json.Marshal(map[string]any{
		"failedMessageId":  failedMessageID,
		"originalSubject":  originalSubject,
		"failedAt":         time.Now().UTC().Format(time.RFC3339Nano),
		"failureReason":    reason,
		"deliveryAttempts": deliveries,
		"messageBase64":    base64.StdEncoding.EncodeToString(msg.Data()),
	})
	if err != nil {
		log.Printf("[%s] encode failed message: %v", w.workerID, err)
		_ = msg.Nak()
		return
	}
	if _, err := w.js.Publish(ctx, failedSubject, payload, jetstream.WithMsgID(failedMessageID)); err != nil {
		log.Printf("[%s] publish failed message: %v", w.workerID, err)
		_ = msg.Nak()
		return
	}
	log.Printf("[%s] isolated message on %s: %s", w.workerID, failedSubject, reason)
	if err := msg.TermWithReason("published to failed subject"); err != nil {
		log.Printf("[%s] terminate poison message: %v", w.workerID, err)
	}
}

func failedMessageIdentity(msg jetstream.Msg) (string, uint64) {
	metadata, _ := msg.Metadata()
	deliveries := uint64(1)
	key := msg.Subject() + ":" + base64.StdEncoding.EncodeToString(msg.Data())
	if metadata != nil {
		deliveries = metadata.NumDelivered
		key = fmt.Sprintf("%s:%d", metadata.Stream, metadata.Sequence.Stream)
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String(), deliveries
}

func (w *Worker) ack(msg jetstream.Msg) {
	if err := msg.Ack(); err != nil {
		log.Printf("[%s] acknowledge message: %v", w.workerID, err)
	}
}

func (w *Worker) loadNodeGroupAffinity(ctx context.Context, planID string) []string {
	if planID == "" {
		return nil
	}
	plan, err := w.planStore.GetPlan(planID)
	if err != nil {
		return nil
	}
	return plan.NodeGroupAffinity
}
