package core

import (
	"fmt"
	"regexp"
	"strings"
)

var k8sSanitize = regexp.MustCompile(`[^a-z0-9-]`)

type NamespaceMapper struct{}

func (m *NamespaceMapper) PlatformToK8s(tenantID, workspaceID, nsName, prefixTemplate string) string {
	prefix := strings.ReplaceAll(prefixTemplate, "{tenant}", shorten(tenantID, 8))
	prefix = strings.ReplaceAll(prefix, "{workspace}", shorten(workspaceID, 8))
	prefix = strings.ReplaceAll(prefix, "{ns}", shorten(nsName, 8))
	raw := prefix + "-" + nsName
	raw = strings.ToLower(raw)
	raw = strings.Trim(raw, "-")
	raw = k8sSanitize.ReplaceAllString(raw, "-")
	if len(raw) > 63 {
		raw = raw[:63]
		raw = strings.TrimRight(raw, "-")
	}
	return raw
}

func (m *NamespaceMapper) K8sToPlatform(k8sNS, tenantID, workspaceID string) (string, bool) {
	prefix := fmt.Sprintf("t-%s-w-%s", shorten(tenantID, 8), shorten(workspaceID, 8))
	if !strings.HasPrefix(k8sNS, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(k8sNS, prefix)
	rest = strings.TrimPrefix(rest, "-")
	if rest == "" {
		return "", false
	}
	return rest, true
}

func shorten(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}