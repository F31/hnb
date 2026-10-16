package toolkit

import (
	"context"
	"strings"
	"testing"
)

// 门禁未认证时，Day-0 管线拒绝且不接触任何节点.
func TestRunManagementBootstrapRefusesWithoutEvidence(t *testing.T) {
	cfg, lock, sources := bootstrapFixture(t)
	node := newFakeBootstrapNode()
	_, err := RunManagementBootstrap(context.Background(), cfg, lock, sources,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", privateRunDir(t), node, CurrentEvidence)
	if err == nil || !strings.Contains(err.Error(), "PLATFORM_NOT_CERTIFIED") {
		t.Fatalf("uncertified bootstrap not refused: %v", err)
	}
	if len(node.scripts) != 0 {
		t.Fatalf("node contacted while gated: %v", node.scripts)
	}
}

// 合成已认证证据下，管线执行 Day-0 引导并返回就绪结果（复用假节点）.
func TestRunManagementBootstrapRunsWhenCertified(t *testing.T) {
	cfg, lock, sources := bootstrapFixture(t)
	node := newFakeBootstrapNode()
	matrix := &EvidenceMatrix{
		Version:       "test",
		CertifiedArch: []string{"amd64"},
		Profiles:      []EvidenceProfile{{Name: string(ProfileMinimal), Status: "certified"}},
		Routes:        []EvidenceProfile{{Name: string(ModeBootstrap), Status: "certified"}},
	}
	result, err := RunManagementBootstrap(context.Background(), cfg, lock, sources,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", privateRunDir(t), node, matrix)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ClusterReady || result.PlatformVerified || len(node.scripts) == 0 {
		t.Fatalf("unexpected bootstrap result: %+v", result)
	}
}
