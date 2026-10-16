// Package toolkit defines the deployment configuration and evidence schemas for
// the HNB installer (hnbctl). The schemas are the single shared contract between
// the CLI, the portal install wizard and future automation. Strict decoding
// rejects unknown fields so unsupported combinations fail before any side effect.
package toolkit

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	APIVersion = "deployment.hnb.io/v1alpha1"
	Kind       = "DeploymentConfig"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var sshUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]*\$?$`)

// databaseIdent matches a PostgreSQL identifier. It is deliberately not the
// Kubernetes DNS rule: database and role names legitimately contain underscores
// and are truncated at 63 bytes by the server, not at 253 by Kubernetes.
var databaseIdent = regexp.MustCompile(`^[a-z_][a-z0-9_$]*$`)

func validDatabaseIdent(name string) bool {
	return name != "" && len(name) <= 63 && databaseIdent.MatchString(name)
}

func validName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) > 63 || !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func validLocalPath(path string) bool {
	return strings.TrimSpace(path) != "" && !strings.ContainsAny(path, "\r\n\x00") && !strings.Contains(path, "://")
}

func validSecretRef(ref string) bool {
	parts := strings.Split(ref, "/")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if !validName(part) {
			return false
		}
	}
	return true
}

// Mode selects how the installer interacts with a target.
type Mode string

const (
	ModeBootstrap Mode = "bootstrap" // dedicated, not-yet-managed HNB management base
	ModeExisting  Mode = "existing"  // install HNB into an externally managed cluster
	ModeManaged   Mode = "managed"   // platform online; every change goes through the server
)

// Profile is one of the four certified deployment tiers.
type Profile string

const (
	ProfileMinimal    Profile = "Minimal"
	ProfileLiteHA     Profile = "LiteHA"
	ProfileStandardHA Profile = "StandardHA"
	ProfileEnterprise Profile = "Enterprise"
)

// NodeRole identifies a bootstrap node role.
type NodeRole string

const (
	RoleControlPlane NodeRole = "control-plane"
	RoleWorker       NodeRole = "worker"
)

// Strategy selects between provisioning a new cluster and using an existing one.
type Strategy string

const (
	StrategyProvision Strategy = "provision"
	StrategyExisting  Strategy = "existing"
)

// ArtifactMode selects how install materials are sourced.
type ArtifactMode string

const (
	ArtifactsOnline  ArtifactMode = "online"
	ArtifactsMirror  ArtifactMode = "mirror"
	ArtifactsOffline ArtifactMode = "offline"
)

// DeploymentConfig is the versioned, validated installer input.
type DeploymentConfig struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Metadata   Metadata   `yaml:"metadata" json:"metadata"`
	Spec       ConfigSpec `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name string `yaml:"name" json:"name"`
}

type ConfigSpec struct {
	Mode        Mode            `yaml:"mode" json:"mode"`
	Profile     Profile         `yaml:"profile" json:"profile"`
	ReleaseLock string          `yaml:"releaseLock" json:"releaseLock"`
	Cluster     ClusterConfig   `yaml:"cluster" json:"cluster"`
	Platform    PlatformConfig  `yaml:"platform" json:"platform"`
	Artifacts   ArtifactsConfig `yaml:"artifacts" json:"artifacts"`
	Timeouts    TimeoutConfig   `yaml:"timeouts" json:"timeouts"`
}

type ClusterConfig struct {
	Strategy          Strategy      `yaml:"strategy" json:"strategy"`
	Distribution      string        `yaml:"distribution" json:"distribution"`
	KubernetesVersion string        `yaml:"kubernetesVersion" json:"kubernetesVersion"`
	APIEndpoint       string        `yaml:"apiEndpoint" json:"apiEndpoint"`
	KubeconfigFile    string        `yaml:"kubeconfigFile" json:"kubeconfigFile,omitempty"`
	Nodes             []NodeConfig  `yaml:"nodes" json:"nodes"`
	Network           NetworkConfig `yaml:"network" json:"network"`
}

type NodeConfig struct {
	Name            string     `yaml:"name" json:"name"`
	Roles           []NodeRole `yaml:"roles" json:"roles"`
	SSHAddress      string     `yaml:"sshAddress" json:"sshAddress"`
	InternalAddress string     `yaml:"internalAddress" json:"internalAddress"`
	User            string     `yaml:"user" json:"user"`
	SSHKeyFile      string     `yaml:"sshKeyFile" json:"sshKeyFile"`
	SSHAuth         string     `yaml:"sshAuth,omitempty" json:"sshAuth,omitempty"`
}

