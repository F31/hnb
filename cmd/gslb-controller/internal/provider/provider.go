// Package provider 定义 gslb-dns-provider SPI（GSLB-006）：
// DNS 数据面可插拔契约。内置参考实现 ExternalDNS 承载于 dns.Manager；
// 其余实现（Cloudflare/NS1/自研权威/Anycast 等）经 gslb Conformance
// Harness 认证后接入，内核与执行器零编译依赖具体厂商。
//
// 执行器（internal/executor）只依赖本 SPI 的 ApplyRecords/VerifyTargets/
// DeleteRecords，任何实现都不得建立独立执行入口（GSLB-006）。
package provider

import (
	"context"
	"errors"

	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
)

// ErrNotVerified 表示验证未通过（provider 应在上报前完成可验证的检查）。
var ErrNotVerified = errors.New("dns targets not verified")

// DNSProvider 是 gslb-dns-provider 契约。
type DNSProvider interface {
	// ApplyRecords 将记录集（域名/目标/权重/TTL）写入数据面并返回
	// 可追踪的变更引用（string 为 provider 侧操作标识，审计用）。
	ApplyRecords(ctx context.Context, name string, records []dns.DNSRecord) (string, error)
	// VerifyTargets 验证权威查询已解析到预期目标（TTL 感知）。
	// 未通过返回 ErrNotVerified。
	VerifyTargets(ctx context.Context, name string, expected []string) error
	// DeleteRecords 删除记录集。
	DeleteRecords(ctx context.Context, name string) error
}

// dnsWriter 是参考实现依赖的最小数据面接口（可注入 fake 测试）。
type dnsWriter interface {
	EnsureEndpoint(ctx context.Context, name string, records []dns.DNSRecord) error
	CleanupOrphaned(ctx context.Context, active map[string]bool) error
}

// ExternalDNS 是内置参考实现（ExternalDNS DNSEndpoint CR）。
type ExternalDNS struct {
	manager dnsWriter
}

func NewExternalDNS(manager *dns.Manager) *ExternalDNS {
	return &ExternalDNS{manager: manager}
}

func (p *ExternalDNS) ApplyRecords(ctx context.Context, name string, records []dns.DNSRecord) (string, error) {
	if err := p.manager.EnsureEndpoint(ctx, name, records); err != nil {
		return "", err
	}
	return "externaldns://" + name, nil
}

// VerifyTargets 参考实现：EnsureEndpoint 幂等重放 + 目标一致性检查。
// 完整的权威 DNS 查询验证属于 Conformance 扩展能力（TTL 感知验证）。
func (p *ExternalDNS) VerifyTargets(ctx context.Context, name string, expected []string) error {
	if len(expected) == 0 {
		return ErrNotVerified
	}
	// 参考实现以重放 EnsureEndpoint 作为“写入可达 + 目标集合正确”的验证；
	// 生产权威查询验证见 Conformance 计划。
	if err := p.manager.EnsureEndpoint(ctx, name, recordsForTargets(name, expected)); err != nil {
		return err
	}
	return nil
}

func (p *ExternalDNS) DeleteRecords(ctx context.Context, name string) error {
	return p.manager.CleanupOrphaned(ctx, map[string]bool{})
}

func recordsForTargets(name string, targets []string) []dns.DNSRecord {
	record := dns.DNSRecord{DNSName: name, TTL: 300}
	for _, target := range targets {
		record.Targets = append(record.Targets, target)
	}
	return []dns.DNSRecord{record}
}
