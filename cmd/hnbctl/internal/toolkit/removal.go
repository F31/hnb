package toolkit

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// RemovalMode is the operator-selected removal intent. The three modes are
// deliberately separate commands: they delete different things and have
// different prerequisites.
type RemovalMode string

const (
	// RemovalUninstallHNB removes the HNB platform release from the cluster it
	// was installed into. It never touches the cluster, CNI or business data.
	RemovalUninstallHNB RemovalMode = "uninstall-hnb"
	// RemovalUnmanage releases the HNB management relationship while leaving the
	// platform release running.
	RemovalUnmanage RemovalMode = "unmanage"
	// RemovalDeleteBase removes a management base that *this install created*.
	// It is refused for an externally owned cluster.
	RemovalDeleteBase RemovalMode = "delete-management-base"
)

const RemovalPlanSchema = "removal-plan/v1"

// RemovalPlan is a dry-run description of exactly what a removal would delete
// and what it would keep. Nothing is executed by this type.
type RemovalPlan struct {
	SchemaVersion string      `json:"schemaVersion"`
	Mode          RemovalMode `json:"mode"`
	RunID         string      `json:"runId"`
	ConfigDigest  string      `json:"configDigest"`
	LockDigest    string      `json:"lockDigest"`
	RoutingMode   string      `json:"routingMode"`
	Namespace     string      `json:"namespace"`
	ReleaseName   string      `json:"releaseName"`
	Delete        []string    `json:"delete"`
	Retain        []string    `json:"retain"`
	// RetainedData is the subset of Retain that holds persistent customer or
	// shared state. It is never deletable by an HNB-only removal.
	RetainedData []string `json:"retainedData"`
	// DataLossAcknowledged must be set before any claimed persistent storage in
	// Delete may actually be removed.
	DataLossAcknowledged bool `json:"dataLossAcknowledged"`
	Authorized           bool `json:"authorized"`
	// Authorization above only describes the proposed boundary. This planner
	// never authorizes execution or claims that deletion has happened.
	ExecutionAuthorized bool      `json:"executionAuthorized"`
	Blocked             []string  `json:"blocked,omitempty"`
	GeneratedAt         time.Time `json:"generatedAt"`
}

// RemovalInput is everything the planner needs. The base ownership flag is
// supplied by the caller from real observation, never assumed.
type RemovalInput struct {
	Manifest *OwnershipManifest
	Run      *InstallRun
	Mode     RemovalMode
	// InstallCreatedBase is true only when this install created the management
	// base. For the existing-Kubernetes route it must be false.
	InstallCreatedBase bool
	// OwnedBaseUID is the identity of a base this install created, if any.
	OwnedBaseUID string
	// DataLossAcknowledged is set by the operator to allow removal of storage
	// that belongs to this release.
	DataLossAcknowledged bool
	// RetainData plans release removal without deleting PVCs or their namespace.
	// The zero value preserves the legacy platform planner contract.
	RetainData      bool
	RetainNamespace bool
	Now             time.Time
}

// persistentMarkers identify claimed entries that carry durable state.
var persistentMarkers = []string{"persistentvolumeclaim", "data", "storage"}

