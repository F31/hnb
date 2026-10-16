package toolkit

import (
	"fmt"
	"sort"
	"strings"
)

// The evidenced bootstrap recipes mirror what the shipped provision scripts
// actually do. Every entry here is traceable to those scripts:
//   - pkg/provision/bootstrap.go
//     (PrereqScript, RuntimeConfigScript, NetworkScript)
//   - pkg/provision/orchestrator.go
//     (Helm preinstalled-requirement gate)
const (
	EvidencedKubernetesVersion = "1.36.5"
	EvidencedCiliumVersion     = "1.20.1"
	// CiliumChartEnvoyTag is the Envoy image tag pinned by the Cilium 1.20.1
	// chart. It is independent of the Cilium version and must be re-verified
	// whenever the chart is upgraded.
	CiliumChartEnvoyTag = "v1.37.5-1786810558-766ccfb37260a43e9d228837aa84ce3faf9f64e7"
)

// kubeadm pins the CoreDNS tag per Kubernetes release. These are the tags the
// shipped kubeadm actually requests (verified with `kubeadm config images list`
// against v1.36.5); guessing them causes a preflight ImagePull failure.
var kubeadmImageTags = map[string]struct {
	Coredns string
	Etcd    string
	Pause   string
}{
	"1.36.5": {Coredns: "1.14.2", Etcd: "3.6.8-0", Pause: "3.10.2"},
}

func corednsVersion(kubernetesVersion string) string {
	return kubeadmImageTags[kubernetesVersion].Coredns
}

// RecipePackage is a package the real scripts install or require on a node.
// Source distinguishes lock-pinned artifacts from packages resolved live from an
// OS or Kubernetes channel, which a certified closure must still pin.
type RecipePackage struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
	Source  string `yaml:"source" json:"source"` // lock | os-channel | k8s-channel
	Purpose string `yaml:"purpose" json:"purpose"`
}

// RecipeImage is a container image the real scripts make the node depend on.
type RecipeImage struct {
	Purpose string `yaml:"purpose" json:"purpose"`
	Ref     string `yaml:"ref" json:"ref"`
}

// RecipeTool is a host tool the real scripts require but do not install.
type RecipeTool struct {
	Name         string `yaml:"name" json:"name"`
	Purpose      string `yaml:"purpose" json:"purpose"`
	Preinstalled bool   `yaml:"preinstalled" json:"preinstalled"`
}

// NodeRecipe is the resolved OS/arch node dependency declaration.
type NodeRecipe struct {
	OS                string          `yaml:"os" json:"os"`
	Arch              string          `yaml:"arch" json:"arch"`
	PackageManager    string          `yaml:"packageManager" json:"packageManager"`
	KubernetesVersion string          `yaml:"kubernetesVersion" json:"kubernetesVersion"`
	NetworkPlugin     string          `yaml:"networkPlugin" json:"networkPlugin"`
	NetworkVersion    string          `yaml:"networkVersion" json:"networkVersion"`
	Packages          []RecipePackage `yaml:"packages" json:"packages"`
	Images            []RecipeImage   `yaml:"images" json:"images"`
	Tools             []RecipeTool    `yaml:"tools" json:"tools"`
}

var recipeBasePackages = []RecipePackage{
	{Name: "kubelet", Version: EvidencedKubernetesVersion, Source: "k8s-channel", Purpose: "node agent"},
	{Name: "kubeadm", Version: EvidencedKubernetesVersion, Source: "k8s-channel", Purpose: "cluster control plane"},
	{Name: "kubectl", Version: EvidencedKubernetesVersion, Source: "k8s-channel", Purpose: "cluster operations and acceptance"},
	{Name: "containerd", Source: "os-channel", Purpose: "CRI runtime"},
	// RuntimeConfigScript requires python3 on every distribution: it merges the
	// containerd import without replacing administrator configuration.
	{Name: "python3", Source: "os-channel", Purpose: "merges the containerd runtime import without replacing admin config"},
}

