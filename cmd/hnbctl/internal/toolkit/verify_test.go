package toolkit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedVerifyTime() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }

// passCheck builds a passing check for the full required catalog.
func passCatalog(overrides map[string]bool) []VerifySpec {
	overrides = overridesOrEmpty(overrides)
	var specs []VerifySpec
	for _, layer := range DeliveryLayers {
		for _, id := range VerificationCatalog()[layer] {
			pass := !overrides[id]
			specs = append(specs, VerifySpec{CheckID: id, Layer: layer, Expected: "verified",
				Probe: func(context.Context) (string, bool, error) {
					if pass {
						return "verified", true, nil
					}
					return "observed failure", false, nil
				}})
		}
	}
	return specs
}

func overridesOrEmpty(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}

// 全层通过才是完整交付.
func TestVerifyCompleteOnlyWhenEveryRequiredCheckPasses(t *testing.T) {
	report := RunVerify(context.Background(), passCatalog(nil), fixedVerifyTime())
	if !report.Delivery.CompleteDelivery || report.Delivery.Overall != DeliverySuccess {
		t.Fatalf("all-pass verification not accepted: %+v", report.Delivery)
	}
	if len(report.MissingRequiredChecks()) != 0 {
		t.Fatalf("unexpected missing checks: %v", report.MissingRequiredChecks())
	}
	if len(report.Cleanup) != 0 {
		t.Fatalf("no test resources were declared, got cleanup: %+v", report.Cleanup)
	}
}

// 平台可登录但真实 Operation 未完成 -> 不得提升为完整交付成功.
func TestVerifyLoginPassingButAppOperationFailingIsNotComplete(t *testing.T) {
	report := RunVerify(context.Background(), passCatalog(map[string]bool{"platform.app-operation": true}), fixedVerifyTime())
	if report.Delivery.CompleteDelivery || report.Delivery.Overall == DeliverySuccess {
		t.Fatalf("login+cluster health promoted to complete delivery: %+v", report.Delivery)
	}
	if len(report.Delivery.FailedLayers) == 0 || report.Delivery.FailedLayers[0] != LayerPlatform {
		t.Fatalf("failing layer not surfaced: %+v", report.Delivery.FailedLayers)
	}
}

// Agent 观测与 Read Model 各自是独立失败门.
func TestVerifyAgentAndReadModelAreIndependentGates(t *testing.T) {
	for _, gate := range []string{"platform.agent-observation", "platform.read-model", "managed.agent-authenticated"} {
		report := RunVerify(context.Background(), passCatalog(map[string]bool{gate: true}), fixedVerifyTime())
		if report.Delivery.CompleteDelivery {
			t.Fatalf("%s failure did not block acceptance", gate)
		}
	}
}

// 未实际评估的必需检查同样阻断验收（健康子集不得冒充完整交付）.
func TestVerifyMissingRequiredChecksBlockAcceptance(t *testing.T) {
	// Drop the login check entirely: cluster/node remain healthy.
	var specs []VerifySpec
	for _, layer := range DeliveryLayers {
		for _, id := range VerificationCatalog()[layer] {
			if id == "platform.login" {
				continue
			}
			specs = append(specs, VerifySpec{CheckID: id, Layer: layer, Expected: "verified",
				Probe: func(context.Context) (string, bool, error) { return "verified", true, nil }})
		}
	}
	report := RunVerify(context.Background(), specs, fixedVerifyTime())
	if report.Delivery.CompleteDelivery {
		t.Fatal("a subset of healthy layers was accepted as complete delivery")
	}
	missing := report.MissingRequiredChecks()
	if len(missing) != 1 || missing[0] != "platform/platform.login" {
		t.Fatalf("missing check not reported: %v", missing)
	}
}

