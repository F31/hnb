package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/kms"
	"github.com/nats-io/nats.go"
)

// Request is the payload extension-controller publishes on
// hnb.extension.provider.<provider>.<action>.
type Request struct {
	ExtensionID string `json:"extension_id,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	PrevVersion string `json:"prev_version,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Action      string `json:"action,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
}

// Response is the reply extension-controller awaits; status must be
// "succeeded" for the extension to reach a ready/uninstalled state.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Worker struct {
	db            *sql.DB
	cipher        kms.Decrypter
	nc            *nats.Conn
	helmPath      string
	kubeconfigDir string
}

// New builds a worker. cipher may be nil when no master key is configured, in
// which case installs fail closed with a clear error.
func New(db *sql.DB, cipher kms.Decrypter, nc *nats.Conn, helmPath, kubeconfigDir string) *Worker {
	return &Worker{
		db:            db,
		cipher:        cipher,
		nc:            nc,
		helmPath:      helmPath,
		kubeconfigDir: kubeconfigDir,
	}
}

// Start subscribes to the extension provider lifecycle subjects.
func (w *Worker) Start(ctx context.Context) error {
	if w.cipher == nil {
		log.Printf("[plugin-worker] WARNING: no HNB_MASTER_KEY configured; plugin installs will fail closed")
	}
	subs := map[string]func(*nats.Msg){
		"hnb.extension.provider.*.install":   w.handle,
		"hnb.extension.provider.*.upgrade":   w.handle,
		"hnb.extension.provider.*.uninstall": w.handle,
		"hnb.extension.provider.*.health":    w.handle,
	}
	for subject := range subs {
		s := subject
		if _, err := w.nc.Subscribe(s, func(msg *nats.Msg) {
			w.handle(msg)
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", s, err)
		}
		log.Printf("[plugin-worker] subscribed to %s", s)
	}
	<-ctx.Done()
	return nil
}

func (w *Worker) handle(msg *nats.Msg) {
	var req Request
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		w.respond(msg, Response{Status: "failed", Message: fmt.Sprintf("invalid request: %v", err)})
		return
	}
	log.Printf("[plugin-worker] %s/%s provider=%s target=%s version=%s", req.Action, req.Name, req.Provider, req.TargetID, req.Version)

	var resp Response
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	switch req.Action {
	case "install", "upgrade":
		resp = w.Install(ctx, &req)
	case "uninstall":
		resp = w.Uninstall(ctx, &req)
	case "health":
		resp = w.Health(ctx, &req)
	default:
		resp = Response{Status: "failed", Message: fmt.Sprintf("unsupported action %q", req.Action)}
	}
	if resp.Status != "succeeded" {
		log.Printf("[plugin-worker] %s/%s failed: %s", req.Action, req.Name, resp.Message)
	}
	w.respond(msg, resp)
}

// kubeconfigFor resolves the target cluster's kubeconfig via its credential_ref
// (or the tenant's most recent kubeconfig secret) and returns a file path
// containing the plaintext kubeconfig.
func (w *Worker) kubeconfigFor(targetID string) (string, error) {
	if w.cipher == nil {
		return "", fmt.Errorf("master key not configured")
	}
	var tenantID, scope, name string
	var encrypted, candidate string
	// Exact credential_ref preferred (same semantics as platform-api
	// KubeConfigEncryptedForRef); legacy targets fall back to the tenant's
	// most recent kubeconfig secret.
	err := w.db.QueryRow(`
		SELECT rt.tenant_id,
		       COALESCE(rt.credential_ref->>'scope','') ,
		       COALESCE(rt.credential_ref->>'name','')
		FROM runtime_targets rt WHERE rt.id = $1::uuid`, targetID).Scan(&tenantID, &scope, &name)
	if err != nil {
		return "", fmt.Errorf("load target: %w", err)
	}
	if scope != "" && name != "" {
		err = w.db.QueryRow(`
			SELECT sr.encrypted_value FROM secret_references sr
			JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
			WHERE sr.tenant_id = $1 AND sr.scope = $2 AND sr.name = $3
			  AND sr.purpose = 'kubeconfig' AND sr.is_active
			ORDER BY sr.created_at DESC LIMIT 1`, tenantID, scope, name).Scan(&encrypted)
		if err == nil {
			return w.writeKubeconfig(targetID, encrypted)
		}
		log.Printf("[plugin-worker] credential_ref secret missing (%s/%s): %v", scope, name, err)
	}
	err = w.db.QueryRow(`
		SELECT sr.encrypted_value FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1 AND sr.purpose = 'kubeconfig'
		  AND sr.is_active
		ORDER BY sr.created_at DESC LIMIT 1`, tenantID).Scan(&candidate)
	if err != nil {
		return "", fmt.Errorf("no kubeconfig secret for tenant %s: %w", tenantID, err)
	}
	return w.writeKubeconfig(targetID, candidate)
}

