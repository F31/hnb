package toolkit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// credentialShape matches the inline secret material that must never reach
// values, reports, or command output. It is intentionally shape-based: a
// SecretReference is a pointer to a Secret name, not the secret itself.
var credentialShape = regexp.MustCompile(`(?i)(postgres(?:ql)?|nats|tls|mysql|redis)://[^\s]*:[^\s@]*@|-----BEGIN [A-Z ]*PRIVATE KEY-----|\b[A-Za-z0-9+/]{40,}={0,2}\b`)

// PlaneCredential describes one plane's binding to a backing store without
// carrying the credential value.
type PlaneCredential struct {
	Plane     string
	Backing   string
	SecretRef string
	Database  string
	Role      string
}

// InfraPlan is the resolved, credential-free description of every plane
// binding. It is what an operator may safely print, diff, or store.
type InfraPlan struct {
	PostgreSQLMode string            `yaml:"postgresqlMode" json:"postgresqlMode"`
	NATSMode       string            `yaml:"natsMode" json:"natsMode"`
	Planes         []PlaneCredential `yaml:"planes" json:"planes"`
}

// BuildInfraPlan resolves plane bindings and refuses to emit a plan that carries
// credential material or shares one identity across planes.
func BuildInfraPlan(c *DeploymentConfig) (*InfraPlan, error) {
	if c == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	infra := c.Spec.Platform.Infrastructure
	if problems := infra.validatePlanes(); len(problems) > 0 {
		return nil, fmt.Errorf("infrastructure plan is unsafe: %s", strings.Join(problems, "; "))
	}
	plan := &InfraPlan{PostgreSQLMode: infra.PostgreSQL.Mode, NATSMode: infra.NATS.Mode}
	for _, plane := range infra.Planes {
		entry := PlaneCredential{
			Plane: plane.Name,
			// Bundled mode still binds per plane; the value lives in a Secret
			// created at install time, never in configuration.
			Backing:   "postgresql",
			SecretRef: plane.PostgreSQLSecretRef,
			Database:  plane.Database,
			Role:      plane.Role,
		}
		if credentialShape.MatchString(entry.SecretRef) {
			return nil, fmt.Errorf("plane %s appears to embed credential material instead of a Secret reference", plane.Name)
		}
		plan.Planes = append(plan.Planes, entry)
		entry.Backing = "nats"
		entry.Database, entry.Role = "", ""
		entry.SecretRef = plane.NATSSecretRef
		if credentialShape.MatchString(entry.SecretRef) {
			return nil, fmt.Errorf("plane %s nats binding appears to embed credential material", plane.Name)
		}
		plan.Planes = append(plan.Planes, entry)
	}
	sort.SliceStable(plan.Planes, func(i, j int) bool {
		if plan.Planes[i].Plane != plan.Planes[j].Plane {
			return plan.Planes[i].Plane < plan.Planes[j].Plane
		}
		return plan.Planes[i].Backing < plan.Planes[j].Backing
	})
	return plan, nil
}

// RenderInfraPlan produces the operator-facing text. It contains no credential
// value by construction, and it re-checks that property before returning.
func RenderInfraPlan(plan *InfraPlan) (string, error) {
	if plan == nil {
		return "", fmt.Errorf("plan is required")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "postgresql: %s\nnats: %s\n", plan.PostgreSQLMode, plan.NATSMode)
	for _, entry := range plan.Planes {
		line := fmt.Sprintf("plane=%s backing=%s secret=%s", entry.Plane, entry.Backing, entry.SecretRef)
		if entry.Database != "" {
			line += fmt.Sprintf(" database=%s role=%s", entry.Database, entry.Role)
		}
		out.WriteString(line + "\n")
	}
	rendered := out.String()
	if credentialShape.MatchString(rendered) {
		return "", fmt.Errorf("refusing to render an infrastructure plan containing credential material")
	}
	return rendered, nil
}
