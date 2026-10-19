// Lab-only provider action runner; not part of the production binary.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/F31/hnb/cmd/kubernetes-provider/internal/provider"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	step := flag.String("step", "", "deploy|delete|service.deploy|pvc.deploy")
	namespace := flag.String("namespace", "hnb-app", "target namespace")
	name := flag.String("name", "", "resource name")
	image := flag.String("image", "", "workload image")
	replicas := flag.String("replicas", "1", "workload replicas")
	pvcName := flag.String("pvc-name", "", "PVC name")
	pvcPath := flag.String("pvc-path", "", "PVC mount path")
	secretName := flag.String("secret-name", "", "Secret name")
	secretPath := flag.String("secret-path", "", "Secret mount path")
	storageClass := flag.String("storage-class", "", "PVC storage class")
	size := flag.String("size", "", "PVC size")
	workload := flag.String("workload", "", "Service workload")
	port := flag.String("port", "", "Service port")
	sourceImage := flag.String("source-image", "", "upgrade source image")
	targetImage := flag.String("target-image", "", "upgrade target image")
	expectedUID := flag.String("expected-uid", "", "expected resource UID")
	dataCompatible := flag.String("data-compatible", "", "upgrade data compatibility")
	dataIrreversible := flag.String("data-irreversible", "", "rollback irreversible data flag")
	dataCleanupAuthorized := flag.String("data-cleanup-authorized", "", "explicit PVC data cleanup authorization")
	backupRef := flag.String("backup-ref", "", "upgrade backup reference")
	generation := flag.Int64("generation", 1, "fencing generation")
	attempt := flag.String("attempt", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "execution attempt UUID")
	flag.Parse()
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		panic(err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	inputs := map[string]any{"namespace": *namespace, "name": *name}
	for key, value := range map[string]string{
		"image": *image, "pvc_name": *pvcName, "pvc_mount_path": *pvcPath,
		"secret_name": *secretName, "secret_mount_path": *secretPath,
		"storage_class": *storageClass, "size": *size, "workload": *workload, "port": *port,
	} {
		if value != "" {
			inputs[key] = value
		}
	}
	if *step == "deploy" {
		inputs["replicas"] = *replicas
		if *expectedUID != "" {
			inputs["expected_uid"] = *expectedUID
		}
	}
	if *step == "delete" && *expectedUID != "" {
		inputs["expected_uid"] = *expectedUID
	}
	if *step == "workload.upgrade" {
		inputs["source_image"] = *sourceImage
		inputs["target_image"] = *targetImage
		inputs["expected_uid"] = *expectedUID
		inputs["data_compatible"] = *dataCompatible
		inputs["backup_ref"] = *backupRef
	}
	if *step == "workload.rollback" {
		inputs["source_image"] = *sourceImage
		inputs["target_image"] = *targetImage
		inputs["expected_uid"] = *expectedUID
		inputs["data_irreversible"] = *dataIrreversible
		inputs["backup_ref"] = *backupRef
	}
	if *step == "pvc.delete" {
		inputs["expected_uid"] = *expectedUID
		inputs["data_cleanup_authorized"] = *dataCleanupAuthorized
	}
	execution := provider.ExecutionContext{
		StepID: "11111111-1111-4111-8111-111111111111", OperationID: "22222222-2222-4222-8222-222222222222",
		TenantID: "default", StepType: *step, Inputs: inputs, ProviderID: "kubernetes-provider",
		IdempotencyKey: "lab-4.4-" + *name, ExecutionAttemptID: *attempt, FencingGeneration: *generation,
	}
	executor := provider.NewExecutor(client, map[string]struct{}{*namespace: {}}, 4)
	result, err := executor.Execute(context.Background(), execution)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result.Outputs)
}