// 探针不可用（错误）判为 blocked，而非 pass.
func TestVerifyProbeErrorIsBlockedNotPassed(t *testing.T) {
	specs := passCatalog(nil)
	for i := range specs {
		if specs[i].CheckID == "platform.pg" {
			specs[i].Probe = func(context.Context) (string, bool, error) { return "", false, errors.New("dial tcp: refused") }
		}
	}
	report := RunVerify(context.Background(), specs, fixedVerifyTime())
	if report.Delivery.CompleteDelivery {
		t.Fatal("probe error accepted as verified")
	}
	found := false
	for _, layer := range report.Layers {
		for _, check := range layer.Checks {
			if check.CheckID == "platform.pg" {
				found = true
				if check.State != LayerBlocked || check.ReasonCode != "VERIFY_PROBE_UNAVAILABLE" {
					t.Fatalf("probe error mislabeled: %+v", check)
				}
			}
		}
	}
	if !found {
		t.Fatal("pg check missing from report")
	}
}

// 自有测试资源必须清理；清理失败阻断验收.
func TestVerifyOwnTestResourceCleanupBlocksOnLeftover(t *testing.T) {
	var cleanupCalls int
	build := func(cleanupErr error) []VerifySpec {
		specs := passCatalog(nil)
		specs = append(specs, VerifySpec{CheckID: "platform.app-operation.smoke-workload", Layer: LayerPlatform,
			Expected: "temporary verification workload executes", OwnedTestResource: true,
			Probe:   func(context.Context) (string, bool, error) { return "executed", true, nil },
			Cleanup: func(context.Context) error { cleanupCalls++; return cleanupErr }})
		return specs
	}

	report := RunVerify(context.Background(), build(nil), fixedVerifyTime())
	if cleanupCalls != 1 || len(report.LeftoverOwnedResources) != 0 || !report.Delivery.CompleteDelivery {
		t.Fatalf("successful cleanup should still allow acceptance: %+v", report)
	}
	if len(report.Cleanup) != 1 || !report.Cleanup[0].Removed {
		t.Fatalf("cleanup not recorded: %+v", report.Cleanup)
	}

	report = RunVerify(context.Background(), build(errors.New("still in use")), fixedVerifyTime())
	if report.Delivery.CompleteDelivery {
		t.Fatal("leftover owned test resource did not block acceptance")
	}
	if len(report.LeftoverOwnedResources) != 1 || report.LeftoverOwnedResources[0] != "platform.app-operation.smoke-workload" {
		t.Fatalf("leftover not reported: %+v", report.LeftoverOwnedResources)
	}
}

// 没有验证任何一层时不得报告成功.
func TestVerifyWithNoChecksIsNotComplete(t *testing.T) {
	report := RunVerify(context.Background(), nil, fixedVerifyTime())
	if report.Delivery.CompleteDelivery || report.Delivery.Overall == DeliverySuccess {
		t.Fatalf("empty verification reported success: %+v", report.Delivery)
	}
	if len(report.MissingLayers()) == 0 {
		t.Fatal("expected unverified layers to be listed")
	}
}

// 缺少探针的检查判 blocked 并保留说明.
func TestVerifyCheckWithoutProbeIsBlocked(t *testing.T) {
	report := RunVerify(context.Background(), []VerifySpec{{CheckID: "platform.login", Layer: LayerPlatform, Expected: "login"}}, fixedVerifyTime())
	var check VerifyCheck
	for _, layer := range report.Layers {
		for _, c := range layer.Checks {
			if c.CheckID == "platform.login" {
				check = c
			}
		}
	}
	if check.State != LayerBlocked || check.ReasonCode != "VERIFY_CHECK_MISSING" {
		t.Fatalf("missing probe mislabeled: %+v", check)
	}
}

// 人类输出列出各层与缺环，且不泄露凭据.
func TestVerifyHumanOutputShowsGapsWithoutSecrets(t *testing.T) {
	specs := passCatalog(map[string]bool{"platform.jetstream": true})
	for i := range specs {
		if specs[i].CheckID == "platform.read-model" {
			specs[i].Probe = func(context.Context) (string, bool, error) {
				return "postgres://user:hunter2@db/hnb", true, nil
			}
		}
	}
	report := RunVerify(context.Background(), specs, fixedVerifyTime())
	var sb strings.Builder
	FormatVerifyHuman(&sb, report)
	out := sb.String()
	for _, want := range []string{"overall=", "platform [failed]", "platform.jetstream"} {
		if !strings.Contains(out, want) {
			t.Fatalf("human output missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{"hunter2", "BEGIN EC PRIVATE KEY"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("verify output leaked %q: %s", forbidden, out)
		}
	}
}
