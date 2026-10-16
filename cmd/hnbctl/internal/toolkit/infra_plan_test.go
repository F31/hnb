package toolkit

import (
	"strings"
	"testing"
)

func planConfig(t *testing.T) *DeploymentConfig {
	t.Helper()
	cfg, err := DecodeConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Each owned plane must have its own identity. A shared account would let one
// plane read another's tables, which is exactly what the boundary must prevent.
func TestPlaneIdentitiesMustBeDistinct(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*InfraConfig)
		reason string
	}{
		{"shared postgresql credential", func(ic *InfraConfig) {
			ic.Planes[1].PostgreSQLSecretRef = ic.Planes[0].PostgreSQLSecretRef
		}, "share one postgresql credential"},
		{"shared nats credential", func(ic *InfraConfig) {
			ic.Planes[2].NATSSecretRef = ic.Planes[0].NATSSecretRef
		}, "share one nats credential"},
		{"shared role", func(ic *InfraConfig) {
			ic.Planes[1].Role = ic.Planes[0].Role
		}, "share one database role"},
		{"shared database", func(ic *InfraConfig) {
			ic.Planes[1].Database = ic.Planes[0].Database
		}, "share one database"},
		{"missing plane", func(ic *InfraConfig) {
			ic.Planes = ic.Planes[:2]
		}, "missing the artifact plane"},
		{"unknown plane", func(ic *InfraConfig) {
			ic.Planes[0].Name = "platform"
			ic.Planes = append(ic.Planes, PlaneConfig{Name: "shared", Database: "d", Role: "r",
				PostgreSQLSecretRef: "x-pg", NATSSecretRef: "x-nats"})
		}, "is not an owned plane"},
		{"duplicate plane", func(ic *InfraConfig) {
			ic.Planes[1].Name = ic.Planes[0].Name
		}, "more than once"},
		{"missing postgresql secret", func(ic *InfraConfig) {
			ic.Planes[0].PostgreSQLSecretRef = ""
		}, "requires infrastructure.planes.postgresqlSecretRef"},
		{"missing nats secret", func(ic *InfraConfig) {
			ic.Planes[2].NATSSecretRef = ""
		}, "requires infrastructure.planes.natsSecretRef"},
		{"injected credential in secret ref", func(ic *InfraConfig) {
			ic.Planes[0].PostgreSQLSecretRef = "postgres://user:hunter2@db/hnb"
		}, "requires infrastructure.planes.postgresqlSecretRef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := planConfig(t)
			tc.mutate(&cfg.Spec.Platform.Infrastructure)
			if problems := cfg.Validate(CurrentEvidence); !containsSubstring(problems, tc.reason) {
				t.Fatalf("expected %q, got %v", tc.reason, problems)
			}
			if _, err := BuildInfraPlan(cfg); err == nil {
				t.Fatal("BuildInfraPlan produced a plan from an unsafe configuration")
			}
		})
	}
}

// Database and role are PostgreSQL identifiers, not Kubernetes DNS names, so
// underscores are legitimate while injection-shaped values are not.
func TestDatabaseIdentifierRules(t *testing.T) {
	for _, ok := range []string{"hnb_platform", "_private", "market1"} {
		if !validDatabaseIdent(ok) {
			t.Fatalf("valid identifier rejected: %s", ok)
		}
	}
	for _, bad := range []string{"", "1bad", "drop;table", "has space", "a" + strings.Repeat("b", 63), "UPPER"} {
		if validDatabaseIdent(bad) {
			t.Fatalf("invalid identifier accepted: %s", bad)
		}
	}
	cfg := planConfig(t)
	cfg.Spec.Platform.Infrastructure.Planes[0].Database = "hnb_platform; DROP TABLE x"
	if problems := cfg.Validate(CurrentEvidence); !containsSubstring(problems, "requires its own database and role name") {
		t.Fatalf("injected identifier accepted: %v", problems)
	}
}

// The rendered plan is the operator-facing artifact; it must never carry the
// credential, only the reference.
func TestInfraPlanCarriesReferencesNotCredentials(t *testing.T) {
	cfg := planConfig(t)
	plan, err := BuildInfraPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Planes) != len(RequiredPlanes)*2 {
		t.Fatalf("expected a binding per plane per backing, got %d", len(plan.Planes))
	}
	rendered, err := RenderInfraPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"postgresql: bundled", "nats: bundled",
		"plane=platform backing=postgresql secret=platform-postgresql",
		"plane=market backing=nats secret=market-nats"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("plan omitted %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"hunter2", "password", "postgres://", "BEGIN "} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("plan leaked %q:\n%s", forbidden, rendered)
		}
	}
	// External mode must still render references only.
	cfg.Spec.Platform.Infrastructure.PostgreSQL.Mode = "external"
	cfg.Spec.Platform.Infrastructure.PostgreSQL.SecretRef = "tenant/external-pg"
	external, err := BuildInfraPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text, err := RenderInfraPlan(external)
	if err != nil || !strings.Contains(text, "postgresql: external") {
		t.Fatalf("external mode not rendered: %v %q", err, text)
	}
}

// Even if a caller hands the renderer a polluted plan, it must refuse to print.
func TestRenderInfraPlanRefusesPollutedInput(t *testing.T) {
	plan := &InfraPlan{PostgreSQLMode: "bundled", NATSMode: "bundled",
		Planes: []PlaneCredential{{Plane: "platform", Backing: "postgresql",
			SecretRef: "postgres://user:hunter2@db/hnb"}}}
	if _, err := RenderInfraPlan(plan); err == nil {
		t.Fatal("credential-bearing plan was rendered")
	}
}