// recipeDebianPackages is what the APT branch of PrereqScript installs before
// it imports the Kubernetes repository key.
var recipeDebianPackages = []RecipePackage{
	{Name: "apt-transport-https", Source: "os-channel", Purpose: "HTTPS package transport"},
	{Name: "ca-certificates", Source: "os-channel", Purpose: "TLS trust for package and image sources"},
	{Name: "curl", Source: "os-channel", Purpose: "fetches the Kubernetes repository signing key"},
	{Name: "gpg", Source: "os-channel", Purpose: "dearmors the Kubernetes repository key"},
}

// cniImages are pulled by the Helm-driven network stage of the real scripts.
// Both resolve through the platform-controlled CNI image repository; bootstrap
// never reaches an upstream public registry implicitly.
//
// Cilium 1.20.1 runs a Cilium Envoy sidecar whose tag is pinned by the chart
// itself, not derived from the Cilium version. It is a real node image
// dependency and must resolve through the platform repository as well.
func cniImages(repository, version string) []RecipeImage {
	return []RecipeImage{
		{Purpose: "cni-datapath", Ref: repository + "/cilium:v" + version},
		{Purpose: "cni-operator", Ref: repository + "/cilium-operator:v" + version},
		{Purpose: "cni-l7-proxy", Ref: repository + "/cilium-envoy:" + CiliumChartEnvoyTag},
	}
}

// ResolveNodeRecipe derives the node dependency declaration for one OS/arch.
// The caller supplies the platform-controlled repositories; no upstream public
// registry is ever assumed.
func ResolveNodeRecipe(os, arch, networkPlugin, networkVersion, kubernetesVersion, sandboxImage, imageRepository, cniImageRepository string) (*NodeRecipe, error) {
	if arch != "amd64" {
		return nil, fmt.Errorf("node recipe is only evidenced for amd64")
	}
	recipe := &NodeRecipe{OS: os, Arch: arch, KubernetesVersion: kubernetesVersion, NetworkPlugin: networkPlugin, NetworkVersion: networkVersion, Packages: append([]RecipePackage(nil), recipeBasePackages...)}
	adapter, ok := LookupNodeOSAdapter(os)
	if !ok {
		return nil, fmt.Errorf("no node OS adapter for %s", os)
	}
	recipe.PackageManager = adapter.PackageManager
	for _, name := range adapter.BootstrapPackages {
		dependency := RecipePackage{Name: name, Source: "os-channel", Purpose: "OS bootstrap prerequisite"}
		for _, pkg := range recipeDebianPackages {
			if pkg.Name == name {
				dependency = pkg
				break
			}
		}
		recipe.Packages = append(recipe.Packages, dependency)
	}
	if networkPlugin != "cilium" {
		return nil, fmt.Errorf("only the Cilium recipe has node evidence")
	}
	if !tagPattern.MatchString(strings.TrimPrefix(networkVersion, "v")) || strings.EqualFold(strings.TrimPrefix(networkVersion, "v"), "latest") {
		return nil, fmt.Errorf("network version must be an exact version")
	}
	// The CNI repository is required so node images resolve through the
	// platform registry rather than a hardcoded upstream.
	if _, known := kubeadmImageTags[kubernetesVersion]; !known {
		return nil, fmt.Errorf("no verified kubeadm image tag set for the requested Kubernetes version")
	}
	if cniImageRepository == "" || !validRepositoryPrefix(cniImageRepository) {
		return nil, fmt.Errorf("an explicit CNI image repository is required; bootstrap must not fall back to a public upstream")
	}
	recipe.Images = cniImages(cniImageRepository, strings.TrimPrefix(networkVersion, "v"))
	// kubeadm and kubelet pull control-plane images from the configured
	// repository; CoreDNS is part of the cluster the acceptance gate waits for.
	if imageRepository == "" || !validRepositoryPrefix(imageRepository) {
		return nil, fmt.Errorf("an explicit image repository is required; registry.k8s.io is not assumed")
	}
	recipe.Images = append(recipe.Images,
		RecipeImage{Purpose: "sandbox", Ref: sandboxImage},
		RecipeImage{Purpose: "control-plane", Ref: imageRepository + "/kube-apiserver:v" + kubernetesVersion},
		RecipeImage{Purpose: "control-plane", Ref: imageRepository + "/kube-controller-manager:v" + kubernetesVersion},
		RecipeImage{Purpose: "control-plane", Ref: imageRepository + "/kube-scheduler:v" + kubernetesVersion},
		RecipeImage{Purpose: "dns", Ref: imageRepository + "/coredns:v" + corednsVersion(kubernetesVersion)},
	)
	if sandboxImage == "" || !validImageRef(sandboxImage) {
		return nil, fmt.Errorf("sandbox image must be an explicit non-latest reference")
	}
	recipe.Tools = []RecipeTool{
		{Name: "helm", Purpose: "installs the Cilium chart", Preinstalled: true},
		{Name: "sudo", Purpose: "runs bootstrap stages when the SSH account is not root", Preinstalled: true},
	}
	sort.Slice(recipe.Images, func(i, j int) bool { return recipe.Images[i].Purpose < recipe.Images[j].Purpose })
	return recipe, nil
}