// carriesPersistentData reports whether a claimed entry holds durable state.
func carriesPersistentData(entry string) bool {
	lower := strings.ToLower(entry)
	for _, marker := range persistentMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// PlanRemoval derives an explicit, reviewable removal plan from the recorded
// ownership manifest. It never executes anything and always fails closed.
func PlanRemoval(in RemovalInput) (RemovalPlan, error) {
	plan := RemovalPlan{SchemaVersion: RemovalPlanSchema, Mode: in.Mode,
		DataLossAcknowledged: in.DataLossAcknowledged}
	plan.GeneratedAt = in.Now
	if plan.GeneratedAt.IsZero() {
		plan.GeneratedAt = time.Now().UTC()
	}
	if in.Manifest == nil {
		return plan, fmt.Errorf("ownership manifest is required")
	}
	plan.Namespace = in.Manifest.Namespace
	plan.ReleaseName = in.Manifest.ReleaseName
	if in.Run != nil {
		plan.RunID, plan.ConfigDigest, plan.LockDigest, plan.RoutingMode =
			in.Run.RunID, in.Run.ConfigDigest, in.Run.LockDigest, in.Run.RoutingMode
	}

	// Everything the external owner keeps is retained by construction, in every
	// mode. This is the list that must never shrink.
	plan.Retain = append(plan.Retain, in.Manifest.RequiredExisting...)
	plan.Retain = append(plan.Retain, in.Manifest.PreservedExternal...)
	for _, entry := range in.Manifest.PreservedExternal {
		plan.RetainedData = append(plan.RetainedData, entry)
	}
	// Operator-provided secrets and shared backings are retained data too.
	for _, entry := range in.Manifest.RequiredExisting {
		plan.RetainedData = append(plan.RetainedData, entry)
	}

	switch in.Mode {
	case RemovalUninstallHNB:
		for _, entry := range in.Manifest.Claimed {
			if entry == in.Manifest.Namespace || entry == in.Manifest.Namespace+"/" {
				continue // namespace handled as a unit below
			}
			if strings.HasPrefix(entry, "namespace/") {
				continue
			}
			if in.RetainData && carriesPersistentData(entry) {
				plan.Retain = append(plan.Retain, entry)
				plan.RetainedData = append(plan.RetainedData, entry)
				continue
			}
			plan.Delete = append(plan.Delete, entry)
		}
		if in.RetainData || in.RetainNamespace {
			plan.Retain = append(plan.Retain, "namespace/"+in.Manifest.Namespace+" (preserves retained data and operator-owned resources)")
		} else {
			plan.Delete = append(plan.Delete, "namespace/"+in.Manifest.Namespace+" (release contents)")
		}
		if !in.RetainData && carriesPersistentDataIn(in.Manifest.Claimed) && !in.DataLossAcknowledged {
			plan.Blocked = append(plan.Blocked, "PERSISTENT_STORAGE_REMOVAL_UNACKNOWLEDGED: this release owns persistent storage; explicit acknowledgement is required")
		}

	case RemovalUnmanage:
		// Detaching changes no local resources. The management relationship is
		// released through the formal platform contract, not by deletion.
		plan.Retain = append(plan.Retain, "release/"+in.Manifest.ReleaseName+" ("+in.Manifest.Namespace+")")
		plan.Blocked = append(plan.Blocked, "UNMANAGE_REQUIRES_PLATFORM_OPERATION: the management relationship is released through a formal platform intent; no local deletion is performed")

	case RemovalDeleteBase:
		if !in.InstallCreatedBase {
			plan.Blocked = append(plan.Blocked, "BASE_NOT_OWNED_BY_THIS_INSTALL: this install did not create the management base; deleting it would destroy an external owner's cluster")
		}
		if in.OwnedBaseUID == "" {
			plan.Blocked = append(plan.Blocked, "BASE_UID_UNVERIFIED: the management-base identity must be observed and bound before deletion")
		}
		// The platform release goes first, then the base itself.
		plan.Delete = append(plan.Delete, "release/"+in.Manifest.ReleaseName+" ("+in.Manifest.Namespace+")")
		plan.Delete = append(plan.Delete, "management-base/uid="+in.OwnedBaseUID)
		if in.Manifest.NoNodeInitAction && !in.InstallCreatedBase {
			// Existing route: nothing about this install ever initialized nodes,
			// so it must not claim node ownership either.
			plan.Blocked = append(plan.Blocked, "NODE_OWNERSHIP_NOT_ESTABLISHED: this install performed no node initialization and therefore owns no nodes")
		}

	default:
		return plan, fmt.Errorf("unsupported removal mode %q", in.Mode)
	}

	plan.Delete = uniqueSorted(plan.Delete)
	plan.Retain = uniqueSorted(plan.Retain)
	plan.RetainedData = uniqueSorted(plan.RetainedData)
	plan.Blocked = uniqueSorted(plan.Blocked)

	// Safety invariant: nothing the external owner owns may ever appear in the
	// delete set, regardless of mode.
	for _, preserved := range in.Manifest.PreservedExternal {
		for _, deletion := range plan.Delete {
			if strings.Contains(deletion, preserved) {
				plan.Blocked = append(plan.Blocked, "OWNERSHIP_VIOLATION: preserved external resource appeared in the delete set")
			}
		}
	}
	for _, required := range in.Manifest.RequiredExisting {
		for _, deletion := range plan.Delete {
			if deletion == required {
				plan.Blocked = append(plan.Blocked, "OWNERSHIP_VIOLATION: operator-provided secret appeared in the delete set")
			}
		}
	}
	plan.Blocked = uniqueSorted(plan.Blocked)
	plan.Authorized = len(plan.Blocked) == 0
	sort.Strings(plan.Delete)
	return plan, nil
}

func carriesPersistentDataIn(claimed []string) bool {
	for _, entry := range claimed {
		if carriesPersistentData(entry) {
			return true
		}
	}
	return false
}

// FormatRemovalHuman renders the plan for review before any action.
func FormatRemovalHuman(w interface{ Write([]byte) (int, error) }, plan RemovalPlan) {
	fmt.Fprintf(w, "mode=%s authorized=%v run=%s namespace=%s\n", plan.Mode, plan.Authorized, plan.RunID, plan.Namespace)
	fmt.Fprintf(w, "  delete:\n")
	for _, entry := range plan.Delete {
		fmt.Fprintf(w, "    - %s\n", entry)
	}
	fmt.Fprintf(w, "  retain:\n")
	for _, entry := range plan.Retain {
		fmt.Fprintf(w, "    - %s\n", entry)
	}
	for _, reason := range plan.Blocked {
		fmt.Fprintf(w, "  blocked: %s\n", reason)
	}
}
