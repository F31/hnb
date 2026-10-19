package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func runApplication(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "application subcommand required: plan|status|delete-preview|deploy|upgrade|rollback|delete")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("application "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var baseURL, tokenFile, tenant, format, deliveryID, compositionFile string
	var idempotencyKey, image, sourceImage, targetImage, expectedUID, backupRef string
	var pvcMountPath, secretName, secretMountPath string
	var dataCompatible, dataIrreversible, approve bool
	var timeout time.Duration
	fs.StringVar(&baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&tokenFile, "token-file", "", "file holding the API bearer token")
	fs.StringVar(&tenant, "tenant-scope", "", "expected authenticated tenant")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.StringVar(&deliveryID, "delivery-id", "", "application delivery id")
	fs.StringVar(&compositionFile, "composition-file", "", "application composition JSON file (plan)")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an application action")
	fs.StringVar(&image, "image", "", "deploy image (digest-pinned)")
	fs.StringVar(&sourceImage, "source-image", "", "upgrade/rollback source image")
	fs.StringVar(&targetImage, "target-image", "", "upgrade/rollback target image")
	fs.StringVar(&expectedUID, "expected-uid", "", "expected workload UID")
	fs.StringVar(&backupRef, "backup-ref", "", "upgrade/rollback backup reference")
	fs.StringVar(&pvcMountPath, "pvc-mount-path", "", "deploy PVC mount path")
	fs.StringVar(&secretName, "secret-name", "", "deploy Secret name")
	fs.StringVar(&secretMountPath, "secret-mount-path", "", "deploy Secret mount path")
	fs.BoolVar(&dataCompatible, "data-compatible", false, "upgrade declares PVC data compatibility")
	fs.BoolVar(&dataIrreversible, "data-irreversible", false, "rollback declares irreversible data migration")
	fs.BoolVar(&approve, "approve", false, "approve a high-risk Operation immediately")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(rest); err != nil || fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "invalid application flags")
	}
	switch sub {
	case "plan":
		if compositionFile == "" {
			return fail(ReasonInvalidConfig, "application plan requires --composition-file")
		}
	case "status", "delete-preview":
		if deliveryID == "" {
			return fail(ReasonInvalidConfig, "application "+sub+" requires --delivery-id")
		}
	case "deploy", "upgrade", "rollback", "delete":
		if deliveryID == "" || idempotencyKey == "" {
			return fail(ReasonInvalidConfig, "application "+sub+" requires --delivery-id and --idempotency-key")
		}
	default:
		return fail(ReasonInvalidConfig, "unknown application subcommand: "+sub)
	}
	client, err := newPlatformClient(clusterFlags{baseURL: baseURL, tokenFile: tokenFile, timeout: timeout})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if tenant != "" && client.TokenTenantID() != tenant {
		return fail(ReasonInvalidConfig, "tenant scope does not match the token tenant")
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var data json.RawMessage
	switch sub {
	case "plan":
		body, err := os.ReadFile(compositionFile)
		if err != nil || len(body) == 0 || len(body) > 1<<20 {
			return fail(ReasonInvalidConfig, "composition file must be 1..1048576 bytes")
		}
		data, err = client.ApplicationData(requestCtx, "POST", "/api/v1/application-plans:preview", body)
		if err != nil {
			return platformResult(err)
		}
	case "status":
		data, err = client.ApplicationData(requestCtx, "GET", "/api/v1/application-deliveries/"+deliveryID+"/readiness", nil)
		if err != nil {
			return platformResult(err)
		}
	case "delete-preview":
		data, err = client.ApplicationData(requestCtx, "GET", "/api/v1/application-deliveries/"+deliveryID+"/deletion-preview", nil)
		if err != nil {
			return platformResult(err)
		}
	case "deploy", "upgrade", "rollback", "delete":
		body, _ := json.Marshal(map[string]any{
			"action": sub, "idempotencyKey": idempotencyKey, "image": image,
			"sourceImage": sourceImage, "targetImage": targetImage,
			"expectedUid": expectedUID, "backupRef": backupRef,
			"dataCompatible": dataCompatible, "dataIrreversible": dataIrreversible,
			"pvcMountPath": pvcMountPath, "secretName": secretName, "secretMountPath": secretMountPath,
		})
		data, err = client.ApplicationData(requestCtx, "POST", "/api/v1/application-deliveries/"+deliveryID+"/actions", body)
		if err != nil {
			return platformResult(err)
		}
		if approve {
			var op struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(data, &op) == nil && op.ID != "" {
				approved, approveErr := client.ApplicationData(requestCtx, "POST", "/api/v1/operations/"+op.ID+"/actions/approve", []byte(`{"reason":"approved by hnbctl application action"}`))
				if approveErr != nil {
					return platformResult(approveErr)
				}
				data = approved
			}
		}
	}
	if format == "json" {
		if _, err := env.Stdout.Write(append(data, '\n')); err != nil {
			return fail(ReasonOutputFailed, "cannot write application result")
		}
		return success("")
	}
	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fail(ReasonOutputFailed, "cannot render application result")
	}
	fmt.Fprintln(env.Stdout, string(pretty))
	return success("")
}
