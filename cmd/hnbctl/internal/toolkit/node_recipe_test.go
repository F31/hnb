package toolkit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func evidencedRecipe(t *testing.T, os string) *NodeRecipe {
	t.Helper()
	recipe, err := ResolveNodeRecipe(os, "amd64", "cilium", "v"+EvidencedCiliumVersion, EvidencedKubernetesVersion,
		"registry.aliyuncs.com/google_containers/pause:3.10", "registry.aliyuncs.com/google_containers",
		"harbor.hnb.local/hnb/cilium")
	if err != nil {
		t.Fatal(err)
	}
	return recipe
}

func recipeImageRefs(recipe *NodeRecipe) map[string]string {
	refs := map[string]string{}
	for _, image := range recipe.Images {
		refs[image.Purpose] = image.Ref
	}
	return refs
}

func TestRecipeMatchesProvisionScripts(t *testing.T) {
	for _, os := range []string{"ubuntu-24.04", "ubuntu-26.04", "rocky-10.1"} {
		recipe := evidencedRecipe(t, os)
		names := map[string]RecipePackage{}
		for _, pkg := range recipe.Packages {
			names[pkg.Name] = pkg
		}
		// PrereqScript installs these on both evidenced distributions.
		for _, required := range []string{"kubelet", "kubeadm", "kubectl", "containerd"} {
			if _, ok := names[required]; !ok {
				t.Fatalf("%s recipe omits %s", os, required)
			}
		}
		// python3 is required by RuntimeConfigScript on both distributions.
		if _, ok := names["python3"]; !ok {
			t.Fatalf("%s recipe omits python3", os)
		}
		// The key/bootstrap tooling belongs to the APT branch of PrereqScript only.
		_, hasAPT := names["apt-transport-https"]
		if os == "ubuntu-24.04" && !hasAPT {
			t.Fatal("Ubuntu recipe omits the APT bootstrap packages")
		}
		if os == "ubuntu-26.04" && hasAPT {
			t.Fatal("APT 3 must not depend on the obsolete HTTPS transport package")
		}
		if os == "rocky-10.1" && hasAPT {
			t.Fatal("Rocky recipe must not claim APT-only packages")
		}
		if os == "ubuntu-24.04" && names["containerd"].Source != "os-channel" {
			t.Fatal("containerd must come from the OS channel")
		}
		if names["kubeadm"].Version != EvidencedKubernetesVersion {
			t.Fatal("kubeadm is not pinned to the evidenced version")
		}
		refs := recipeImageRefs(recipe)
		for _, required := range []string{"cni-datapath", "cni-operator", "sandbox"} {
			if !strings.Contains(refs[required], "cilium") && required != "sandbox" {
				t.Fatalf("%s image %s is not the Cilium reference", os, required)
			}
			if refs[required] == "" {
				t.Fatalf("%s recipe omits %s", os, required)
			}
		}
		// registry.k8s.io was unreachable during Rocky acceptance, so the
		// recipe must never fall back to it.
		for purpose, ref := range refs {
			if strings.HasPrefix(ref, "registry.k8s.io/") {
				t.Fatalf("%s (%s) uses the unevidenced default registry", os, purpose)
			}
		}
		if !strings.Contains(refs["sandbox"], "pause:3.10") {
			t.Fatal("sandbox image is not the evidenced pause image")
		}
	}
	// The Debian bootstrap tooling is APT-specific.
	recipe := evidencedRecipe(t, "ubuntu-24.04")
	rocky := evidencedRecipe(t, "rocky-10.1")
	if len(recipe.Packages) <= len(rocky.Packages) {
		t.Fatal("Ubuntu recipe must include the APT bootstrap packages")
	}
	if len(rocky.Tools) == 0 || !rocky.Tools[0].Preinstalled {
		t.Fatal("helm preinstalled requirement must be recorded")
	}
}

func TestRecipeRejectsUnsupportedInputs(t *testing.T) {
	const (
		pause   = "registry.aliyuncs.com/google_containers/pause:3.10"
		k8sRepo = "registry.aliyuncs.com/google_containers"
		cniRepo = "harbor.hnb.local/hnb/cilium"
	)
	for name, args := range map[string][]string{
		"unknown-os":        {"debian-12", "amd64", "cilium", "v1.20.1", "1.36.5", pause, k8sRepo, cniRepo},
		"arm":               {"ubuntu-24.04", "arm64", "cilium", "v1.20.1", "1.36.5", pause, k8sRepo, cniRepo},
		"flannel":           {"ubuntu-24.04", "amd64", "flannel", "v0.25.2", "1.36.5", pause, k8sRepo, cniRepo},
		"latest-network":    {"ubuntu-24.04", "amd64", "cilium", "latest", "1.36.5", pause, k8sRepo, cniRepo},
		"no-repository":     {"ubuntu-24.04", "amd64", "cilium", "v1.20.1", "1.36.5", pause, "", cniRepo},
		"latest-sandbox":    {"ubuntu-24.04", "amd64", "cilium", "v1.20.1", "1.36.5", "registry.aliyuncs.com/google_containers/pause:latest", k8sRepo, cniRepo},
		"bare-sandbox-name": {"ubuntu-24.04", "amd64", "cilium", "v1.20.1", "1.36.5", "pause", k8sRepo, cniRepo},
		// A Cilium recipe without a platform CNI repository would resolve node
		// images from a hardcoded public upstream.
		"no-cni-repository": {"ubuntu-24.04", "amd64", "cilium", "v1.20.1", "1.36.5", pause, k8sRepo, ""},
		"bare-cni-registry": {"ubuntu-24.04", "amd64", "cilium", "v1.20.1", "1.36.5", pause, k8sRepo, "quay.io"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveNodeRecipe(args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7])
			if err == nil {
				t.Fatal("unsupported recipe input accepted")
			}
		})
	}
	// Node CNI images must resolve through the platform repository, never quay.io.
	recipe := evidencedRecipe(t, "ubuntu-24.04")
	cniImages := 0
	for _, image := range recipe.Images {
		if strings.Contains(image.Ref, "quay.io") || strings.Contains(image.Ref, "registry.k8s.io") {
			t.Fatalf("node image %s still reaches a public upstream", image.Ref)
		}
		if strings.HasPrefix(image.Ref, "harbor.hnb.local/hnb/cilium/") {
			cniImages++
		}
	}
	if cniImages != 3 {
		t.Fatalf("CNI images must come from the platform repository: %+v", recipe.Images)
	}
}

