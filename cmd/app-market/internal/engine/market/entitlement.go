package market

import (
	"fmt"
	"time"
)

type EntitlementChecker struct {
	entitlements map[string]*Entitlement
	subscriptions map[string]*Subscription
}

func NewEntitlementChecker() *EntitlementChecker {
	return &EntitlementChecker{
		entitlements: make(map[string]*Entitlement),
		subscriptions: make(map[string]*Subscription),
	}
}

func (ec *EntitlementChecker) AddEntitlement(e *Entitlement) {
	ec.entitlements[e.ID] = e
}

func (ec *EntitlementChecker) AddSubscription(s *Subscription) {
	ec.subscriptions[s.ID] = s
}

type AuthorizationResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

func (ec *EntitlementChecker) CheckAccess(tenantID, productID string) *AuthorizationResult {
	for _, sub := range ec.subscriptions {
		if sub.TenantID == tenantID && sub.ProductID == productID {
			if sub.Status != "active" {
				return &AuthorizationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("subscription is %s (not active)", sub.Status),
				}
			}
			if sub.ExpiresAt != nil && time.Now().After(*sub.ExpiresAt) {
				return &AuthorizationResult{
					Allowed: false,
					Reason:  "subscription has expired",
				}
			}
			ent, ok := ec.entitlements[sub.EntitlementID]
			if !ok {
				return &AuthorizationResult{
					Allowed: false,
					Reason:  "entitlement not found",
				}
			}
			if !ent.IsActive {
				return &AuthorizationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("entitlement %s is inactive", ent.EntitlementType),
				}
			}
			return &AuthorizationResult{Allowed: true}
		}
	}
	return &AuthorizationResult{
		Allowed: false,
		Reason:  fmt.Sprintf("no active subscription for tenant %s on product %s", tenantID, productID),
	}
}

func (ec *EntitlementChecker) CheckDeploymentLimit(tenantID, productID string, currentDeployments int) *AuthorizationResult {
	for _, sub := range ec.subscriptions {
		if sub.TenantID == tenantID && sub.ProductID == productID {
			ent, ok := ec.entitlements[sub.EntitlementID]
			if !ok || !ent.IsActive {
				return &AuthorizationResult{Allowed: false, Reason: "inactive entitlement"}
			}
			if ent.MaxDeployments > 0 && currentDeployments >= ent.MaxDeployments {
				return &AuthorizationResult{
					Allowed: false,
					Reason:  fmt.Sprintf("max deployments reached (%d/%d)", currentDeployments, ent.MaxDeployments),
				}
			}
			return &AuthorizationResult{Allowed: true}
		}
	}
	return &AuthorizationResult{Allowed: false, Reason: "no active subscription"}
}
