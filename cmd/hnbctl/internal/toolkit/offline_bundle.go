package toolkit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const OfflineBundleManifest = "offline-bundle/v1"

type BundleEntry struct {
	Role   string `json:"role"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type OfflineBundle struct {
	SchemaVersion  string        `json:"schemaVersion"`
	BundleID       string        `json:"bundleId"`
	LockDigest     string        `json:"lockDigest"`
	ReleaseID      string        `json:"releaseId"`
	ReleaseVersion string        `json:"releaseVersion"`
	Required       []BundleEntry `json:"required"`
	Entries        []BundleEntry `json:"entries"`
	ContentDigest  string        `json:"contentDigest"`
	CreatedAt      time.Time     `json:"createdAt"`
}

type BundleVerification struct {
	SchemaVersion string   `json:"schemaVersion"`
	BundleID      string   `json:"bundleId"`
	LockDigest    string   `json:"lockDigest"`
	Complete      bool     `json:"complete"` // local material closure only
	Missing       []string `json:"missing,omitempty"`
	Corrupt       []string `json:"corrupt,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
}

// OSDependencyIndex is a publisher-declared dependency closure, itself pinned
// as a package-index NodeArtifact. All roots and transitive packages must exist.
// It is not an OS package solver or evidence of a successful offline install.
type OSDependencyIndex struct {
	SchemaVersion string                `json:"schemaVersion"`
	OS            string                `json:"os"`
	Arch          string                `json:"arch"`
	Roots         []string              `json:"roots"`
	Packages      []OSDependencyPackage `json:"packages"`
}
type OSDependencyPackage struct {
	Ref          string   `json:"ref"`
	Version      string   `json:"version"`
	Digest       string   `json:"digest"`
	Dependencies []string `json:"dependencies"`
}

// bundleSource resolves only named content objects. It neither scans arbitrary
// operator files nor contains any network or runtime-import client.
type bundleSource struct {
	root      string
	locations map[string]BundleEntry
}

func bundleSafePath(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) || filepath.Clean(rel) != rel || strings.ContainsAny(rel, "\\:\r\n\x00") {
		return "", fmt.Errorf("invalid bundle-relative path")
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid bundle directory")
	}
	fi, err := os.Lstat(base)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("bundle directory must exist and not be a symlink")
	}
	current := base
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		fi, err = os.Lstat(current)
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !fi.IsDir()) || (i == len(parts)-1 && !fi.Mode().IsRegular()) {
			return "", fmt.Errorf("bundle path must contain only real directories and a regular file")
		}
	}
	return current, nil
}

func contentPathFor(digest string) string {
	return filepath.Join("blobs", "sha256", strings.TrimPrefix(digest, "sha256:"))
}
func validBundleDigest(digest string) bool {
	return digestPattern.MatchString(digest) && digest != placeholderDigest
}
func entryKey(e BundleEntry) string { return e.Role + "\x00" + e.Name + "\x00" + e.Digest }
func sortBundleEntries(entries []BundleEntry) {
	sort.Slice(entries, func(i, j int) bool { return entryKey(entries[i]) < entryKey(entries[j]) })
}

func (s bundleSource) file(digest string) (string, error) {
	if !validBundleDigest(digest) {
		return "", fmt.Errorf("invalid bundle content digest")
	}
	if entry, ok := s.locations[digest]; ok {
		return bundleSafePath(s.root, entry.Path)
	}
	path, err := bundleSafePath(s.root, contentPathFor(digest))
	if !os.IsNotExist(err) {
		return path, err
	}
	// Accept the early split-directory cache layout as an input only. Imports
	// always publish the flat layout used by the actual prepare downloader.
	hex := strings.TrimPrefix(digest, "sha256:")
	return bundleSafePath(s.root, filepath.Join("blobs", "sha256", hex[:2], hex[2:]))
}

func bundleFileIdentity(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (1<<30)+1))
	if err != nil || n > 1<<30 {
		return "", 0, fmt.Errorf("bundle object exceeds budget or cannot be read")
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), n, nil
}

func (s bundleSource) entry(role, name, digest string, size int64) (BundleEntry, error) {
	e := BundleEntry{Role: role, Name: name, Digest: digest}
	path, err := s.file(digest)
	if err != nil {
		return e, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: missing or unsafe %s", role)
	}
	got, n, err := bundleFileIdentity(path)
	if err != nil || got != digest || (size > 0 && n != size) {
		return e, fmt.Errorf("BUNDLE_CONTENT_CORRUPT: %s checksum/size differs", role)
	}
	e.Path, err = filepath.Rel(s.root, path)
	if err != nil {
		return e, err
	}
	e.Size = n
	return e, nil
}

