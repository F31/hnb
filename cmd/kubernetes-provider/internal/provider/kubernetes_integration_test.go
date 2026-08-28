package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestKubernetesProviderE2E(t *testing.T) {
	kubeconfig := os.Getenv("HNB_TEST_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("HNB_TEST_KUBECONFIG is not set")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	const namespace = "hnb-e2e"
	createdNamespace := false
	if _, err := client.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{}); err == nil {
		createdNamespace = true
	} else if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	if createdNamespace {
		t.Cleanup(func() {
			_ = client.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		})
	}
	name := fmt.Sprintf("provider-e2e-%d", time.Now().UnixNano())
	executor := NewExecutor(client, map[string]struct{}{namespace: {}}, 2)
	server := httptest.NewServer(NewHandler(executor))
	defer server.Close()
	defer func() {
		_ = client.AppsV1().Deployments(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	deploy := ExecutionContext{
		StepID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222", TenantID: "e2e-tenant",
		StepType: "deploy", ProviderID: "kind-provider", IdempotencyKey: "e2e-idempotency",
		ExecutionAttemptID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", FencingGeneration: 1,
		Inputs: map[string]any{"namespace": namespace, "name": name, "image": "registry.k8s.io/pause:3.10", "replicas": "1"},
	}
	first := postExecution(t, server.URL, deploy)
	if first.code != http.StatusOK || first.body.Status != "succeeded" {
		t.Fatalf("create: HTTP %d %#v", first.code, first.body)
	}
	uid, _ := first.body.Outputs["uid"].(string)
	second := postExecution(t, server.URL, deploy)
	if second.code != http.StatusOK || second.body.Outputs["uid"] != uid {
		t.Fatalf("replay: HTTP %d %#v", second.code, second.body)
	}

	stale := deploy
	stale.ExecutionAttemptID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"
	conflict := postExecution(t, server.URL, stale)
	if conflict.code != http.StatusConflict || conflict.body.ErrorCode != ErrorResourceConflict {
		t.Fatalf("conflict: HTTP %d %#v", conflict.code, conflict.body)
	}

	remove := ExecutionContext{
		StepID: "33333333-3333-4333-8333-333333333333", OperationID: deploy.OperationID, TenantID: deploy.TenantID,
		StepType: "delete", ProviderID: deploy.ProviderID, IdempotencyKey: "e2e-delete",
		ExecutionAttemptID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2", FencingGeneration: 2,
		Inputs: map[string]any{"namespace": namespace, "name": name, "expected_uid": uid},
	}
	deleted := postExecution(t, server.URL, remove)
	if deleted.code != http.StatusOK || deleted.body.Outputs["action"] != "deleted" {
		t.Fatalf("delete: HTTP %d %#v", deleted.code, deleted.body)
	}
	tombstone, err := client.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil || tombstone.Spec.Replicas == nil || *tombstone.Spec.Replicas != 0 || tombstone.Annotations[lastActionAnnotation] != "delete" {
		t.Fatalf("logical tombstone: deployment=%#v err=%v", tombstone, err)
	}
	staleAfterDelete := postExecution(t, server.URL, deploy)
	if staleAfterDelete.code != http.StatusConflict || staleAfterDelete.body.ErrorCode != ErrorFenced {
		t.Fatalf("stale after delete: HTTP %d %#v", staleAfterDelete.code, staleAfterDelete.body)
	}
	higherDeploy := deploy
	higherDeploy.ExecutionAttemptID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"
	higherDeploy.FencingGeneration = 3
	resurrection := postExecution(t, server.URL, higherDeploy)
	if resurrection.code != http.StatusConflict || resurrection.body.ErrorCode != ErrorResourceConflict {
		t.Fatalf("resurrection: HTTP %d %#v", resurrection.code, resurrection.body)
	}
}

func postExecution(t *testing.T, baseURL string, execution ExecutionContext) httpResult {
	t.Helper()
	payload, err := json.Marshal(executeRequest{SchemaVersion: contractVersion, Execution: wireExecution(execution)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/v2/steps:execute", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer response.Body.Close()
	var body executeResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return httpResult{code: response.StatusCode, body: body}
}