// validRepositoryPrefix checks a registry/repository prefix that node images
// will be appended to. An explicit repository is required: a bare registry host
// would let callers redirect the whole node image set to a public upstream.
func validRepositoryPrefix(prefix string) bool {
	if strings.Contains(prefix, "@") || strings.HasSuffix(prefix, ":latest") {
		return false
	}
	if i := strings.LastIndex(prefix, ":"); i >= 0 && !strings.Contains(prefix[i+1:], "/") {
		if !tagPattern.MatchString(prefix[i+1:]) || strings.EqualFold(prefix[i+1:], "latest") {
			return false
		}
	}
	registry, repository, ok := strings.Cut(prefix, "/")
	if !ok || repository == "" {
		return false
	}
	return redirectHostPattern.MatchString(registry) && validImageRef(prefix+"/probe")
}

func validImageRef(ref string) bool {
	registry, repository, _ := strings.Cut(ref, "/")
	if !redirectHostPattern.MatchString(registry) {
		return false
	}
	if i := strings.LastIndex(repository, ":"); i >= 0 {
		tag := repository[i+1:]
		if !tagPattern.MatchString(tag) || strings.EqualFold(tag, "latest") {
			return false
		}
		repository = repository[:i]
	}
	if strings.Contains(repository, "@") {
		return false
	}
	for _, part := range strings.Split(repository, "/") {
		if !repositorySegment.MatchString(part) {
			return false
		}
	}
	return repository != ""
}

// CheckClosure compares a recipe against a candidate lock. Pinned identity is
// proven only for artifacts the lock actually pins; channel-resolved packages
// and preinstalled host tools stay explicitly unpinned and block certification.
func (r *NodeRecipe) CheckClosure(lock *ReleaseLock) []string {
	if lock == nil {
		return []string{"node recipe requires a candidate release lock"}
	}
	if r.KubernetesVersion != "" && !containsStr(lock.Compatibility.KubernetesVersions, r.KubernetesVersion) {
		return []string{"lock does not declare Kubernetes " + r.KubernetesVersion}
	}
	if !containsStr(lock.Compatibility.OS, r.OS) || !containsStr(lock.Compatibility.Arch, r.Arch) {
		return []string{"lock does not declare " + r.OS + "/" + r.Arch}
	}
	pinned := map[string]PinnedImage{}
	for _, image := range lock.Images {
		pinned[image.Name] = image
	}
	var gaps []string
	for _, image := range r.Images {
		found := false
		for _, candidate := range lock.Images {
			if candidate.Ref == image.Ref {
				found = true
			}
		}
		if !found {
			gaps = append(gaps, "node image "+image.Purpose+" ("+image.Ref+") is not in the lock closure")
			continue
		}
		_ = pinned
	}
	for _, pkg := range r.Packages {
		if pkg.Source != "lock" {
			gaps = append(gaps, "node package "+pkg.Name+" ("+pkg.Source+") has no release-lock identity")
		}
	}
	for _, tool := range r.Tools {
		if tool.Preinstalled {
			gaps = append(gaps, "node tool "+tool.Name+" is required preinstalled and is not pinned by the release")
		}
	}
	return gaps
}
