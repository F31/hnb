package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/operation-worker/internal/driver"
	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

type delayedRunner struct {
	delay    time.Duration
	received chan engine.ExecutionContext
}

type expireFirstLeaseRunner struct {
	db       *sql.DB
	delegate *driver.HTTPRunner
	once     sync.Once
	received chan engine.ExecutionContext
}

func (r *expireFirstLeaseRunner) Execute(
	ctx context.Context,
	execution engine.ExecutionContext,
) (map[string]string, string, error) {
	select {
	case r.received <- execution:
	default:
	}
	outputs, checkpoint, err := r.delegate.Execute(ctx, execution)
	if err != nil {
		return outputs, checkpoint, err
	}
	r.once.Do(func() {
		_, err = r.db.ExecContext(ctx, `
			UPDATE worker_leases SET expires_at = now() - interval '1 second'
			WHERE step_id = $1 AND lease_id = $2 AND fencing_generation = $3`,
			execution.StepID, execution.ExecutionAttemptID, execution.FencingGeneration,
		)
	})
	return outputs, checkpoint, err
}

func TestWorkerRecoversProviderEffectAfterLeaseLoss(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	natsURL := os.Getenv("HNB_TEST_NATS_URL")
	providerURL := os.Getenv("HNB_TEST_RUNTIME_PROVIDER_URL")
	providerToken := os.Getenv("HNB_TEST_RUNTIME_PROVIDER_TOKEN")
	if dsn == "" || natsURL == "" || providerURL == "" || providerToken == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN, HNB_TEST_NATS_URL, HNB_TEST_RUNTIME_PROVIDER_URL, and HNB_TEST_RUNTIME_PROVIDER_TOKEN are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nc, err := natslib.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	httpRunner, err := driver.NewHTTPRunner(map[string]driver.ProviderConfig{"kind-provider": {
		Endpoint: providerURL, Audience: "hnb-kubernetes-provider", TokenSource: fixedTokenSource(providerToken),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan engine.ExecutionContext, 4)
	runner := &expireFirstLeaseRunner{db: db, delegate: httpRunner, received: received}
	worker, err := NewWorker(db, nc, 1500*time.Millisecond, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ensureStreams(ctx); err != nil {
		t.Fatal(err)
	}
	consumer, err := worker.ensureConsumer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	consumeContext, err := consumer.Consume(func(message jetstream.Msg) {
		worker.handleMessage(ctx, message)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer consumeContext.Stop()

	operationID := uuid.NewString()
	stepID := uuid.NewString()
	resourceName := "fencing-e2e-" + stepID[:8]
	idempotencyKey := "step:" + stepID
	_, err = db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, project_id, environment_id, operation_type,
			status, initiated_by, idempotency_key, total_steps
		) VALUES ($1, 'tenant-fencing', 'project-fencing', 'kind', 'deploy',
			'queued', 'test', $2, 1)`, operationID, "operation:"+operationID)
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := json.Marshal(map[string]string{
		"namespace": "hnb-workloads", "name": resourceName,
		"image": "registry.k8s.io/pause:3.10", "replicas": "1",
	})
	_, err = db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, step_name, step_type, provider_id,
			idempotency_key, step_input, timeout_seconds
		) VALUES ($1, $2, 'deploy', 'deploy', 'kind-provider', $3, $4::jsonb, 20)`,
		stepID, operationID, idempotencyKey, string(inputs))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM outbox_events WHERE operation_id = $1", operationID)
		_, _ = db.Exec("DELETE FROM operation_read_model WHERE operation_id = $1", operationID)
		_, _ = db.Exec("DELETE FROM operations WHERE id = $1", operationID)
	})

	commandPayload, _ := json.Marshal(map[string]any{
		"operationId": operationID, "stepId": stepID, "stepType": "deploy",
		"idempotencyKey": idempotencyKey, "expectedVersion": 0,
	})
	command, _ := json.Marshal(eventEnvelope{
		MessageID: uuid.NewString(), MessageType: stepRequestedSubject, SchemaVersion: "1.0.0",
		OccurredAt: time.Now().UTC(), TenantID: "tenant-fencing", ProjectID: "project-fencing",
		EnvironmentID: "kind", CorrelationID: uuid.NewString(), IdempotencyKey: idempotencyKey,
		OperationID: operationID, StepID: stepID, ExpectedVersion: 0, Payload: commandPayload,
	})
	if _, err := worker.js.Publish(ctx, stepRequestedSubject, command, jetstream.WithMsgID(uuid.NewString())); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(25 * time.Second)
	for {
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM operations WHERE id = $1", operationID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operation did not recover after the first Lease expired")
		}
		time.Sleep(100 * time.Millisecond)
	}

	first := <-received
	second := <-received
	if second.FencingGeneration <= first.FencingGeneration || second.ExecutionAttemptID == first.ExecutionAttemptID {
		t.Fatalf("takeover did not advance the fence: first=%+v second=%+v", first, second)
	}
	var outputs map[string]string
	var outputJSON []byte
	if err := db.QueryRowContext(ctx, "SELECT step_output FROM operation_steps WHERE id = $1", stepID).Scan(&outputJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(outputJSON, &outputs); err != nil {
		t.Fatal(err)
	}
	if outputs["uid"] == "" {
		t.Fatalf("committed output has no Kubernetes UID: %#v", outputs)
	}
	for query, want := range map[string]int{
		"SELECT completed_steps FROM operations WHERE id = $1":                                           1,
		"SELECT count(*) FROM operation_audit WHERE operation_id = $1 AND event_type = 'step_completed'": 1,
		"SELECT count(*) FROM outbox_events WHERE operation_id = $1 AND message_type = $2":               1,
	} {
		var got int
		args := []any{operationID}
		if query[len(query)-2:] == "$2" {
			args = append(args, stepCompletedSubject)
		}
		if err := db.QueryRowContext(ctx, query, args...).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("query %q = %d, want %d", query, got, want)
		}
	}
}