func (s bundleSource) document(digest string, out any) error {
	path, err := s.file(digest)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || digestOf(data) != digest {
		return fmt.Errorf("invalid or oversized pinned document")
	}
	return json.Unmarshal(data, out)
}

// deriveBundleClosure is shared by build and verify. Its roots come from the
// caller's release lock, not from the bundle's mutable Required declaration.
func deriveBundleClosure(lock *ReleaseLock, source bundleSource) ([]BundleEntry, error) {
	if lock == nil || len(lock.Validate()) > 0 {
		return nil, fmt.Errorf("invalid release lock")
	}
	if len(lock.CLI) == 0 || len(lock.Charts) == 0 || len(lock.Migrations) == 0 {
		return nil, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: CLI, Chart and migrations are required")
	}
	for _, arch := range lock.Compatibility.Arch {
		if arch != "amd64" {
			return nil, fmt.Errorf("unsupported offline bundle architecture")
		}
	}
	var entries []BundleEntry
	seen := map[string]bool{}
	add := func(role, name, digest string, size int64) error {
		e, err := source.entry(role, name, digest, size)
		if err != nil {
			return err
		}
		if !seen[entryKey(e)] {
			seen[entryKey(e)] = true
			entries = append(entries, e)
		}
		return nil
	}
	image := func(img PinnedImage) error {
		if img.Platform != "" && img.Platform != "linux/amd64" {
			return fmt.Errorf("unsupported offline image architecture")
		}
		if err := add("image-manifest", img.Name, img.Digest, 0); err != nil {
			return err
		}
		var m ociManifest
		if err := source.document(img.Digest, &m); err != nil || m.SchemaVersion != 2 {
			return fmt.Errorf("invalid locked OCI manifest")
		}
		if len(m.Manifests) > 0 {
			if len(m.Manifests) > 128 || (m.MediaType != "application/vnd.oci.image.index.v1+json" && m.MediaType != "application/vnd.docker.distribution.manifest.list.v2+json") {
				return fmt.Errorf("unsupported OCI index")
			}
			var selected []ociDescriptor
			for _, desc := range m.Manifests {
				if p := desc.Platform; p != nil && p.OS == "linux" && p.Architecture == "amd64" && p.Variant == "" {
					selected = append(selected, desc)
				}
			}
			if len(selected) != 1 || (img.ManifestDigest != "" && selected[0].Digest != img.ManifestDigest) {
				return fmt.Errorf("OCI index does not identify the locked target platform")
			}
			desc := selected[0]
			if desc.Size <= 0 {
				return fmt.Errorf("invalid OCI manifest descriptor size")
			}
			if err := add("image-manifest", img.Name, desc.Digest, desc.Size); err != nil {
				return err
			}
			m = ociManifest{}
			if err := source.document(desc.Digest, &m); err != nil || m.SchemaVersion != 2 || len(m.Manifests) > 0 {
				return fmt.Errorf("invalid selected OCI manifest")
			}
		} else if img.ManifestDigest != "" && img.ManifestDigest != img.Digest {
			return fmt.Errorf("OCI manifest differs from locked identity")
		}
		if (m.MediaType != "application/vnd.oci.image.manifest.v1+json" && m.MediaType != "application/vnd.docker.distribution.manifest.v2+json") || len(m.Layers) > 128 {
			return fmt.Errorf("unsupported OCI manifest")
		}
		for i, desc := range append([]ociDescriptor{m.Config}, m.Layers...) {
			if desc.Size <= 0 {
				return fmt.Errorf("invalid OCI config/layer descriptor")
			}
			role := "image-layer"
			if i == 0 {
				role = "image-config"
			}
			if err := add(role, img.Name, desc.Digest, desc.Size); err != nil {
				return err
			}
			if i == 0 {
				var p ociPlatform
				if err := source.document(desc.Digest, &p); err != nil || p.OS != "linux" || p.Architecture != "amd64" {
					return fmt.Errorf("OCI config target architecture mismatch")
				}
			}
		}
		return nil
	}
	for _, img := range lock.Images {
		if err := image(img); err != nil {
			return nil, err
		}
	}
	for _, c := range lock.Charts {
		if err := add("chart", c.Name, c.Digest, 0); err != nil {
			return nil, err
		}
	}
	for i, c := range lock.CLI {
		if err := add("cli", fmt.Sprintf("cli-%s-%s-%d", c.OS, c.Arch, i), c.Digest, 0); err != nil {
			return nil, err
		}
	}
	for _, m := range lock.Migrations {
		if err := add("migration", m.Owner+"/"+m.Name, "sha256:"+m.Checksum, 0); err != nil {
			return nil, err
		}
	}
	indexes := map[string]OSDependencyIndex{}
	sandbox := false
	for _, a := range lock.NodeArtifacts {
		digest, ok := probeDigest(a.Digest)
		if !ok {
			return nil, fmt.Errorf("invalid node artifact digest")
		}
		switch a.Kind {
		case "container":
			if err := image(PinnedImage{Name: a.Ref, Ref: a.Ref, Digest: digest}); err != nil {
				return nil, err
			}
			if strings.Contains(a.Ref, "/pause:") {
				sandbox = true
				if err := add("sandbox-image", a.Ref, digest, 0); err != nil {
					return nil, err
				}
			}
		case "binary", "tool", "package":
			if err := add(a.Kind, a.Ref, digest, 0); err != nil {
				return nil, err
			}
		case "package-index":
			if err := add("os-package-index", a.Ref, digest, 0); err != nil {
				return nil, err
			}
			var idx OSDependencyIndex
			if err := source.document(digest, &idx); err != nil || idx.SchemaVersion != "os-package-closure/v1" || idx.OS != a.OS || idx.Arch != a.Arch || len(idx.Roots) == 0 {
				return nil, fmt.Errorf("invalid pinned OS dependency index")
			}
			key := idx.OS + "/" + idx.Arch
			if _, duplicate := indexes[key]; duplicate {
				return nil, fmt.Errorf("duplicate OS dependency index")
			}
			indexes[key] = idx
		default:
			return nil, fmt.Errorf("unsupported offline node artifact kind")
		}
	}
	if !sandbox || len(indexes) == 0 {
		return nil, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: lock must pin OS dependency indexes and a pause sandbox image")
	}
	for _, osName := range lock.Compatibility.OS {
		for _, arch := range lock.Compatibility.Arch {
			if _, ok := indexes[osName+"/"+arch]; !ok {
				return nil, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: target OS/architecture lacks a package index")
			}
		}
	}
	for _, idx := range indexes {
		closure, err := indexedPackageClosure(idx)
		if err != nil {
			return nil, err
		}
		packages := map[string]OSDependencyPackage{}
		for _, pkg := range closure {
			packages[pkg.Ref] = pkg
			if err := add("os-package", idx.OS+"/"+idx.Arch+"/"+pkg.Ref, pkg.Digest, 0); err != nil {
				return nil, err
			}
		}
		for _, a := range lock.NodeArtifacts {
			if a.Kind == "package" && a.OS == idx.OS && a.Arch == idx.Arch {
				digest, _ := probeDigest(a.Digest)
				pkg, ok := packages[a.Ref]
				if !ok || pkg.Digest != digest {
					return nil, fmt.Errorf("OS dependency index omits or changes a locked package root")
				}
			}
		}
	}
	sortBundleEntries(entries)
	return entries, nil
}

