package gateway

import (
	"regexp"
)

var timeoutPattern = regexp.MustCompile(`^\d+(ms|s|m|h)$`)

type GatewayProfile struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenant_id"`
	WorkspaceID    string             `json:"workspace_id,omitempty"`
	ProjectID      string             `json:"project_id,omitempty"`
	EnvironmentID  string             `json:"environment_id,omitempty"`
	Name           string             `json:"name"`
	Type           GatewayType        `json:"type"`
	Listeners      []Listener         `json:"listeners"`
	Rules          []ProfileRule      `json:"rules"`
	TLS            *TLSConfig         `json:"tls,omitempty"`
	ProfileDigest  string             `json:"profile_digest"`
}

type ProfileRule struct {
	Name     string            `json:"name"`
	Hostname string            `json:"hostname,omitempty"`
	Matches  []MatchCriteria   `json:"matches,omitempty"`
	Backends []WeightedBackend `json:"backends"`
	Mirror   *MirrorTarget     `json:"mirror,omitempty"`
	Rewrite  *RewriteRule      `json:"rewrite,omitempty"`
	Redirect *RedirectRule     `json:"redirect,omitempty"`
	Headers  *HeaderModifier   `json:"headers,omitempty"`
	Timeout  string            `json:"timeout,omitempty"`
}

type ProfileValidator struct{}

func NewProfileValidator() *ProfileValidator {
	return &ProfileValidator{}
}

func (pv *ProfileValidator) Validate(profile *GatewayProfile) []ValidationError {
	var errors []ValidationError

	if profile.Name == "" {
		errors = append(errors, ValidationError{Field: "name", Message: "name is required"})
	}
	if len(profile.Listeners) == 0 && len(profile.Rules) == 0 {
		errors = append(errors, ValidationError{Field: "profile", Message: "at least one listener or rule is required"})
	}
	for i, l := range profile.Listeners {
		if l.Port < 1 || l.Port > 65535 {
			errors = append(errors, ValidationError{Field: "listeners", Message: "port must be 1-65535", Index: i})
		}
	}
	for i, rule := range profile.Rules {
		if len(rule.Backends) == 0 && rule.Redirect == nil {
			errors = append(errors, ValidationError{
				Field:   "rules",
				Message: "rule requires at least one backend or a redirect",
				Index:   i,
			})
		}
		totalWeight := int32(0)
		for _, b := range rule.Backends {
			totalWeight += b.Weight
		}
		if len(rule.Backends) > 1 && totalWeight == 0 {
			errors = append(errors, ValidationError{
				Field:   "rules",
				Message: "backends with more than one target must have weights",
				Index:   i,
			})
		}
		if rule.Timeout != "" && !timeoutPattern.MatchString(rule.Timeout) {
			errors = append(errors, ValidationError{
				Field:   "rules",
				Message: "invalid timeout format (expected e.g. 5s, 1m, 100ms)",
				Index:   i,
			})
		}
	}
	if profile.Type == GwAI || profile.Type == GwMesh {
		if profile.TLS == nil {
			errors = append(errors, ValidationError{Field: "tls", Message: "AI and Mesh gateways require TLS"})
		}
	}
	return errors
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Index   int    `json:"index,omitempty"`
	Key     string `json:"key,omitempty"`
}
