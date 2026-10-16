package toolkit

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// ReadSecretKey reads one key from a cluster Secret via the cluster runner. The
// value is returned in-process only; callers must never log or retain it, and
// errors never echo the decoded value.
func ReadSecretKey(ctx context.Context, runner ClusterRunner, kubeconfig, namespace, name, key string) (string, error) {
	if runner == nil || kubeconfig == "" || namespace == "" || name == "" || key == "" {
		return "", fmt.Errorf("SECRET_INPUT_INVALID: runner, kubeconfig, namespace, name and key are required")
	}
	out, err := runner.Run(ctx, kubeconfig, nil,
		"-n", namespace, "get", "secret", name, "-o", "jsonpath={.data."+key+"}")
	if err != nil {
		return "", fmt.Errorf("SECRET_READ_FAILED")
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return "", fmt.Errorf("SECRET_KEY_MISSING")
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) == 0 {
		return "", fmt.Errorf("SECRET_KEY_INVALID")
	}
	return string(decoded), nil
}
