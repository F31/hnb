package gateway

import "time"

type GatewayCapabilitySnapshot struct {
	ID              string    `json:"id"`
	GatewayClassID  string    `json:"gateway_class_id"`
	ProviderName    string    `json:"provider_name"`
	SupportedRoutes []string  `json:"supported_routes"`
	CoreFeatures    []string  `json:"core_features"`
	ExtendedFeatures []string `json:"extended_features"`
	SnapshotJSON    map[string]any `json:"snapshot_json,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
}

type GatewayRequirements struct {
	RequiredRoutes   []string `json:"required_routes"`
	RequiredFeatures []string `json:"required_features"`
}

type CapabilityChecker struct{}

func NewCapabilityChecker() *CapabilityChecker {
	return &CapabilityChecker{}
}

type CapabilityIssue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type CapabilityResult struct {
	Passed bool              `json:"passed"`
	Issues []CapabilityIssue `json:"issues,omitempty"`
}

func (cc *CapabilityChecker) Check(
	req *GatewayRequirements,
	cap *GatewayCapabilitySnapshot,
) *CapabilityResult {
	result := &CapabilityResult{Passed: true}

	for _, routeType := range req.RequiredRoutes {
		found := false
		for _, supported := range cap.SupportedRoutes {
			if routeType == supported {
				found = true
				break
			}
		}
		if !found {
			result.Passed = false
			result.Issues = append(result.Issues, CapabilityIssue{
				Severity: "error",
				Category: "route_type",
				Message:  "requires route type " + routeType + ", provider " + cap.ProviderName + " does not support it",
			})
		}
	}

	for _, feature := range req.RequiredFeatures {
		found := false
		for _, f := range cap.CoreFeatures {
			if feature == f {
				found = true
				break
			}
		}
		if !found {
			for _, f := range cap.ExtendedFeatures {
				if feature == f {
					found = true
					break
				}
			}
		}
		if !found {
			result.Passed = false
			result.Issues = append(result.Issues, CapabilityIssue{
				Severity: "error",
				Category: "feature",
				Message:  "requires feature " + feature + ", provider " + cap.ProviderName + " does not support it",
			})
		}
	}

	return result
}