type NetworkConfig struct {
	Plugin      string `yaml:"plugin" json:"plugin"`
	Version     string `yaml:"version" json:"version"`
	PodCIDR     string `yaml:"podCIDR" json:"podCIDR"`
	ServiceCIDR string `yaml:"serviceCIDR" json:"serviceCIDR"`
	RoutingMode string `yaml:"routingMode" json:"routingMode"`
}

type PlatformConfig struct {
	Namespace      string        `yaml:"namespace" json:"namespace"`
	PublicBaseURL  string        `yaml:"publicBaseURL" json:"publicBaseURL"`
	TLS            TLSConfig     `yaml:"tls" json:"tls"`
	Storage        StorageConfig `yaml:"storage" json:"storage"`
	Infrastructure InfraConfig   `yaml:"infrastructure" json:"infrastructure"`
}

type TLSConfig struct {
	ExistingSecret string `yaml:"existingSecret" json:"existingSecret"`
}

type StorageConfig struct {
	ClassName string `yaml:"className" json:"className"`
}

type InfraConfig struct {
	PostgreSQL InfraBacking `yaml:"postgresql" json:"postgresql"`
	NATS       InfraBacking `yaml:"nats" json:"nats"`
	// Planes are the independently owned data domains. Each plane gets its own
	// database/account so one plane's compromise or owner cannot reach another's
	// tables. Sharing one credential across planes defeats that boundary.
	Planes []PlaneConfig `yaml:"planes" json:"planes"`
}

// PlaneConfig declares one owned plane and how its database/identity is bound.
type PlaneConfig struct {
	Name string `yaml:"name" json:"name"` // platform | market | artifact
	// Database is the plane's own database inside the PostgreSQL backing.
	Database string `yaml:"database" json:"database"`
	// Role is the plane's own least-privilege account name.
	Role string `yaml:"role" json:"role"`
	// SecretRef names the Secret holding this plane's credential. It must be
	// distinct from every other plane's reference in both backings.
	PostgreSQLSecretRef string `yaml:"postgresqlSecretRef" json:"postgresqlSecretRef"`
	NATSSecretRef       string `yaml:"natsSecretRef" json:"natsSecretRef"`
}

type InfraBacking struct {
	Mode      string `yaml:"mode" json:"mode"` // bundled | external
	SecretRef string `yaml:"secretRef" json:"secretRef,omitempty"`
}

type ArtifactsConfig struct {
	Mode       ArtifactMode `yaml:"mode" json:"mode"`
	BundlePath string       `yaml:"bundlePath" json:"bundlePath,omitempty"`
}

type TimeoutConfig struct {
	Download  time.Duration `yaml:"download" json:"download"`
	Readiness time.Duration `yaml:"readiness" json:"readiness"`
}