func BuildOfflineBundle(lock *ReleaseLock, dir string, osPackages, sandboxImages []BundleEntry, now time.Time) (OfflineBundle, error) {
	b := OfflineBundle{SchemaVersion: OfflineBundleManifest, CreatedAt: now.UTC()}
	if now.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	source := bundleSource{root: dir, locations: map[string]BundleEntry{}}
	for _, e := range append(append([]BundleEntry{}, osPackages...), sandboxImages...) {
		if !validBundleDigest(e.Digest) {
			return b, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: invalid supplementary identity")
		}
		if e.Path != "" {
			rel := e.Path
			if filepath.IsAbs(rel) {
				var err error
				rel, err = filepath.Rel(dir, rel)
				if err != nil {
					return b, err
				}
			}
			e.Path = rel
			source.locations[e.Digest] = e
		}
		pinned := false
		if lock != nil {
			for _, a := range lock.NodeArtifacts {
				digest, _ := probeDigest(a.Digest)
				if digest == e.Digest && ((e.Role == "os-package-index" && a.Kind == "package-index") || (e.Role == "sandbox-image" && a.Kind == "container" && strings.Contains(a.Ref, "/pause:"))) {
					pinned = true
				}
			}
		}
		if !pinned {
			return b, fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: supplementary index/sandbox is not pinned by release lock")
		}
	}
	entries, err := deriveBundleClosure(lock, source)
	if err != nil {
		return b, err
	}
	b.ReleaseID, b.ReleaseVersion, b.LockDigest = lock.ReleaseID, lock.ReleaseVersion, lockContentDigest(lock)
	b.Entries = entries
	b.Required = make([]BundleEntry, len(entries))
	for i, e := range entries {
		e.Path = ""
		b.Required[i] = e
	}
	b.ContentDigest = bundleContentDigest(b)
	b.BundleID = "bundle-" + strings.TrimPrefix(b.ContentDigest, "sha256:")[:16]
	return b, nil
}

