package toolkit

import (
	"context"
	"fmt"
	"sort"
)

func indexedPackageClosure(idx OSDependencyIndex) ([]OSDependencyPackage, error) {
	if idx.SchemaVersion != "os-package-closure/v1" || len(idx.Roots) == 0 || len(idx.Packages) > 4096 {
		return nil, fmt.Errorf("invalid or oversized OS dependency index")
	}
	packages := map[string]OSDependencyPackage{}
	for _, pkg := range idx.Packages {
		if pkg.Ref == "" || pkg.Version == "" || !validBundleDigest(pkg.Digest) || len(pkg.Dependencies) > 128 {
			return nil, fmt.Errorf("invalid indexed OS package")
		}
		if _, dup := packages[pkg.Ref]; dup {
			return nil, fmt.Errorf("duplicate indexed package")
		}
		packages[pkg.Ref] = pkg
	}
	visited := map[string]bool{}
	var closure []OSDependencyPackage
	var visit func(string) error
	visit = func(ref string) error {
		if visited[ref] {
			return nil
		}
		pkg, ok := packages[ref]
		if !ok {
			return fmt.Errorf("BUNDLE_CLOSURE_INCOMPLETE: unresolved package dependency")
		}
		visited[ref] = true
		closure = append(closure, pkg)
		for _, dep := range pkg.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		return nil
	}
	for _, ref := range idx.Roots {
		if err := visit(ref); err != nil {
			return nil, err
		}
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].Ref < closure[j].Ref })
	return closure, nil
}

// Online preparation copies/downloads every declared transitive dependency,
// pinned by the index digest in the release lock. Bundle verify/import never
// invokes this downloader and never falls back to the public network.
func (d *Downloader) prepareIndexedPackages(ctx context.Context, dir string, a NodeArtifact, r *PrepareResult) error {
	digest, _ := probeDigest(a.Digest)
	var idx OSDependencyIndex
	source := bundleSource{root: dir}
	if err := source.document(digest, &idx); err != nil || idx.OS != a.OS || idx.Arch != a.Arch {
		return fmt.Errorf("invalid pinned OS dependency index")
	}
	closure, err := indexedPackageClosure(idx)
	if err != nil {
		return err
	}
	for _, pkg := range closure {
		entry, err := d.fetch(ctx, dir, "os-package", idx.OS+"/"+idx.Arch+"/"+pkg.Ref, pkg.Ref, pkg.Digest, 0)
		if err != nil {
			return err
		}
		r.Artifacts = append(r.Artifacts, entry)
		r.BytesVerified += entry.Size
		if entry.FromCache {
			r.FromCache++
		}
	}
	return nil
}
