package toolkit

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

// secretRunner returns a canned stdout for the secret query.
type secretRunner struct {
	out string
	err error
}

func (r secretRunner) Run(_ context.Context, kubeconfig string, _ io.Reader, args ...string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.out, nil
}

func TestReadSecretKeyDecodes(t *testing.T) {
	dsn := "postgres://user:pass@db:5432/hnb?sslmode=disable"
	encoded := base64.StdEncoding.EncodeToString([]byte(dsn))
	got, err := ReadSecretKey(context.Background(), secretRunner{out: encoded + "\n"}, "/kc", "hnb-system", "platform-postgresql", "dsn")
	if err != nil {
		t.Fatal(err)
	}
	if got != dsn {
		t.Fatalf("decoded mismatch: %q", got)
	}
}

func TestReadSecretKeyFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		runner ClusterRunner
		want   string
	}{
		{"read-error", secretRunner{err: errors.New("boom: postgres://secret")}, "SECRET_READ_FAILED"},
		{"missing", secretRunner{out: "  \n"}, "SECRET_KEY_MISSING"},
		{"invalid-b64", secretRunner{out: "not base64!!"}, "SECRET_KEY_INVALID"},
	}
	for _, tc := range cases {
		_, err := ReadSecretKey(context.Background(), tc.runner, "/kc", "ns", "name", "dsn")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %s, got %v", tc.name, tc.want, err)
		}
		if strings.Contains(err.Error(), "postgres://secret") {
			t.Fatalf("%s leaked secret material: %v", tc.name, err)
		}
	}
	if _, err := ReadSecretKey(context.Background(), secretRunner{}, "", "ns", "n", "k"); err == nil || !strings.Contains(err.Error(), "SECRET_INPUT_INVALID") {
		t.Fatalf("missing input not refused: %v", err)
	}
}