// UnmarshalYAML overrides time.Duration decoding so bare durations such as
// "15m" parse without requiring a string type change.
func (t *TimeoutConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("timeouts must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key != "download" && key != "readiness" {
			return fmt.Errorf("unknown timeouts field %q", key)
		}
	}
	var raw struct {
		Download  string `yaml:"download"`
		Readiness string `yaml:"readiness"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Download != "" {
		d, err := time.ParseDuration(raw.Download)
		if err != nil {
			return fmt.Errorf("timeouts.download: %w", err)
		}
		t.Download = d
	}
	if raw.Readiness != "" {
		d, err := time.ParseDuration(raw.Readiness)
		if err != nil {
			return fmt.Errorf("timeouts.readiness: %w", err)
		}
		t.Readiness = d
	}
	return nil
}

// DecodeConfig parses a DeploymentConfig with strict unknown-field rejection.
func DecodeConfig(data []byte) (*DeploymentConfig, error) {
	var cfg DeploymentConfig
	if err := decodeStrict(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate performs the full side-effect-free input validation. It returns a
// problem list; an empty list means the config is accepted by the toolkit.
func (c *DeploymentConfig) Validate(ev *EvidenceMatrix) []string {
	return c.validate(ev, false)
}

// ValidatePreparation permits an explicit registry mapping for local material
// preparation only. It does not authorize node writes or mirror-mode installation.
func (c *DeploymentConfig) ValidatePreparation(ev *EvidenceMatrix, mirror string) []string {
	problems := c.validate(ev, true)
	if c.Spec.Artifacts.Mode == ArtifactsMirror {
		if mirror == "" {
			problems = append(problems, "artifacts.mode=mirror requires --mirror-origin; no source fallback performed")
		} else if err := validateMirrorOrigin(mirror); err != nil {
			problems = append(problems, err.Error())
		}
	} else if mirror != "" {
		problems = append(problems, "--mirror-origin requires artifacts.mode=mirror")
	}
	return problems
}

func (c *DeploymentConfig) validate(ev *EvidenceMatrix, preparing bool) []string {
	var problems []string
	if c.APIVersion != APIVersion {
		problems = append(problems, fmt.Sprintf("unsupported apiVersion %q (want %q)", c.APIVersion, APIVersion))
	}
	if c.Kind != Kind {
		problems = append(problems, fmt.Sprintf("unsupported kind %q (want %q)", c.Kind, Kind))
	}
	if !validName(c.Metadata.Name) {
		problems = append(problems, "metadata.name must be a valid DNS name")
	}
	switch c.Spec.Mode {
	case ModeBootstrap, ModeExisting, ModeManaged:
	default:
		problems = append(problems, fmt.Sprintf("invalid mode %q", c.Spec.Mode))
	}
	if c.Spec.Mode == ModeManaged {
		problems = append(problems, "mode=managed requires a platform context and tenant-bound SecretReference contract that is not implemented; use server-authoritative cluster commands when available")
	}
	switch c.Spec.Profile {
	case ProfileMinimal, ProfileLiteHA, ProfileStandardHA, ProfileEnterprise:
	default:
		problems = append(problems, fmt.Sprintf("invalid profile %q", c.Spec.Profile))
	}
	if !validLocalPath(c.Spec.ReleaseLock) {
		problems = append(problems, "spec.releaseLock is required (a real release lock, not a handwritten example)")
	}
	problems = append(problems, c.Spec.Cluster.validate(c.Spec.Mode, ev)...)
	problems = append(problems, c.Spec.Platform.validate()...)
	problems = append(problems, c.Spec.Artifacts.validate(preparing)...)
	problems = append(problems, c.Spec.Timeouts.validate()...)
	return problems
}

func (cc ClusterConfig) validate(mode Mode, ev *EvidenceMatrix) []string {
	var problems []string
	if (mode == ModeExisting && cc.Strategy != StrategyExisting) || (mode == ModeBootstrap && cc.Strategy != StrategyProvision) {
		problems = append(problems, "mode and cluster.strategy conflict: existing requires existing; bootstrap requires provision")
	}
	switch cc.Strategy {
	case StrategyProvision, StrategyExisting:
	default:
		problems = append(problems, fmt.Sprintf("invalid cluster.strategy %q", cc.Strategy))
	}
	if cc.Strategy == StrategyProvision {
		if cc.KubeconfigFile != "" {
			problems = append(problems, "provision must not include an existing kubeconfigFile")
		}
		if cc.Distribution != "kubeadm" {
			problems = append(problems, fmt.Sprintf("unsupported distribution %q (only kubeadm is certified)", cc.Distribution))
		}
		if cc.KubernetesVersion == "" {
			problems = append(problems, "cluster.kubernetesVersion is required")
		} else if ev == nil || !ev.VersionCertified(cc.KubernetesVersion) {
			problems = append(problems, fmt.Sprintf("kubernetes version %q lacks certified evidence", cc.KubernetesVersion))
		}
		if len(cc.Nodes) == 0 {
			problems = append(problems, "cluster.strategy=provision requires at least one node")
		}
		if len(cc.Nodes) > 2 {
			problems = append(problems, "provision candidate evidence covers at most one control-plane and one worker")
		}
		controlPlanes := 0
		seen := map[string]bool{}
		seenSSH, seenInternal := map[string]bool{}, map[string]bool{}
		var controlPlaneAddress string
		for i, n := range cc.Nodes {
			if !validName(n.Name) {
				problems = append(problems, fmt.Sprintf("cluster.nodes[%d].name must be a valid DNS name", i))
			} else if seen[n.Name] {
				problems = append(problems, fmt.Sprintf("duplicate node name %q", n.Name))
			}
			seen[n.Name] = true
			if len(n.Roles) != 1 {
				problems = append(problems, fmt.Sprintf("node %q must have exactly one declared role", n.Name))
			}
			seenRoles := map[NodeRole]bool{}
			for _, r := range n.Roles {
				if seenRoles[r] {
					problems = append(problems, fmt.Sprintf("node %q has duplicate role %q", n.Name, r))
				}
				seenRoles[r] = true
				switch r {
				case RoleControlPlane:
					controlPlanes++
					controlPlaneAddress = n.InternalAddress
				case RoleWorker:
				default:
					problems = append(problems, fmt.Sprintf("node %q has invalid role %q", n.Name, r))
				}
			}
			if !usableIPv4(n.SSHAddress) {
				problems = append(problems, fmt.Sprintf("node %q sshAddress must be a unicast IPv4 address (IPv6 route lacks evidence)", n.Name))
			}
			if !usableIPv4(n.InternalAddress) {
				problems = append(problems, fmt.Sprintf("node %q internalAddress must be a unicast IPv4 address", n.Name))
			}
			for _, entry := range []struct {
				address string
				seen    map[string]bool
				field   string
			}{
				{n.SSHAddress, seenSSH, "sshAddress"}, {n.InternalAddress, seenInternal, "internalAddress"},
			} {
				if ip := net.ParseIP(entry.address); ip != nil {
					key := ip.String()
					if entry.seen[key] {
						problems = append(problems, fmt.Sprintf("node %q duplicates %s", n.Name, entry.field))
					}
					entry.seen[key] = true
				}
			}
			if !sshUser.MatchString(n.User) {
				problems = append(problems, fmt.Sprintf("node %q user must be a valid SSH account name", n.Name))
			}
			if n.SSHAuth != "" && n.SSHAuth != "key" && n.SSHAuth != "password" {
				problems = append(problems, fmt.Sprintf("node %q sshAuth must be key or password", n.Name))
			}
			if n.SSHAuth != "password" && !validLocalPath(n.SSHKeyFile) {
				problems = append(problems, fmt.Sprintf("node %q sshKeyFile is required", n.Name))
			}
		}
		if cc.APIEndpoint == "" {
			problems = append(problems, "cluster.apiEndpoint is required")
		} else if host, _, err := net.SplitHostPort(cc.APIEndpoint); err == nil && host != controlPlaneAddress {
			problems = append(problems, "cluster.apiEndpoint must use the single control-plane internalAddress; other endpoint topologies lack evidence")
		}
		if controlPlanes != 1 {
			problems = append(problems, fmt.Sprintf("provision currently supports exactly one control-plane node (got %d)", controlPlanes))
		}
		if mode == ModeManaged {
			problems = append(problems, "cluster.strategy=provision is not valid for mode=managed (managed changes go through the server)")
		}
		problems = append(problems, cc.Network.validate(ev)...)
		for _, node := range cc.Nodes {
			for _, cidr := range []string{cc.Network.PodCIDR, cc.Network.ServiceCIDR} {
				_, network, err := net.ParseCIDR(cidr)
				if err == nil && (network.Contains(net.ParseIP(node.InternalAddress)) || network.Contains(net.ParseIP(node.SSHAddress))) {
					problems = append(problems, fmt.Sprintf("node %q address overlaps Pod/Service CIDR", node.Name))
				}
			}
		}
	} else { // existing
		if !validLocalPath(cc.KubeconfigFile) {
			problems = append(problems, "cluster.strategy=existing requires cluster.kubeconfigFile")
		}
		if len(cc.Nodes) > 0 {
			problems = append(problems, "cluster.strategy=existing must not list bootstrap nodes")
		}
		if cc.Distribution != "" || cc.KubernetesVersion != "" || cc.APIEndpoint != "" || cc.Network != (NetworkConfig{}) {
			problems = append(problems, "existing must not include provision distribution/version/endpoint/network settings; the external owner controls them")
		}
	}
	if cc.APIEndpoint != "" {
		host, port, err := net.SplitHostPort(cc.APIEndpoint)
		if err != nil || !usableIPv4(host) || port != "6443" {
			problems = append(problems, fmt.Sprintf("cluster.apiEndpoint must be an IP address with port 6443 (got %q)", cc.APIEndpoint))
		}
	}
	return problems
}

func usableIPv4(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil && ip.To4() != nil && ip.IsGlobalUnicast()
}

func (nc NetworkConfig) validate(ev *EvidenceMatrix) []string {
	var problems []string
	if nc.Plugin != "cilium" {
		problems = append(problems, fmt.Sprintf("unsupported network plugin %q (only cilium is certified)", nc.Plugin))
	}
	if nc.Version == "" {
		problems = append(problems, "network.version is required")
	} else if ev == nil || !ev.CNICertified(nc.Plugin, nc.Version) {
		problems = append(problems, fmt.Sprintf("cilium version %q lacks certified evidence", nc.Version))
	}
	if _, _, err := net.ParseCIDR(nc.PodCIDR); err != nil {
		problems = append(problems, fmt.Sprintf("invalid podCIDR %q", nc.PodCIDR))
	}
	if _, _, err := net.ParseCIDR(nc.ServiceCIDR); err != nil {
		problems = append(problems, fmt.Sprintf("invalid serviceCIDR %q", nc.ServiceCIDR))
	}
	for _, cidr := range []string{nc.PodCIDR, nc.ServiceCIDR} {
		ip, network, err := net.ParseCIDR(cidr)
		if err == nil && (ip.To4() == nil || !ip.IsGlobalUnicast() || !ip.Equal(network.IP)) {
			problems = append(problems, "Pod/Service CIDR must be canonical unicast IPv4 networks; IPv6 lacks evidence")
		}
	}
	if nc.PodCIDR != "" && nc.ServiceCIDR != "" {
		_, pod, _ := net.ParseCIDR(nc.PodCIDR)
		_, svc, _ := net.ParseCIDR(nc.ServiceCIDR)
		if pod != nil && svc != nil && (pod.Contains(svc.IP) || svc.Contains(pod.IP)) {
			problems = append(problems, "podCIDR and serviceCIDR must not overlap")
		}
	}
	if nc.RoutingMode != "native" {
		problems = append(problems, fmt.Sprintf("unsupported routingMode %q (only native is certified)", nc.RoutingMode))
	}
	return problems
}

func (pc PlatformConfig) validate() []string {
	var problems []string
	if len(pc.Namespace) > 63 || !dnsLabel.MatchString(pc.Namespace) {
		problems = append(problems, "platform.namespace must be a valid DNS label")
	}
	if pc.PublicBaseURL == "" {
		problems = append(problems, "platform.publicBaseURL is required (there is no default fake domain)")
	}
	u, err := url.Parse(pc.PublicBaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !validName(u.Hostname()) {
		problems = append(problems, "platform.publicBaseURL must be an HTTPS origin without credentials, query, fragment or path")
	} else if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			problems = append(problems, "platform.publicBaseURL has an invalid port")
		}
	}
	if !validName(pc.TLS.ExistingSecret) {
		problems = append(problems, "platform.tls.existingSecret is required and must name a Secret")
	}
	if !validName(pc.Storage.ClassName) {
		problems = append(problems, "platform.storage.className is required and must name a StorageClass")
	}
	if pc.Infrastructure.PostgreSQL.Mode != "bundled" && pc.Infrastructure.PostgreSQL.Mode != "external" {
		problems = append(problems, fmt.Sprintf("invalid infrastructure.postgresql.mode %q", pc.Infrastructure.PostgreSQL.Mode))
	}
	if pc.Infrastructure.NATS.Mode != "bundled" && pc.Infrastructure.NATS.Mode != "external" {
		problems = append(problems, fmt.Sprintf("invalid infrastructure.nats.mode %q", pc.Infrastructure.NATS.Mode))
	}
	if pc.Infrastructure.PostgreSQL.Mode == "external" && !validSecretRef(pc.Infrastructure.PostgreSQL.SecretRef) {
		problems = append(problems, "external postgresql requires infrastructure.postgresql.secretRef")
	}
	if pc.Infrastructure.NATS.Mode == "external" && !validSecretRef(pc.Infrastructure.NATS.SecretRef) {
		problems = append(problems, "external nats requires infrastructure.nats.secretRef")
	}
	for _, backing := range []InfraBacking{pc.Infrastructure.PostgreSQL, pc.Infrastructure.NATS} {
		if backing.Mode == "bundled" && backing.SecretRef != "" {
			problems = append(problems, "bundled infrastructure must not include an external secretRef")
		}
	}
	problems = append(problems, pc.Infrastructure.validatePlanes()...)
	return problems
}

// RequiredPlanes are the data domains that must each own their identity.
var RequiredPlanes = []string{"platform", "market", "artifact"}

// validatePlanes enforces one independent identity per owned plane. The failure
// this prevents is the important one: a single shared account would let the
// market plane read or write platform tables, so cross-owner access would not
// actually be denied anywhere below the application layer.
func (ic InfraConfig) validatePlanes() []string {
	var problems []string
	seen := map[string]bool{}
	for i, plane := range ic.Planes {
		switch {
		case plane.Name == "":
			problems = append(problems, fmt.Sprintf("infrastructure.planes[%d].name is required", i))
			continue
		case !containsStr(RequiredPlanes, plane.Name):
			problems = append(problems, fmt.Sprintf("infrastructure.planes[%d].name %q is not an owned plane", i, plane.Name))
			continue
		}
		if seen[plane.Name] {
			problems = append(problems, fmt.Sprintf("infrastructure.planes declares %q more than once", plane.Name))
			continue
		}
		seen[plane.Name] = true
		if !validDatabaseIdent(plane.Database) || !validDatabaseIdent(plane.Role) {
			problems = append(problems, fmt.Sprintf("plane %q requires its own database and role name", plane.Name))
		}
		if !validSecretRef(plane.PostgreSQLSecretRef) {
			problems = append(problems, fmt.Sprintf("plane %q requires infrastructure.planes.postgresqlSecretRef", plane.Name))
		}
		if !validSecretRef(plane.NATSSecretRef) {
			problems = append(problems, fmt.Sprintf("plane %q requires infrastructure.planes.natsSecretRef", plane.Name))
		}
	}
	for _, required := range RequiredPlanes {
		if !seen[required] {
			problems = append(problems, "infrastructure.planes is missing the "+required+" plane")
		}
	}
	// Identities must be pairwise distinct across planes, otherwise the boundary
	// only appears to exist.
	for i := 0; i < len(ic.Planes); i++ {
		for j := i + 1; j < len(ic.Planes); j++ {
			a, b := ic.Planes[i], ic.Planes[j]
			if a.PostgreSQLSecretRef != "" && a.PostgreSQLSecretRef == b.PostgreSQLSecretRef {
				problems = append(problems, fmt.Sprintf("planes %q and %q share one postgresql credential", a.Name, b.Name))
			}
			if a.NATSSecretRef != "" && a.NATSSecretRef == b.NATSSecretRef {
				problems = append(problems, fmt.Sprintf("planes %q and %q share one nats credential", a.Name, b.Name))
			}
			if a.Role != "" && a.Role == b.Role {
				problems = append(problems, fmt.Sprintf("planes %q and %q share one database role", a.Name, b.Name))
			}
			if a.Database != "" && a.Database == b.Database {
				problems = append(problems, fmt.Sprintf("planes %q and %q share one database", a.Name, b.Name))
			}
		}
	}
	return problems
}

func (ac ArtifactsConfig) validate(preparing bool) []string {
	var problems []string
	switch ac.Mode {
	case ArtifactsOnline, ArtifactsMirror, ArtifactsOffline:
	default:
		problems = append(problems, fmt.Sprintf("invalid artifacts.mode %q", ac.Mode))
	}
	if ac.Mode == ArtifactsOffline && ac.BundlePath == "" {
		problems = append(problems, "artifacts.mode=offline requires artifacts.bundlePath")
	}
	if ac.BundlePath != "" && (!validLocalPath(ac.BundlePath) || ac.Mode != ArtifactsOffline) {
		problems = append(problems, "artifacts.bundlePath must be a local path and only applies to offline mode")
	}
	if ac.Mode == ArtifactsMirror && !preparing {
		problems = append(problems, "mirror mode is supported only for local prepare with an explicit --mirror-origin; installation source mappings are not implemented")
	}
	return problems
}

func (tc TimeoutConfig) validate() []string {
	var problems []string
	if tc.Download <= 0 {
		problems = append(problems, "timeouts.download must be a positive duration")
	}
	if tc.Readiness <= 0 {
		problems = append(problems, "timeouts.readiness must be a positive duration")
	}
	return problems
}
