package toolkit

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Capacity is a planning estimate from rendered requests, never installation
// authorization. Node inputs are operator-declared, not live observations.
type CapacityNode struct {
	Name          string `yaml:"name" json:"name"`
	FailureDomain string `yaml:"failureDomain" json:"failureDomain"`
	ControlPlane  bool   `yaml:"controlPlane" json:"controlPlane"`
	CPUMilli      int64  `yaml:"cpuMilli" json:"cpuMilli"`
	MemoryMi      int64  `yaml:"memoryMi" json:"memoryMi"`
	DiskGi        int64  `yaml:"diskGi" json:"diskGi"`
	// Optional per-node system reserve overrides. Zero means the planning
	// default (1 core, 2 GiB, 20 Gi per node). Use these to plan a lean host.
	ReserveCPUMilli int64 `yaml:"reserveCpuMilli,omitempty" json:"reserveCpuMilli,omitempty"`
	ReserveMemoryMi int64 `yaml:"reserveMemoryMi,omitempty" json:"reserveMemoryMi,omitempty"`
	ReserveDiskGi   int64 `yaml:"reserveDiskGi,omitempty" json:"reserveDiskGi,omitempty"`
}

// CapacityPlanningOptions lets an operator place download cache and backup on
// external storage so they are reported as budgets but do not consume node disk.
// Scope selects the planning margin: "delivery" (default, 30% headroom and the
// conservative per-node reserve) or "startup" (10% headroom and a lean default
// reserve for a dedicated evaluation/startup host). Startup is planning for a
// dev/test starting tier, never a production certification.
type CapacityPlanningOptions struct {
	CacheSource  string // "local" (default) or "external"
	BackupSource string // "local" (default) or "external"
	Scope        string // "delivery" (default) or "startup"
	// Authorization, when provided, connects the certification/material evidence
	// so InstallAuthorized reflects a real decision instead of defaulting false.
	// When nil the report keeps today's behavior (InstallAuthorized=false with no
	// extra reason); the field exists so callers that hold verified material and a
	// passed EvidenceGate can surface an authorized install decision.
	Authorization *InstallAuthorization
}

// InstallAuthorization carries the evidence an install authorization decision is
// computed from. Rejections is the failing EvidenceGate list; MaterialVerified
// means a complete offline closure was validated. All must be satisfied AND the
// declared capacity must fit before InstallAuthorized becomes true.
type InstallAuthorization struct {
	Certified        bool
	MaterialVerified bool
	Rejections       []string
}

type CapacityResources struct {
	CPUMilli int64 `json:"cpuMilli"`
	MemoryMi int64 `json:"memoryMi"`
	DiskGi   int64 `json:"diskGi"`
}

type CapacityWorkload struct {
	Name     string            `json:"name"`
	Replicas int64             `json:"replicas"`
	PerPod   CapacityResources `json:"perPod"`
}

type CapacityReport struct {
	SchemaVersion              string             `json:"schemaVersion"`
	Profile                    Profile            `json:"profile"`
	ManifestDigest             string             `json:"manifestDigest"`
	InputSource                string             `json:"inputSource"`
	Workloads                  []CapacityWorkload `json:"workloads"`
	Requests                   CapacityResources  `json:"requests"`
	SystemReserve              CapacityResources  `json:"systemReserve"`
	DownloadCacheGi            int64              `json:"downloadCacheGi"`
	BackupGi                   int64              `json:"backupGi"`
	HeadroomPercent            int64              `json:"headroomPercent"`
	Scope                      string             `json:"scope"`
	CacheSource                string             `json:"cacheSource"`
	BackupSource               string             `json:"backupSource"`
	Required                   CapacityResources  `json:"required"`
	Declared                   CapacityResources  `json:"declared"`
	ControlPlaneFailureDomains int                `json:"controlPlaneFailureDomains"`
	Reasons                    []string           `json:"reasons"`
	FitsDeclaredCapacity       bool               `json:"fitsDeclaredCapacity"`
	InstallAuthorized          bool               `json:"installAuthorized"`
}

