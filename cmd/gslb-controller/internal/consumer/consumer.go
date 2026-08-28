package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/gslb-controller/internal/store"
	"github.com/F31/hnb/pkg/gslb"
)

const (
	domainEventStream = "domain-events"
	// commandSubject 是执行命令 subject：apiserver 审批通过后经
	// Transactional Outbox + relay 投递到 domain-events 流（GSLB-005）。
	commandSubject = "hnb.event.gslb.step-requested.v1"
	consumerName   = "gslb-controller"
)

var errRejected = errors.New("gslb command rejected: request not in executable state")

// RequestStore 是消费者依赖的请求存储（接口便于测试）。
type RequestStore interface {
	GetRequest(ctx context.Context, id string) (store.SwitchRequest, bool, error)
	Transition(ctx context.Context, id string, from []string, to string, errorMsg string) error
	EmitStatusChanged(ctx context.Context, request store.SwitchRequest, status, errorMsg string) error
}

// PlanExecutor 执行不可变计划的 DNS 步骤。
type PlanExecutor interface {
	ExecutePlan(ctx context.Context, plan *gslb.Plan) error
}

// Consumer 消费执行命令并驱动 executor。这是 gslb-controller 中
// 唯一允许触达 DNS 数据面的入口（GSLB-005 无旁路）。
type Consumer struct {
	js    jetstream.JetStream
	store RequestStore
	exec  PlanExecutor
}

func New(js jetstream.JetStream, requestStore RequestStore, planExecutor PlanExecutor) *Consumer {
	return &Consumer{js: js, store: requestStore, exec: planExecutor}
}

// Start 建立 durable 消费者并循环处理命令。
func (c *Consumer) Start(ctx context.Context) error {
	if _, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       domainEventStream,
		Subjects:   []string{"hnb.event.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   5 << 30,
		MaxMsgSize: 2 << 20,
		Discard:    jetstream.DiscardOld,
		Duplicates: 2 * time.Minute,
	}); err != nil {
		return fmt.Errorf("ensure domain-event stream: %w", err)
	}
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, domainEventStream, jetstream.ConsumerConfig{
		Name:           consumerName,
		Durable:        consumerName,
		FilterSubject:  commandSubject,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     5,
		AckWait:        time.Minute,
		MaxAckPending:  1024,
	})
	if err != nil {
		return fmt.Errorf("create gslb consumer: %w", err)
	}

	consumeContext, err := consumer.Consume(func(msg jetstream.Msg) {
		if err := c.HandleCommand(ctx, msg.Data()); err != nil {
			if errors.Is(err, errRejected) {
				log.Printf("[gslb-consumer] %v", err)
			} else {
				log.Printf("[gslb-consumer] command failed: %v", err)
			}
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("consume gslb commands: %w", err)
	}
	log.Printf("[gslb-consumer] consuming %s", commandSubject)
	<-ctx.Done()
	consumeContext.Stop()
	return nil
}

type commandPayload struct {
	RequestID string          `json:"requestId"`
	ServiceID string          `json:"serviceId"`
	Plan      json.RawMessage `json:"plan"`
}

// HandleCommand 处理一条执行命令（可独立测试）。
func (c *Consumer) HandleCommand(ctx context.Context, data []byte) error {
	var payload commandPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("parse gslb command: %w", err)
	}
	if payload.RequestID == "" || len(payload.Plan) == 0 {
		return errors.New("gslb command missing requestId/plan")
	}

	request, ok, err := c.store.GetRequest(ctx, payload.RequestID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: request %s not found", errRejected, payload.RequestID)
	}
	// fail-closed：只有 Approved/Dispatched 状态可执行
	if request.Status != store.RequestStatusApproved && request.Status != store.RequestStatusDispatched {
		return fmt.Errorf("%w: request %s status %s", errRejected, payload.RequestID, request.Status)
	}

	// 首次执行：Approved → Dispatched（并发下只有一方成功，输者忽略）
	if request.Status == store.RequestStatusApproved {
		if err := c.store.Transition(ctx, payload.RequestID, []string{store.RequestStatusApproved}, store.RequestStatusDispatched, ""); err != nil {
			return fmt.Errorf("%w: concurrent dispatch", errRejected)
		}
	}

	var plan gslb.Plan
	if err := json.Unmarshal(payload.Plan, &plan); err != nil {
		_ = c.store.Transition(ctx, payload.RequestID, []string{store.RequestStatusDispatched}, store.RequestStatusFailed, "invalid plan payload")
		return err
	}

	if err := c.exec.ExecutePlan(ctx, &plan); err != nil {
		_ = c.store.Transition(ctx, payload.RequestID, []string{store.RequestStatusDispatched}, store.RequestStatusFailed, err.Error())
		_ = c.store.EmitStatusChanged(ctx, request, store.RequestStatusFailed, err.Error())
		return err
	}
	if err := c.store.Transition(ctx, payload.RequestID, []string{store.RequestStatusDispatched}, store.RequestStatusSucceeded, ""); err != nil {
		return err
	}
	return c.store.EmitStatusChanged(ctx, request, store.RequestStatusSucceeded, "")
}

// Connect 建立 NATS 连接与 JetStream 上下文。
func Connect(ctx context.Context, url string) (jetstream.JetStream, func(), error) {
	nc, err := nats.Connect(url,
		nats.Name("gslb-controller"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, nil, err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, err
	}
	cleanup := func() { nc.Close() }
	go func() {
		<-ctx.Done()
		nc.Close()
	}()
	return js, cleanup, nil
}