func TestRecipeClosureReportsUnpinnedNodeDependencies(t *testing.T) {
	recipe := evidencedRecipe(t, "ubuntu-24.04")
	if gaps := recipe.CheckClosure(nil); len(gaps) == 0 {
		t.Fatal("recipe closure accepted without a lock")
	}
	// A lock that only pins the sandbox image still leaves the remaining node
	// dependencies unresolved; the gap must be explicit, not silently accepted.
	lock := &ReleaseLock{SchemaVersion: LockSchemaVersion, Images: []PinnedImage{
		{Name: "pause", Ref: "registry.aliyuncs.com/google_containers/pause:3.10", Digest: bodyDigest("pause")},
	}, Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}, KubernetesVersions: []string{EvidencedKubernetesVersion}}}
	gaps := recipe.CheckClosure(lock)
	joined := strings.Join(gaps, "\n")
	for _, required := range []string{"cni-datapath", "cni-operator", "cni-l7-proxy", "kube-apiserver", "coredns", "kubeadm", "containerd", "python3", "helm"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("closure gap missing %s: %v", required, gaps)
		}
	}
	if !strings.Contains(joined, "no release-lock identity") || !strings.Contains(joined, "preinstalled") {
		t.Fatal("channel-resolved and preinstalled dependencies must stay explicitly unpinned")
	}
	// The evidenced combination must not be reported as a compatibility gap.
	if strings.Contains(joined, "lock does not declare") {
		t.Fatal("compatible lock reported a declaration gap")
	}
	// The pinned sandbox image is satisfied; removing it must surface the gap.
	if strings.Contains(joined, "sandbox") {
		t.Fatal("a pinned, matching sandbox image must not be reported as a gap")
	}
	withoutSandbox := *lock
	withoutSandbox.Images = nil
	if gaps := recipe.CheckClosure(&withoutSandbox); !strings.Contains(strings.Join(gaps, "\n"), "sandbox") {
		t.Fatal("missing sandbox image was not reported")
	}
	// A lock pinning a different image for the same purpose must not satisfy it.
	otherRef := *lock
	otherRef.Images = []PinnedImage{{Name: "pause", Ref: "registry.aliyuncs.com/google_containers/pause:3.9", Digest: bodyDigest("other")}}
	if gaps := recipe.CheckClosure(&otherRef); !strings.Contains(strings.Join(gaps, "\n"), "sandbox") {
		t.Fatal("a different sandbox image satisfied the recipe")
	}
	other := &ReleaseLock{SchemaVersion: LockSchemaVersion, Compatibility: LockCompatibility{OS: []string{"rocky-10.1"}, Arch: []string{"arm64"}}}
	if gaps := evidencedRecipe(t, "ubuntu-24.04").CheckClosure(other); !strings.Contains(strings.Join(gaps, "\n"), "does not declare") {
		t.Fatal("incompatible lock accepted for the recipe")
	}
}

func TestRecipeRealNodeImageResolution(t *testing.T) {
	refs := os.Getenv("HNB_NODE_RECIPE_E2E_IMAGES")
	if refs == "" {
		t.Skip("set HNB_NODE_RECIPE_E2E_IMAGES to a comma-separated list of node images")
	}
	resolver := NewRegistryResolver(nil)
	if err := resolver.DeclareRedirectOrigins(strings.Split(os.Getenv("HNB_REGISTRY_IDENTITY_E2E_REDIRECT_ORIGINS"), ",")); err != nil {
		t.Fatal(err)
	}
	for i, ref := range strings.Split(refs, ",") {
		name := fmt.Sprintf("node-recipe-%d", i)
		image, err := resolver.Resolve(t.Context(), name, ref)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		t.Logf("%s -> root=%s manifest=%s platform=%s", ref, image.Digest, image.ManifestDigest, image.Platform)
		if image.Platform != "linux/amd64" {
			t.Fatalf("%s is not linux/amd64", ref)
		}
		if image.ManifestDigest == placeholderDigest || !digestPattern.MatchString(image.ManifestDigest) {
			t.Fatalf("%s has no real platform manifest identity", ref)
		}
	}
}