type capacityContainer struct {
	RestartPolicy string `yaml:"restartPolicy"`
	Resources     struct {
		Requests map[string]string `yaml:"requests"`
	} `yaml:"resources"`
}
type capacityPod struct {
	Containers     []capacityContainer `yaml:"containers"`
	InitContainers []capacityContainer `yaml:"initContainers"`
	Volumes        []struct {
		PVC *struct {
			ClaimName string `yaml:"claimName"`
		} `yaml:"persistentVolumeClaim"`
	} `yaml:"volumes"`
}
type capacityObject struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string            `yaml:"name"`
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		Replicas    *int64 `yaml:"replicas"`
		Parallelism *int64 `yaml:"parallelism"`
		Template    struct {
			Spec capacityPod `yaml:"spec"`
		} `yaml:"template"`
		Containers     []capacityContainer `yaml:"containers"`
		InitContainers []capacityContainer `yaml:"initContainers"`
		Resources      struct {
			Requests map[string]string `yaml:"requests"`
		} `yaml:"resources"`
		VolumeClaimTemplates []struct {
			Spec struct {
				Resources struct {
					Requests map[string]string `yaml:"requests"`
				} `yaml:"resources"`
			} `yaml:"spec"`
		} `yaml:"volumeClaimTemplates"`
	} `yaml:"spec"`
}

