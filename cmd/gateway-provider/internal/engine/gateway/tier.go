package gateway

type TrafficTierValidator struct{}

func NewTrafficTierValidator() *TrafficTierValidator {
	return &TrafficTierValidator{}
}

type AllowedCombination struct {
	AppType string
	GwType  GatewayType
}

var allowedCombinations = []AllowedCombination{
	{AppType: "application", GwType: GwStandard},
	{AppType: "application", GwType: GwAPIManagement},
	{AppType: "api", GwType: GwAPIManagement},
	{AppType: "api", GwType: GwStandard},
	{AppType: "mesh", GwType: GwMesh},
	{AppType: "ai", GwType: GwAI},
}

func (tv *TrafficTierValidator) Check(appType string, gwType GatewayType) *TierResult {
	for _, c := range allowedCombinations {
		if c.AppType == appType && c.GwType == gwType {
			return &TierResult{Allowed: true}
		}
	}
	return &TierResult{
		Allowed: false,
		Reason:  "app type '" + appType + "' cannot use gateway type '" + string(gwType) + "'",
	}
}

type TierResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}
