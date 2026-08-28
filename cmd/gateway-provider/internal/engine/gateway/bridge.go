package gateway

import (
	"fmt"
)

type GatewayTask struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	StepType    string            `json:"step_type"`
	Inputs      map[string]string `json:"inputs"`
	TimeoutS    int               `json:"timeout_seconds"`
}

type GatewayExecutor struct{}

func NewGatewayExecutor() *GatewayExecutor {
	return &GatewayExecutor{}
}

func (ge *GatewayExecutor) ToTask(profile *GatewayProfile) *GatewayTask {
	stepName := fmt.Sprintf("configure-gateway-%s", profile.Name)
	digestPrefix := profile.ProfileDigest
	if len(digestPrefix) > 16 {
		digestPrefix = digestPrefix[:16]
	}
	return &GatewayTask{
		ID:       fmt.Sprintf("gw-%s", digestPrefix),
		Name:     stepName,
		StepType: "configure_gateway",
		Inputs: map[string]string{
			"gateway_profile_name": profile.Name,
			"gateway_type":         string(profile.Type),
			"profile_digest":       profile.ProfileDigest,
			"rule_count":           fmt.Sprintf("%d", len(profile.Rules)),
			"listener_count":       fmt.Sprintf("%d", len(profile.Listeners)),
		},
		TimeoutS: 300,
	}
}

func (ge *GatewayExecutor) ValidateAndPrepare(
	profile *GatewayProfile,
	req *GatewayRequirements,
	cap *GatewayCapabilitySnapshot,
	gw *Gateway,
) error {
	pv := NewProfileValidator()
	if errs := pv.Validate(profile); len(errs) > 0 {
		return fmt.Errorf("profile validation failed: %v", errs)
	}

	cc := NewCapabilityChecker()
	if result := cc.Check(req, cap); !result.Passed {
		return fmt.Errorf("capability check failed: %v", result.Issues)
	}

	mv := NewMultiTenantValidator()
	if result := mv.CheckAllowedRoutes(gw, profile.Name); !result.Allowed {
		return fmt.Errorf("route not allowed: %s", result.Reason)
	}

	tv := NewTrafficTierValidator()
	if result := tv.Check(appTypeFromProfile(profile), profile.Type); !result.Allowed {
		return fmt.Errorf("tier mismatch: %s", result.Reason)
	}

	return nil
}

func appTypeFromProfile(profile *GatewayProfile) string {
	switch profile.Type {
	case GwAI:
		return "ai"
	case GwMesh:
		return "mesh"
	case GwAPIManagement:
		return "api"
	default:
		return "application"
	}
}