// Only quantity forms used by these charts are accepted. Unsupported units fail
// explicitly rather than becoming zero resource requirements.
func capacityQuantity(raw string, cpu bool) (int64, error) {
	scale := float64(1)
	if cpu {
		scale = 1000
		if strings.HasSuffix(raw, "m") {
			raw, scale = strings.TrimSuffix(raw, "m"), 1
		}
	} else {
		switch {
		case strings.HasSuffix(raw, "Gi"):
			raw, scale = strings.TrimSuffix(raw, "Gi"), 1024
		case strings.HasSuffix(raw, "Mi"):
			raw = strings.TrimSuffix(raw, "Mi")
		case strings.HasSuffix(raw, "Ki"):
			raw, scale = strings.TrimSuffix(raw, "Ki"), 1.0/1024
		default:
			return 0, fmt.Errorf("unsupported memory/storage quantity")
		}
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n*scale > 1e12 {
		return 0, fmt.Errorf("invalid or missing resource quantity")
	}
	return int64(math.Ceil(n * scale)), nil
}

func podCapacity(p capacityPod) (CapacityResources, error) {
	var sum, init CapacityResources
	if len(p.Containers) == 0 {
		return sum, fmt.Errorf("workload has no containers")
	}
	for _, c := range p.Containers {
		cpu, err := capacityQuantity(c.Resources.Requests["cpu"], true)
		if err != nil {
			return sum, err
		}
		mem, err := capacityQuantity(c.Resources.Requests["memory"], false)
		if err != nil {
			return sum, err
		}
		sum.CPUMilli += cpu
		sum.MemoryMi += mem
	}
	for _, c := range p.InitContainers {
		if c.RestartPolicy != "" {
			return sum, fmt.Errorf("restartable init containers require a separate scheduling model")
		}
		cpu, err := capacityQuantity(c.Resources.Requests["cpu"], true)
		if err != nil {
			return sum, err
		}
		mem, err := capacityQuantity(c.Resources.Requests["memory"], false)
		if err != nil {
			return sum, err
		}
		if cpu > init.CPUMilli {
			init.CPUMilli = cpu
		}
		if mem > init.MemoryMi {
			init.MemoryMi = mem
		}
	}
	if init.CPUMilli > sum.CPUMilli {
		sum.CPUMilli = init.CPUMilli
	}
	if init.MemoryMi > sum.MemoryMi {
		sum.MemoryMi = init.MemoryMi
	}
	return sum, nil
}

func BuildCapacityReport(profile Profile, manifest []byte, nodes []CapacityNode) (CapacityReport, error) {
	return BuildCapacityReportWithOptions(profile, manifest, nodes, CapacityPlanningOptions{})
}

func capacityNodeReserve(n CapacityNode, scope string) CapacityResources {
	cpu, memory, disk := int64(1000), int64(2048), int64(20)
	if scope == "startup" {
		cpu, memory, disk = 500, 1024, 10
	}
	if n.ReserveCPUMilli != 0 {
		cpu = n.ReserveCPUMilli
	}
	if n.ReserveMemoryMi != 0 {
		memory = n.ReserveMemoryMi
	}
	if n.ReserveDiskGi != 0 {
		disk = n.ReserveDiskGi
	}
	return CapacityResources{CPUMilli: cpu, MemoryMi: memory, DiskGi: disk}
}

func BuildCapacityReportWithOptions(profile Profile, manifest []byte, nodes []CapacityNode, opts CapacityPlanningOptions) (CapacityReport, error) {
	if opts.CacheSource == "" {
		opts.CacheSource = "local"
	}
	if opts.BackupSource == "" {
		opts.BackupSource = "local"
	}
	if opts.Scope == "" {
		opts.Scope = "delivery"
	}
	if (opts.CacheSource != "local" && opts.CacheSource != "external") ||
		(opts.BackupSource != "local" && opts.BackupSource != "external") ||
		(opts.Scope != "delivery" && opts.Scope != "startup") {
		return CapacityReport{}, fmt.Errorf("cache/backup placement must be local or external; scope must be delivery or startup")
	}
	r := CapacityReport{SchemaVersion: "capacity-report/v1", Profile: profile, ManifestDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(manifest)), InputSource: "rendered manifests and operator-declared nodes; planning only", HeadroomPercent: 30, Scope: opts.Scope, CacheSource: opts.CacheSource, BackupSource: opts.BackupSource, Reasons: []string{}, Workloads: []CapacityWorkload{}}
	if opts.Scope == "startup" {
		r.HeadroomPercent = 10
	}
	switch profile {
	case ProfileMinimal, ProfileLiteHA, ProfileStandardHA, ProfileEnterprise:
	default:
		return r, fmt.Errorf("unknown deployment profile")
	}
	seenNodes, domains := map[string]bool{}, map[string]bool{}
	for _, n := range nodes {
		if !validName(n.Name) || seenNodes[n.Name] || n.CPUMilli <= 1000 || n.CPUMilli > 1e9 || n.MemoryMi <= 2048 || n.MemoryMi > 1e9 || n.DiskGi < 10 || n.DiskGi > 1e9 {
			return r, fmt.Errorf("invalid, duplicate or undersized declared node")
		}
		if n.ReserveCPUMilli < 0 || n.ReserveMemoryMi < 0 || n.ReserveDiskGi < 0 || n.ReserveCPUMilli >= n.CPUMilli || n.ReserveMemoryMi >= n.MemoryMi || n.ReserveDiskGi >= n.DiskGi {
			return r, fmt.Errorf("node reserve must be nonnegative and smaller than declared capacity")
		}
		seenNodes[n.Name] = true
		if n.ControlPlane && n.FailureDomain != "" {
			domains[n.FailureDomain] = true
		}
		r.Declared.CPUMilli += n.CPUMilli
		r.Declared.MemoryMi += n.MemoryMi
		r.Declared.DiskGi += n.DiskGi
	}
	if len(nodes) == 0 {
		return r, fmt.Errorf("node capacity declarations are required")
	}
	r.ControlPlaneFailureDomains = len(domains)
	seenObjects, components, claims := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var usedClaims []string
	decoder := yaml.NewDecoder(strings.NewReader(string(manifest)))
	for {
		var obj capacityObject
		if err := decoder.Decode(&obj); err == io.EOF {
			break
		} else if err != nil {
			return r, fmt.Errorf("invalid rendered YAML")
		}
		if obj.Kind == "" {
			continue
		}
		key := obj.Kind + "/" + obj.Metadata.Namespace + "/" + obj.Metadata.Name
		if seenObjects[key] {
			return r, fmt.Errorf("duplicate rendered object")
		}
		seenObjects[key] = true
		switch obj.Kind {
		case "PersistentVolumeClaim":
			mi, err := capacityQuantity(obj.Spec.Resources.Requests["storage"], false)
			if err != nil {
				return r, err
			}
			r.Requests.DiskGi += (mi + 1023) / 1024
			claims[obj.Metadata.Namespace+"/"+obj.Metadata.Name] = true
		case "Deployment", "StatefulSet", "Job", "Pod":
			replicas := int64(1)
			if obj.Spec.Replicas != nil {
				replicas = *obj.Spec.Replicas
			}
			if obj.Kind == "Job" && obj.Spec.Parallelism != nil {
				replicas = *obj.Spec.Parallelism
			}
			if replicas < 1 || replicas > 10000 {
				return r, fmt.Errorf("replicas must be positive and bounded")
			}
			pod := obj.Spec.Template.Spec
			if obj.Kind == "Pod" {
				pod.Containers, pod.InitContainers = obj.Spec.Containers, obj.Spec.InitContainers
			}
			perPod, err := podCapacity(pod)
			if err != nil {
				return r, fmt.Errorf("workload requests incomplete: %w", err)
			}
			for _, v := range pod.Volumes {
				if v.PVC != nil {
					usedClaims = append(usedClaims, obj.Metadata.Namespace+"/"+v.PVC.ClaimName)
				}
			}
			for _, claim := range obj.Spec.VolumeClaimTemplates {
				mi, err := capacityQuantity(claim.Spec.Resources.Requests["storage"], false)
				if err != nil {
					return r, err
				}
				r.Requests.DiskGi += ((mi + 1023) / 1024) * replicas
			}
			r.Workloads = append(r.Workloads, CapacityWorkload{Name: key, Replicas: replicas, PerPod: perPod})
			r.Requests.CPUMilli += perPod.CPUMilli * replicas
			r.Requests.MemoryMi += perPod.MemoryMi * replicas
			component := obj.Metadata.Labels["app.kubernetes.io/name"]
			if component == "" {
				component = obj.Metadata.Name
			}
			components[component] = true
			fits := false
			for _, n := range nodes {
				reserve := capacityNodeReserve(n, opts.Scope)
				if n.CPUMilli-reserve.CPUMilli >= perPod.CPUMilli && n.MemoryMi-reserve.MemoryMi >= perPod.MemoryMi {
					fits = true
				}
			}
			if !fits {
				r.Reasons = append(r.Reasons, "POD_EXCEEDS_SINGLE_NODE_CAPACITY")
			}
		case "DaemonSet", "CronJob", "List":
			return r, fmt.Errorf("workload kind requires an unsupported capacity model")
		}
	}
	if len(r.Workloads) == 0 {
		return r, fmt.Errorf("rendered manifest has no workloads")
	}
	for _, claim := range usedClaims {
		if !claims[claim] {
			r.Reasons = append(r.Reasons, "EXTERNAL_PVC_CAPACITY_UNVERIFIED")
		}
	}
	if profile == ProfileMinimal {
		for _, c := range []string{"edge-provider", "karmada-provider", "gslb-controller", "calico-provider", "kube-ovn-provider", "network-provider"} {
			if components[c] {
				r.Reasons = append(r.Reasons, "MINIMAL_OPTIONAL_PACK_ENABLED")
			}
		}
	} else {
		if len(domains) < 3 {
			r.Reasons = append(r.Reasons, "HA_FAILURE_DOMAINS_INSUFFICIENT")
		}
		r.Reasons = append(r.Reasons, "HA_PROFILE_UNCERTIFIED")
	}
	var sysReserve CapacityResources
	startup := r.Scope == "startup"
	for _, n := range nodes {
		reserve := capacityNodeReserve(n, opts.Scope)
		sysReserve.CPUMilli += reserve.CPUMilli
		sysReserve.MemoryMi += reserve.MemoryMi
		sysReserve.DiskGi += reserve.DiskGi
	}
	r.SystemReserve = sysReserve
	r.DownloadCacheGi = int64(len(nodes)) * 20
	r.BackupGi = r.Requests.DiskGi
	// Budgets placed on external storage are still reported, but they no longer
	// consume node disk when deciding whether the declared node fits.
	onNodeDiskGi := r.Requests.DiskGi
	if r.CacheSource == "local" {
		onNodeDiskGi += r.DownloadCacheGi
	}
	if r.BackupSource == "local" {
		onNodeDiskGi += r.BackupGi
	}
	headroom := int64(130)
	if startup {
		headroom = 110
	}
	r.Required = CapacityResources{CPUMilli: (r.Requests.CPUMilli*headroom+99)/100 + r.SystemReserve.CPUMilli, MemoryMi: (r.Requests.MemoryMi*headroom+99)/100 + r.SystemReserve.MemoryMi, DiskGi: ((onNodeDiskGi)*headroom+99)/100 + r.SystemReserve.DiskGi}
	if r.Declared.CPUMilli < r.Required.CPUMilli || r.Declared.MemoryMi < r.Required.MemoryMi || r.Declared.DiskGi < r.Required.DiskGi {
		r.Reasons = append(r.Reasons, "DECLARED_CAPACITY_INSUFFICIENT")
	}
	sort.Strings(r.Reasons)
	r.Reasons = compactCapacityReasons(r.Reasons)
	r.FitsDeclaredCapacity = len(r.Reasons) == 0
	// Install authorization is a distinct, evidence-backed decision. Only a
	// caller that supplies certified + material-verified evidence can authorize
	// an install; otherwise the report stays explicitly unauthorized.
	authorized := false
	if opts.Authorization != nil {
		switch {
		case len(opts.Authorization.Rejections) > 0:
			r.Reasons = append(r.Reasons, "INSTALL_AUTHORIZATION_DENIED:"+strings.Join(opts.Authorization.Rejections, ","))
		case !opts.Authorization.Certified:
			r.Reasons = append(r.Reasons, "INSTALL_AUTHORIZATION_DENIED")
		case !opts.Authorization.MaterialVerified:
			r.Reasons = append(r.Reasons, "OFFLINE_MATERIAL_UNVERIFIED")
		default:
			authorized = r.FitsDeclaredCapacity
			if !authorized {
				r.Reasons = append(r.Reasons, "INSTALL_CAPACITY_BLOCKED")
			}
		}
		sort.Strings(r.Reasons)
		r.Reasons = compactCapacityReasons(r.Reasons)
		r.FitsDeclaredCapacity = len(r.Reasons) == 0
	}
	r.InstallAuthorized = authorized
	return r, nil
}

func compactCapacityReasons(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
