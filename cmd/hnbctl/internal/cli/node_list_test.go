package cli

import (
	"strings"
	"testing"
)

// node list/status 在平台 API 上没有契约：必须诚实拒绝并指向正确入口，不伪造节点行.
func TestNodeListRefusesWithoutFabricatingNodes(t *testing.T) {
	for _, sub := range []string{"list", "status"} {
		code, out, errOut := runCLI(t, "node", sub)
		if code != ExitBlocked {
			t.Fatalf("node %s must be blocked: %d %s", sub, code, out)
		}
		if !strings.Contains(errOut, "cluster list") || !strings.Contains(errOut, "verify") {
			t.Fatalf("node %s must redirect to real entry points: %s", sub, errOut)
		}
		for _, forbidden := range []string{"\"nodes\":", "kubeadm", "ssh "} {
			if strings.Contains(out+errOut, forbidden) {
				t.Fatalf("node %s fabricated output %q: %s", sub, forbidden, out+errOut)
			}
		}
	}
}
