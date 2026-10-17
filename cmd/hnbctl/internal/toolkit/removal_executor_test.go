package toolkit

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type removalRunner struct {
	calls   [][]string
	configs string
	pods    string
}

func (r *removalRunner) Run(_ context.Context, _ string, _ io.Reader, args ...string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	switch {
	case len(args) >= 2 && args[0] == "get" && args[1] == "configmap":
		return r.configs, nil
	case len(args) >= 2 && args[0] == "get" && args[1] == "pods":
		return r.pods, nil
	default:
		return "", nil
	}
}

func TestRemovalExecutorDeletesWorkloadsAndRetainsData(t *testing.T) {
	runner := &removalRunner{configs: "configmap/nats-config\nconfigmap/kube-root-ca.crt\n"}
	executor := RemovalExecutor{Kubeconfig: "/kc", Namespace: "hnb-system", Selector: "app.kubernetes.io/instance=hnb", Runner: runner}
	plan := RemovalPlan{Mode: RemovalUninstallHNB, Authorized: true, Retain: []string{"persistentvolumeclaim/postgresql-data"}}
	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified {
		t.Fatal("execution was not verified")
	}
	joined := ""
	for _, call := range runner.calls {
		joined += strings.Join(call, " ") + "\n"
	}
	for _, want := range []string{
		"delete deployment,statefulset,daemonset,service,serviceaccount,role,rolebinding --all --namespace hnb-system",
		"delete --namespace hnb-system --ignore-not-found=true --wait=true --timeout=2m0s configmap/nats-config",
		"delete clusterrole -l app.kubernetes.io/instance=hnb",
		"delete clusterrolebinding -l app.kubernetes.io/instance=hnb",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"delete pvc", "delete persistentvolumeclaim", "delete secret", "delete namespace", "kube-root-ca.crt"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("executor touched retained resource %q:\n%s", forbidden, joined)
		}
	}
	if len(result.Retained) == 0 {
		t.Fatal("retained resources not reported")
	}
}

func TestRemovalExecutorRefusesNonLocalModesAndIncompletePods(t *testing.T) {
	runner := &removalRunner{pods: "pod/leftover"}
	executor := RemovalExecutor{Kubeconfig: "/kc", Namespace: "hnb-system", Runner: runner, Timeout: 20 * time.Millisecond}
	if _, err := executor.Execute(context.Background(), RemovalPlan{Mode: RemovalUnmanage, Authorized: true}); err == nil || !strings.Contains(err.Error(), "REMOVAL_MODE_REQUIRES_PLATFORM") {
		t.Fatalf("unmanage executed locally: %v", err)
	}
	if _, err := executor.Execute(context.Background(), RemovalPlan{Mode: RemovalUninstallHNB, Authorized: false}); err == nil || !strings.Contains(err.Error(), "REMOVAL_NOT_AUTHORIZED") {
		t.Fatalf("unauthorized plan executed: %v", err)
	}
	if _, err := executor.Execute(context.Background(), RemovalPlan{Mode: RemovalUninstallHNB, Authorized: true}); err == nil || !strings.Contains(err.Error(), "REMOVAL_INCOMPLETE") {
		t.Fatalf("leftover pods reported success: %v", err)
	}
}