type fixedTokenSource string

func (s fixedTokenSource) Token(context.Context) (string, error) { return string(s), nil }

func (r delayedRunner) Execute(
	ctx context.Context,
	execution engine.ExecutionContext,
) (map[string]string, string, error) {
	select {
	case r.received <- execution:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-time.After(r.delay):
		return map[string]string{"status": "ok"}, "completed", nil
	}
}

func TestSharedConsumerAndOutboxRelay(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	natsURL := os.Getenv("HNB_TEST_NATS_URL")
	if dsn == "" || natsURL == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN and HNB_TEST_NATS_URL are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nc, err := natslib.Connect(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	workerA, err := NewWorker(db, nc, 600*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	workerB, err := NewWorker(db, nc, time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	if workerA.workerID == workerB.workerID {
		t.Fatal("workers unexpectedly share an instance ID")
	}
	if err := workerA.ensureStreams(ctx); err != nil {
		t.Fatal(err)
	}
	consumerA, err := workerA.ensureConsumer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	consumerB, err := workerB.ensureConsumer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	infoA, err := consumerA.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := consumerB.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if infoA.Name != operationConsumer || infoB.Name != operationConsumer {
		t.Fatalf("workers did not bind shared durable consumer: %s, %s", infoA.Name, infoB.Name)
	}
	failedSubscription, err := nc.SubscribeSync("hnb.failed.command.operation.step-requested.v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	consumeContext, err := consumerA.Consume(func(message jetstream.Msg) {
		workerA.handleMessage(ctx, message)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workerA.js.Publish(ctx, stepRequestedSubject, []byte("{"), jetstream.WithMsgID(uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	failedMessage, err := failedSubscription.NextMsg(5 * time.Second)
	consumeContext.Stop()
	if err != nil {
		t.Fatal(err)
	}
	var failedPayload map[string]any
	if err := json.Unmarshal(failedMessage.Data, &failedPayload); err != nil {
		t.Fatal(err)
	}
	if failedPayload["originalSubject"] != stepRequestedSubject {
		t.Fatalf("poison message was not isolated with its source subject: %+v", failedPayload)
	}

	operationID := uuid.NewString()
	stepID := uuid.NewString()
	idempotencyKey := "step:" + stepID
	_, err = db.ExecContext(ctx, `
		INSERT INTO operations (
			id, tenant_id, project_id, environment_id, operation_type,
			status, initiated_by, idempotency_key, total_steps
		) VALUES ($1, 'tenant-test', 'project-test', 'environment-test', 'deploy',
			'queued', 'test', $2, 1)`, operationID, "operation:"+operationID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, step_name, step_type, provider_id,
			idempotency_key, checkpoint, step_input
		) VALUES ($1, $2, 'deploy', 'deploy', 'provider-test', $3, 'resume', $4::jsonb)`,
		stepID, operationID, idempotencyKey, `{"image":"example:v1"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	executedOperationID := operationID
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM outbox_events WHERE operation_id = $1", executedOperationID)
		_, _ = db.Exec("DELETE FROM operation_read_model WHERE operation_id = $1", executedOperationID)
		_, _ = db.Exec("DELETE FROM operations WHERE id = $1", executedOperationID)
	})
	receivedExecution := make(chan engine.ExecutionContext, 1)
	workerA.runner = delayedRunner{delay: 1200 * time.Millisecond, received: receivedExecution}
	consumeContext, err = consumerA.Consume(func(message jetstream.Msg) {
		workerA.handleMessage(ctx, message)
	})
	if err != nil {
		t.Fatal(err)
	}
	correlationID := uuid.NewString()
	commandPayload, err := json.Marshal(map[string]any{
		"operationId":     operationID,
		"stepId":          stepID,
		"stepType":        "deploy",
		"idempotencyKey":  idempotencyKey,
		"expectedVersion": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	command, err := json.Marshal(eventEnvelope{
		MessageID:       uuid.NewString(),
		MessageType:     stepRequestedSubject,
		SchemaVersion:   "1.0.0",
		OccurredAt:      time.Now().UTC(),
		TenantID:        "tenant-test",
		ProjectID:       "project-test",
		EnvironmentID:   "environment-test",
		CorrelationID:   correlationID,
		IdempotencyKey:  idempotencyKey,
		OperationID:     operationID,
		StepID:          stepID,
		ExpectedVersion: 0,
		Payload:         commandPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workerA.js.Publish(ctx, stepRequestedSubject, command, jetstream.WithMsgID(uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status string
		if err := db.QueryRow("SELECT status FROM operation_steps WHERE id = $1", stepID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("long-running step did not commit after lease renewal")
		}
		time.Sleep(50 * time.Millisecond)
	}
	consumeContext.Stop()
	execution := <-receivedExecution
	if execution.ProviderID != "provider-test" || execution.Checkpoint != "resume" || execution.Inputs["image"] != "example:v1" {
		t.Fatalf("runner did not receive authoritative step context: %+v", execution)
	}
	if _, err := uuid.Parse(execution.ExecutionAttemptID); err != nil || execution.FencingGeneration <= 0 {
		t.Fatalf("runner did not receive a valid execution fence: %+v", execution)
	}
	var startedAt, completedAt time.Time
	if err := db.QueryRow(
		"SELECT started_at, completed_at FROM operation_steps WHERE id = $1", stepID,
	).Scan(&startedAt, &completedAt); err != nil {
		t.Fatal(err)
	}
	if completedAt.Sub(startedAt) < time.Second {
		t.Fatalf("step completion time does not include runner duration: %s", completedAt.Sub(startedAt))
	}

	relay, err := NewOutboxRelay(db, nc, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.ensureStream(ctx); err != nil {
		t.Fatal(err)
	}
	subscription, err := nc.SubscribeSync("hnb.>")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	messageID := uuid.NewString()
	operationID = uuid.NewString()
	stepID = uuid.NewString()
	correlationID = uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, tenant_id,
			correlation_id, idempotency_key, aggregate_id, aggregate_version,
			operation_id, step_id, expected_version, payload
		) VALUES ($1, $2, '1.0.0', $2, 'tenant-test', $3, $4, $5::text, 1, $5::uuid, $6, 1, $7::jsonb)`,
		messageID, stepCompletedSubject, correlationID, "relay-test:"+messageID,
		operationID, stepID, `{"operationId":"`+operationID+`","stepId":"`+stepID+`","status":"succeeded"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM outbox_events WHERE message_id = $1", messageID)
	})

	var message *natslib.Msg
	for i := 0; i < 10; i++ {
		published, err := relay.publishOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !published {
			break
		}
		message, err = subscription.NextMsg(5 * time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if message.Header.Get("Nats-Msg-Id") == messageID {
			break
		}
		message = nil
	}
	if message == nil {
		t.Fatal("relay did not publish the test outbox event")
	}
	var envelope eventEnvelope
	if err := json.Unmarshal(message.Data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.MessageID != messageID || envelope.OperationID != operationID || envelope.StepID != stepID {
		t.Fatalf("unexpected published envelope: %+v", envelope)
	}
	var status string
	if err := db.QueryRow("SELECT status FROM outbox_events WHERE message_id = $1", messageID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("outbox status=%s, want published", status)
	}
}