func (w *Worker) writeKubeconfig(targetID, sealed string) (string, error) {
	decrypted, err := w.cipher.Decrypt(sealed)
	if err != nil {
		return "", fmt.Errorf("decrypt kubeconfig: %w", err)
	}
	if err := os.MkdirAll(w.kubeconfigDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(w.kubeconfigDir, "kubeconfig-"+targetID+".yaml")
	if err := os.WriteFile(path, decrypted, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Worker) Install(ctx context.Context, req *Request) Response {
	kubeconfig, err := w.kubeconfigFor(req.TargetID)
	if err != nil {
		return Response{Status: "failed", Message: err.Error()}
	}
	out, err := helmRun(ctx, w.helmPath, kubeconfig, "upgrade", "--install", req.Name,
		chartName(req.Name),
		"--namespace", namespaceFor(req.Name),
		"--version", req.Version,
		"--wait", "--timeout", "15m")
	if err != nil {
		return Response{Status: "failed", Message: fmt.Sprintf("helm install: %v (%s)", err, truncate(out))}
	}
	return Response{Status: "succeeded"}
}

func (w *Worker) Uninstall(ctx context.Context, req *Request) Response {
	kubeconfig, err := w.kubeconfigFor(req.TargetID)
	if err != nil {
		return Response{Status: "failed", Message: err.Error()}
	}
	out, err := helmRun(ctx, w.helmPath, kubeconfig, "uninstall", req.Name,
		"--namespace", namespaceFor(req.Name))
	if err != nil {
		// A release that is absent is already uninstalled: treat as success.
		if strings.Contains(err.Error(), "not found") || strings.Contains(out, "not found") {
			return Response{Status: "succeeded", Message: "release not found (already uninstalled)"}
		}
		return Response{Status: "failed", Message: fmt.Sprintf("helm uninstall: %v (%s)", err, truncate(out))}
	}
	return Response{Status: "succeeded"}
}

func (w *Worker) Health(ctx context.Context, req *Request) Response {
	kubeconfig, err := w.kubeconfigFor(req.TargetID)
	if err != nil {
		return Response{Status: "failed", Message: err.Error()}
	}
	out, err := helmRun(ctx, w.helmPath, kubeconfig, "status", req.Name,
		"--namespace", namespaceFor(req.Name))
	if err != nil {
		return Response{Status: "failed", Message: fmt.Sprintf("helm status: %v (%s)", err, truncate(out))}
	}
	if !strings.Contains(out, "deployed") {
		return Response{Status: "failed", Message: "release not deployed"}
	}
	return Response{Status: "succeeded"}
}

func helmRun(ctx context.Context, helmPath, kubeconfig string, args ...string) (string, error) {
	full := []string{"--kubeconfig", kubeconfig}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, helmPath, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String() + stderr.String(), err
	}
	return stdout.String() + stderr.String(), nil
}

func (w *Worker) respond(msg *nats.Msg, resp Response) {
	data, _ := json.Marshal(resp)
	if err := msg.Respond(data); err != nil {
		log.Printf("[plugin-worker] respond error: %v", err)
	}
}

func chartName(name string) string {
	// The platform catalog uses product keys that map to upstream helm charts.
	switch name {
	case "cilium":
		return "cilium/cilium"
	case "calico":
		return "projectcalico/tigera-operator"
	case "kube-ovn":
		return "kubeovn/kube-ovn"
	case "prometheus-operator":
		return "prometheus-community/kube-prometheus-stack"
	case "kubeedge":
		return "kubeedge/cloudcore"
	case "rook-ceph":
		return "rook-release/rook-ceph"
	case "longhorn":
		return "longhorn/longhorn"
	case "falco":
		return "falcosecurity/falco"
	case "keda":
		return "kedacore/keda"
	case "karmada":
		return "karmada/karmada-operator"
	case "hami":
		return "projecthami/hami"
	case "gpu-operator":
		return "nvidia/gpu-operator"
	default:
		return name + "/" + name
	}
}

func namespaceFor(name string) string {
	switch name {
	case "cilium", "calico", "kube-ovn":
		return "kube-system"
	case "longhorn":
		return "longhorn-system"
	case "falco":
		return "falco"
	case "keda":
		return "keda"
	case "rook-ceph":
		return "rook-ceph"
	default:
		return "kp-default"
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}
