package nats

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/F31/hnb/cmd/network-provider/internal/provider"
)

type fakeMsg struct {
	data    []byte
	subject string
	reply   string
	acked   bool
	naked   bool
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) { return nil, nil }
func (m *fakeMsg) Data() []byte                              { return m.data }
func (m *fakeMsg) Headers() natslib.Header                   { return nil }
func (m *fakeMsg) Subject() string                           { return m.subject }
func (m *fakeMsg) Reply() string                             { return m.reply }
func (m *fakeMsg) Ack() error                                { m.acked = true; return nil }
func (m *fakeMsg) DoubleAck(context.Context) error           { m.acked = true; return nil }
func (m *fakeMsg) Nak() error                                { m.naked = true; return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error          { m.naked = true; return nil }
func (m *fakeMsg) InProgress() error                         { return nil }
func (m *fakeMsg) Term() error                               { return nil }
func (m *fakeMsg) TermWithReason(string) error               { return nil }

type publishedMessage struct {
	subject string
	payload []byte
}

type fakePublisher struct {
	published []publishedMessage
}

func (p *fakePublisher) Publish(_ context.Context, subject string, payload []byte, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	p.published = append(p.published, publishedMessage{subject: subject, payload: payload})
	return &jetstream.PubAck{}, nil
}

type fakeReplier struct {
	published []publishedMessage
}

func (r *fakeReplier) Publish(subj string, data []byte) error {
	r.published = append(r.published, publishedMessage{subject: subj, payload: data})
	return nil
}

type fakeResolver struct {
	kubeconfig string
	err        error
	calls      []string
}

func (r *fakeResolver) KubeconfigFor(_ context.Context, targetID string) (string, error) {
	r.calls = append(r.calls, targetID)
	if r.err != nil {
		return "", r.err
	}
	return r.kubeconfig, nil
}

type fakeProvider struct {
	name   string
	target *provider.RuntimeTarget
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Install(_ context.Context, _ *provider.NetworkProfile, target *provider.RuntimeTarget) error {
	p.target = target
	return nil
}
func (p *fakeProvider) Uninstall(_ context.Context, _ *provider.NetworkProfile, target *provider.RuntimeTarget) error {
	p.target = target
	return nil
}
func (p *fakeProvider) Upgrade(_ context.Context, _ *provider.NetworkProfile, target *provider.RuntimeTarget, _ string) error {
	p.target = target
	return nil
}
func (p *fakeProvider) Capability() provider.NetworkCapability {
	return provider.NetworkCapability{ProviderName: p.name}
}
func (p *fakeProvider) Health(_ context.Context, target *provider.RuntimeTarget) error {
	p.target = target
	return nil
}

func newTestWorker(pub *fakePublisher, creds *fakeResolver, prov *fakeProvider) *Worker {
	return &Worker{
		publisher:   pub,
		replier:     &fakeReplier{},
		providers:   map[string]provider.NetworkProvider{prov.name: prov},
		credentials: creds,
		workerID:    "test-worker",
	}
}

func requestMessage(t *testing.T, req NetworkRequestMessage) *fakeMsg {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return &fakeMsg{data: data, subject: "hnb.network." + req.Provider + "." + req.Action}
}

func lastResult(t *testing.T, pub *fakePublisher) NetworkResultMessage {
	t.Helper()
	if len(pub.published) == 0 {
		t.Fatal("no result published")
	}
	var res NetworkResultMessage
	if err := json.Unmarshal(pub.published[len(pub.published)-1].payload, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return res
}

func TestHandleMessageInjectsResolvedKubeconfig(t *testing.T) {
	pub := &fakePublisher{}
	creds := &fakeResolver{kubeconfig: "/tmp/hnb-network-provider/kubeconfig-target-1.yaml"}
	prov := &fakeProvider{name: "cilium"}
	w := newTestWorker(pub, creds, prov)

	msg := requestMessage(t, NetworkRequestMessage{
		OperationID: "op-1",
		Action:      "install",
		Provider:    "cilium",
		TargetID:    "target-1",
	})
	w.handleMessage(context.Background(), msg)

	if !msg.acked || msg.naked {
		t.Fatalf("expected ack, got acked=%v naked=%v", msg.acked, msg.naked)
	}
	if len(creds.calls) != 1 || creds.calls[0] != "target-1" {
		t.Fatalf("resolver calls = %v, want [target-1]", creds.calls)
	}
	if prov.target == nil {
		t.Fatal("provider was not called")
	}
	if prov.target.ID != "target-1" {
		t.Fatalf("target.ID = %q, want target-1", prov.target.ID)
	}
	if prov.target.Kubeconfig != creds.kubeconfig {
		t.Fatalf("target.Kubeconfig = %q, want %q", prov.target.Kubeconfig, creds.kubeconfig)
	}
	if res := lastResult(t, pub); res.Status != "succeeded" {
		t.Fatalf("result status = %q, want succeeded (error=%q)", res.Status, res.Error)
	}
}

func TestHandleMessageFailsClosedWhenCredentialsUnresolvable(t *testing.T) {
	pub := &fakePublisher{}
	creds := &fakeResolver{err: errors.New("no kubeconfig secret for tenant tenant-1")}
	prov := &fakeProvider{name: "cilium"}
	w := newTestWorker(pub, creds, prov)

	msg := requestMessage(t, NetworkRequestMessage{
		OperationID: "op-2",
		Action:      "install",
		Provider:    "cilium",
		TargetID:    "target-1",
	})
	w.handleMessage(context.Background(), msg)

	if prov.target != nil {
		t.Fatal("provider must not be called when credentials cannot be resolved")
	}
	// Permanent failure: ack (no redelivery), but the caller gets a clear
	// failed result instead of a silent fallback to the local kube context.
	if !msg.acked || msg.naked {
		t.Fatalf("expected ack without nak, got acked=%v naked=%v", msg.acked, msg.naked)
	}
	res := lastResult(t, pub)
	if res.Status != "failed" {
		t.Fatalf("result status = %q, want failed", res.Status)
	}
	if !strings.Contains(res.Error, "resolve credentials for target target-1") {
		t.Fatalf("result error = %q, want it to name the target and credential failure", res.Error)
	}
}

func TestHandleMessageFailsClosedWhenTargetIDMissing(t *testing.T) {
	pub := &fakePublisher{}
	creds := &fakeResolver{kubeconfig: "/unused"}
	prov := &fakeProvider{name: "cilium"}
	w := newTestWorker(pub, creds, prov)

	msg := requestMessage(t, NetworkRequestMessage{
		OperationID: "op-3",
		Action:      "health",
		Provider:    "cilium",
	})
	w.handleMessage(context.Background(), msg)

	if prov.target != nil {
		t.Fatal("provider must not be called without a target_id")
	}
	if len(creds.calls) != 0 {
		t.Fatalf("resolver must not be called without a target_id, calls = %v", creds.calls)
	}
	res := lastResult(t, pub)
	if res.Status != "failed" || !strings.Contains(res.Error, "missing target_id") {
		t.Fatalf("result = %+v, want failed with missing target_id error", res)
	}
}

func TestNewWorkerRequiresCredentialResolver(t *testing.T) {
	if _, err := NewWorker(&natslib.Conn{}, nil, nil); err == nil {
		t.Fatal("expected error when credential resolver is nil")
	}
}

// The reconciler issues health checks as core NATS requests and awaits the
// reply inbox; the worker must answer msg.Reply() in addition to publishing
// the audit result on <subject>.result.
func TestHandleMessageAnswersReplySubject(t *testing.T) {
	pub := &fakePublisher{}
	replier := &fakeReplier{}
	creds := &fakeResolver{kubeconfig: "/tmp/hnb-network-provider/kubeconfig-target-1.yaml"}
	prov := &fakeProvider{name: "cilium"}
	w := &Worker{
		publisher:   pub,
		replier:     replier,
		providers:   map[string]provider.NetworkProvider{prov.name: prov},
		credentials: creds,
		workerID:    "test-worker",
	}

	msg := requestMessage(t, NetworkRequestMessage{
		OperationID: "op-4",
		Action:      "health",
		Provider:    "cilium",
		TargetID:    "target-1",
	})
	msg.reply = "_INBOX.abc"
	w.handleMessage(context.Background(), msg)

	if !msg.acked || msg.naked {
		t.Fatalf("expected ack, got acked=%v naked=%v", msg.acked, msg.naked)
	}
	if res := lastResult(t, pub); res.Status != "succeeded" {
		t.Fatalf("audit result status = %q, want succeeded", res.Status)
	}
	if len(replier.published) != 1 || replier.published[0].subject != "_INBOX.abc" {
		t.Fatalf("reply publishes = %+v, want one message on _INBOX.abc", replier.published)
	}
	var replyRes NetworkResultMessage
	if err := json.Unmarshal(replier.published[0].payload, &replyRes); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}
	if replyRes.Status != "succeeded" {
		t.Fatalf("reply status = %q, want succeeded", replyRes.Status)
	}
}
