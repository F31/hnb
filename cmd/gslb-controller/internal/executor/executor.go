package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/F31/hnb/cmd/gslb-controller/internal/dns"
	"github.com/F31/hnb/cmd/gslb-controller/internal/provider"
	"github.com/F31/hnb/pkg/gslb"
)

// DNSProvider 是执行器依赖的 gslb-dns-provider SPI（GSLB-006）。
// 执行器是 DNS 数据面的唯一写入口（GSLB-005：除本执行器外，
// 控制器任何代码路径 SHALL NOT 修改 DNS 数据面）。
type DNSProvider = provider.DNSProvider

// Executor 在 Operation/切换请求上下文中执行不可变计划的 DNS 步骤。
// 它只被 NATS 消费者（经审批的请求）调用，绝不自主触发。
type Executor struct {
	dns DNSProvider
	ttl int
}

func NewExecutor(dnsProvider DNSProvider, ttl int) *Executor {
	if ttl <= 0 {
		ttl = 300
	}
	return &Executor{dns: dnsProvider, ttl: ttl}
}

// ExecutePlan 按依赖顺序执行计划步骤；apply 失败时执行补偿（revert）。
// 仅处理 gslb_dns_* 步骤；drill_compute 不触达数据面（GSLB-010）。
func (e *Executor) ExecutePlan(ctx context.Context, plan *gslb.Plan) error {
	ordered, err := topoSort(plan.Steps)
	if err != nil {
		return err
	}
	compensations := make(map[string]string)
	for _, step := range ordered {
		if step.Compensation != "" {
			compensations[step.StepID] = step.Compensation
		}
	}

	for _, step := range ordered {
		if err := e.ExecuteStep(ctx, step); err != nil {
			// 失败补偿：执行声明了 compensation 的步骤
			if compID := compensations[step.StepID]; compID != "" {
				for _, candidate := range plan.Steps {
					if candidate.StepID == compID {
						_ = e.ExecuteStep(ctx, candidate)
						break
					}
				}
			}
			return fmt.Errorf("execute step %s: %w", step.StepType, err)
		}
	}
	return nil
}

// ExecuteStep 执行单个计划步骤。
func (e *Executor) ExecuteStep(ctx context.Context, step gslb.Step) error {
	switch step.StepType {
	case gslb.StepDNSApply, gslb.StepDNSRevert:
		records := recordsFromInputs(step.Inputs, e.ttl)
		name := endpointName(step.Inputs)
		if _, err := e.dns.ApplyRecords(ctx, name, records); err != nil {
			return err
		}
		return nil
	case gslb.StepDNSVerify:
		name := endpointName(step.Inputs)
		targets := anyToStrings(step.Inputs["targets"])
		if err := e.dns.VerifyTargets(ctx, name, targets); err != nil {
			return err
		}
		return nil
	case gslb.StepDrillCompute:
		// 只读演练：不产生任何 DNS 变更（GSLB-010）
		return nil
	default:
		return fmt.Errorf("unsupported step type %q", step.StepType)
	}
}

// recordsFromInputs 把计划输入的 targets/weights/domain 转换为 DNS 记录集。
func recordsFromInputs(inputs map[string]any, ttl int) []dns.DNSRecord {
	domain, _ := inputs["domain"].(string)
	targets := anyToStrings(inputs["targets"])
	weights := anyToInts(inputs["weights"])
	if len(targets) == 0 {
		return nil
	}
	record := dns.DNSRecord{
		DNSName: domain,
		TTL:     ttl,
	}
	for _, target := range targets {
		record.Targets = append(record.Targets, target)
		if weight, ok := weights[target]; ok {
			record.Weight = weight
		}
	}
	return []dns.DNSRecord{record}
}

func endpointName(inputs map[string]any) string {
	domain, _ := inputs["domain"].(string)
	if domain == "" {
		return "gslb-default"
	}
	name := strings.ReplaceAll(domain, "*", "wildcard")
	return strings.ReplaceAll(name, ".", "-")
}

func anyToStrings(value any) []string {
	switch raw := value.(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyToInts(value any) map[string]int {
	switch raw := value.(type) {
	case map[string]int:
		return raw
	case map[string]any:
		out := make(map[string]int, len(raw))
		for key, item := range raw {
			switch n := item.(type) {
			case float64:
				out[key] = int(n)
			case int:
				out[key] = n
			}
		}
		return out
	default:
		return nil
	}
}

// topoSort 按 DependsOn 排序步骤（线性计划为主，含防御性环检测）。
func topoSort(steps []gslb.Step) ([]gslb.Step, error) {
	byID := make(map[string]gslb.Step, len(steps))
	for _, step := range steps {
		byID[step.StepID] = step
	}
	visited := make(map[string]int) // 0=未访问 1=访问中 2=完成
	var order []gslb.Step
	var visit func(id string) error
	visit = func(id string) error {
		switch visited[id] {
		case 1:
			return fmt.Errorf("plan dependency cycle at %q", id)
		case 2:
			return nil
		}
		visited[id] = 1
		step := byID[id]
		for _, dep := range step.DependsOn {
			if _, ok := byID[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		visited[id] = 2
		order = append(order, step)
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}