func bundleContentDigest(bundle OfflineBundle) string {
	entries := append([]BundleEntry{}, bundle.Entries...)
	for i := range entries {
		entries[i].Path = ""
	}
	sortBundleEntries(entries)
	data, _ := json.Marshal(struct {
		Lock    string
		Entries []BundleEntry
	}{bundle.LockDigest, entries})
	return digestOf(data)
}

func VerifyOfflineBundle(b OfflineBundle, lock *ReleaseLock, dir string) BundleVerification {
	v := BundleVerification{SchemaVersion: OfflineBundleManifest, BundleID: b.BundleID, LockDigest: b.LockDigest}
	if lock == nil || len(lock.Validate()) > 0 {
		v.Reasons = append(v.Reasons, "invalid release lock")
		return v
	}
	if b.SchemaVersion != OfflineBundleManifest || b.LockDigest != lockContentDigest(lock) || b.ReleaseID != lock.ReleaseID || b.ReleaseVersion != lock.ReleaseVersion {
		v.Reasons = append(v.Reasons, "BUNDLE_LOCK_MISMATCH: release identity differs")
	}
	if b.ContentDigest != bundleContentDigest(b) || b.BundleID != "bundle-"+strings.TrimPrefix(bundleContentDigest(b), "sha256:")[:16] {
		v.Reasons = append(v.Reasons, "BUNDLE_IDENTITY_MISMATCH")
	}
	source := bundleSource{root: dir, locations: map[string]BundleEntry{}}
	have := map[string]BundleEntry{}
	for _, e := range b.Entries {
		if _, duplicate := have[entryKey(e)]; duplicate {
			v.Reasons = append(v.Reasons, "duplicate bundle entry")
		}
		have[entryKey(e)] = e
		if e.Size <= 0 || !validBundleDigest(e.Digest) {
			v.Corrupt = append(v.Corrupt, e.Name)
			continue
		}
		path, err := bundleSafePath(dir, e.Path)
		if err != nil {
			v.Corrupt = append(v.Corrupt, e.Name)
			continue
		}
		got, size, err := bundleFileIdentity(path)
		if err != nil || got != e.Digest || size != e.Size {
			v.Corrupt = append(v.Corrupt, e.Name)
			continue
		}
		source.locations[e.Digest] = e
	}
	expected, err := deriveBundleClosure(lock, source)
	if err != nil {
		v.Reasons = append(v.Reasons, err.Error())
	} else {
		contract := map[string]BundleEntry{}
		for _, e := range b.Required {
			if _, dup := contract[entryKey(e)]; dup {
				v.Reasons = append(v.Reasons, "duplicate required entry")
			}
			contract[entryKey(e)] = e
		}
		for _, need := range expected {
			e, ok := have[entryKey(need)]
			if !ok || e.Size != need.Size {
				v.Missing = append(v.Missing, need.Role+" "+need.Name)
			}
			c, ok := contract[entryKey(need)]
			if !ok || c.Size != need.Size {
				v.Reasons = append(v.Reasons, "BUNDLE_CONTRACT_MISMATCH: required identity differs from pinned closure")
			}
		}
		if len(have) != len(expected) || len(contract) != len(expected) {
			v.Reasons = append(v.Reasons, "BUNDLE_CONTRACT_MISMATCH: entry set differs from pinned closure")
		}
	}
	if len(v.Corrupt) > 0 {
		v.Reasons = append(v.Reasons, "BUNDLE_CONTENT_CORRUPT")
	}
	if len(v.Missing) > 0 || err != nil {
		v.Reasons = append(v.Reasons, "BUNDLE_CLOSURE_INCOMPLETE: node modification must not proceed")
	}
	v.Corrupt = uniqueSorted(v.Corrupt)
	v.Missing = uniqueSorted(v.Missing)
	v.Reasons = uniqueSorted(v.Reasons)
	v.Complete = len(v.Reasons) == 0
	return v
}

func WriteBundleManifest(path string, b OfflineBundle) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot exclusively create bundle manifest")
	}
	writeErr := json.NewEncoder(f).Encode(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("cannot write bundle manifest")
	}
	return nil
}
func LoadBundleManifest(path string) (OfflineBundle, error) {
	var b OfflineBundle
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 8<<20 {
		return b, fmt.Errorf("bundle manifest unavailable, unsafe or too large")
	}
	f, err := os.Open(path)
	if err != nil {
		return b, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		return b, fmt.Errorf("invalid bundle manifest")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return b, fmt.Errorf("bundle manifest must contain one document")
	}
	if b.SchemaVersion != OfflineBundleManifest {
		return b, fmt.Errorf("unsupported bundle schema")
	}
	return b, nil
}
