package provider

import "fmt"

const ContractVersion = "2.0.0"

type Profile struct {
	ProviderID      string
	TargetKind      string
	ObservationKind string
	SecretPurpose   string
	StepActions     map[string]string
}

func ProfileForProviderID(providerID string) (Profile, error) {
	switch providerID {
	case "runtime-target.lifecycle.kubernetes":
		return Profile{
			ProviderID:      providerID,
			TargetKind:      "KubernetesTarget",
			ObservationKind: "Agent",
			SecretPurpose:   "kubeconfig",
			StepActions: map[string]string{
				"runtime_target.kubernetes.provision-and-register": "create",
				"runtime_target.kubernetes.register":               "import",
				"runtime_target.kubernetes.upgrade":                "upgrade",
				"runtime_target.kubernetes.unregister":             "unmanage",
			},
		}, nil
	case "runtime-target.lifecycle.edge":
		return Profile{
			ProviderID:      providerID,
			TargetKind:      "EdgeRuntimeTarget",
			ObservationKind: "CloudCore",
			SecretPurpose:   "cloudcore-client",
			StepActions: map[string]string{
				"runtime_target.edge.register":   "import",
				"runtime_target.edge.upgrade":    "upgrade",
				"runtime_target.edge.unregister": "unmanage",
			},
		}, nil
	default:
		return Profile{}, fmt.Errorf("unsupported PROVIDER_ID %q", providerID)
	}
}
