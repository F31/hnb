package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCopiesIndexedTransitiveDependency(t *testing.T) {
	lock, dir, index, _ := offlineFixture(t)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// All locked roots are cached; only the transitive dependency is absent.
	var idx OSDependencyIndex
	data, err := os.ReadFile(filepath.Join(dir, contentPathFor(index[0].Digest)))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	dependency := idx.Packages[1]
	if err := os.Remove(filepath.Join(dir, contentPathFor(dependency.Digest))); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, dependency.Ref), []byte("synthetic libc dependency"), 0600); err != nil {
		t.Fatal(err)
	}
	lock.Charts[0].Ref = "hnb.tgz"
	d := Downloader{SourceDir: source}
	r, err := d.PrepareLock(context.Background(), lock, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Artifacts {
		if a.Kind == "os-package" && a.Digest == dependency.Digest {
			if a.FromCache {
				t.Fatal("uncached dependency reported as a cache hit")
			}
			if actual, err := hashSource(a.Path); err != nil || actual != dependency.Digest {
				t.Fatal("transitive dependency not prepared correctly")
			}
			return
		}
	}
	t.Fatal("transitive dependency preparation not recorded")
}

func builtOfflineFixture(t *testing.T) (*ReleaseLock, string, OfflineBundle) {
	t.Helper()
	lock, dir, index, sandbox := offlineFixture(t)
	bundle, err := BuildOfflineBundle(lock, dir, index, sandbox, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	return lock, dir, bundle
}

// Remove an entry and its self-declared requirement and repair the public hash.
// The pinned lock/index/OCI descriptors must still expose the omission.
func TestOfflineContractCannotErasePinnedRequirements(t *testing.T) {
	for _, role := range []string{"image-config", "image-layer", "chart", "cli", "migration", "os-package-index", "os-package", "binary", "sandbox-image"} {
		t.Run(role, func(t *testing.T) {
			lock, dir, bundle := builtOfflineFixture(t)
			for _, e := range bundle.Entries {
				if e.Role == role {
					key := entryKey(e)
					filter := func(entries []BundleEntry) []BundleEntry {
						var out []BundleEntry
						for _, item := range entries {
							if entryKey(item) != key {
								out = append(out, item)
							}
						}
						return out
					}
					bundle.Entries = filter(bundle.Entries)
					bundle.Required = filter(bundle.Required)
					break
				}
			}
			bundle.ContentDigest = bundleContentDigest(bundle)
			bundle.BundleID = "bundle-" + strings.TrimPrefix(bundle.ContentDigest, "sha256:")[:16]
			if v := VerifyOfflineBundle(bundle, lock, dir); v.Complete {
				t.Fatalf("self-rewritten contract hid missing %s", role)
			}
		})
	}
}

func TestOfflineDeletionOfEveryDeliveredFileFails(t *testing.T) {
	_, _, baseline := builtOfflineFixture(t)
	for i := range baseline.Entries {
		t.Run(baseline.Entries[i].Role+"/"+baseline.Entries[i].Name, func(t *testing.T) {
			lock, dir, bundle := builtOfflineFixture(t)
			path := filepath.Join(dir, bundle.Entries[i].Path)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if v := VerifyOfflineBundle(bundle, lock, dir); v.Complete {
				t.Fatal("deleted material still verified")
			}
			target := filepath.Join(t.TempDir(), "must-not-exist")
			if _, err := ImportOfflineBundle(bundle, lock, dir, target); err == nil {
				t.Fatal("deleted material imported")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("invalid closure changed destination")
			}
		})
	}
}

func TestOfflineManifestAndDependencyIndexFailClosed(t *testing.T) {
	lock, dir, index, sandbox := offlineFixture(t)
	// Replace a legitimate package index with an internally incomplete one and
	// update the publisher lock so the rejection proves dependency validation,
	// rather than just a top-level checksum mismatch.
	var doc OSDependencyIndex
	path := filepath.Join(dir, contentPathFor(index[0].Digest))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Packages = doc.Packages[:1]
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	index[0].Digest = writeOfflineCache(t, dir, string(data))
	lock.NodeArtifacts[0].Digest = index[0].Digest
	if _, err := BuildOfflineBundle(lock, dir, index, sandbox, offlineNow()); err == nil {
		t.Fatal("unresolved transitive package accepted")
	}
	// Independently test invalid OCI input, not an unrealistic success fixture.
	lock, dir, index, sandbox = offlineFixture(t)
	lock.Images[0].Digest = writeOfflineCache(t, dir, `{"schemaVersion":2}`)
	if _, err := BuildOfflineBundle(lock, dir, index, sandbox, offlineNow()); err == nil {
		t.Fatal("OCI manifest without config accepted")
	}
}

func TestOfflineBundleSupportsLockedIndexClosure(t *testing.T) {
	lock, dir, index, sandbox := offlineFixture(t)
	leaf := lock.Images[0].Digest
	leafFile := filepath.Join(dir, contentPathFor(leaf))
	info, err := os.Stat(leafFile)
	if err != nil {
		t.Fatal(err)
	}
	root := ociManifest{SchemaVersion: 2, MediaType: "application/vnd.oci.image.index.v1+json", Manifests: []ociDescriptor{{Digest: leaf, Size: info.Size(), Platform: &ociPlatform{OS: "linux", Architecture: "amd64"}}}}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	lock.Images[0].Digest = writeOfflineCache(t, dir, string(data))
	lock.Images[0].ManifestDigest = leaf
	bundle, err := BuildOfflineBundle(lock, dir, index, sandbox, offlineNow())
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyOfflineBundle(bundle, lock, dir).Complete {
		t.Fatal("realistic index closure failed")
	}
	for _, e := range bundle.Entries {
		if e.Role == "image-manifest" && e.Name == "apiserver" && e.Digest == leaf {
			if err := os.Remove(filepath.Join(dir, e.Path)); err != nil {
				t.Fatal(err)
			}
			if VerifyOfflineBundle(bundle, lock, dir).Complete {
				t.Fatal("missing selected architecture manifest accepted")
			}
			return
		}
	}
	t.Fatal("selected manifest not in closure")
}

func TestOfflinePathsAndMetadataRejectTampering(t *testing.T) {
	for _, change := range []func(*OfflineBundle){
		func(b *OfflineBundle) { b.Entries[0].Path = "../escape" },
		func(b *OfflineBundle) { b.Entries[0].Path = "/etc/passwd" },
		func(b *OfflineBundle) { b.Entries[0].Size++ },
		func(b *OfflineBundle) { b.LockDigest = "" },
		func(b *OfflineBundle) { b.Entries = append(b.Entries, b.Entries[0]) },
		func(b *OfflineBundle) { b.Required = nil },
	} {
		lock, dir, bundle := builtOfflineFixture(t)
		change(&bundle)
		if VerifyOfflineBundle(bundle, lock, dir).Complete {
			t.Fatal("tampered metadata accepted")
		}
	}
	lock, dir, bundle := builtOfflineFixture(t)
	path := filepath.Join(dir, bundle.Entries[0].Path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "external-data")
	if err := os.WriteFile(outside, []byte("customer-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if VerifyOfflineBundle(bundle, lock, dir).Complete {
		t.Fatal("symlink object accepted")
	}
}

func TestOfflineImportCopiesIndependentVerifiedObjects(t *testing.T) {
	lock, dir, bundle := builtOfflineFixture(t)
	target := filepath.Join(t.TempDir(), "imported")
	imported, err := ImportOfflineBundle(bundle, lock, dir, target)
	if err != nil {
		t.Fatal(err)
	}
	if imported.ContentDigest != bundle.ContentDigest {
		t.Fatal("relocation changed material identity")
	}
	fi, err := os.Stat(target)
	if err != nil || fi.Mode().Perm() != 0700 {
		t.Fatal("import directory not private")
	}
	for _, e := range imported.Entries {
		fi, err := os.Stat(filepath.Join(target, e.Path))
		if err != nil || fi.Mode().Perm() != 0600 {
			t.Fatal("import object not private")
		}
	}
	if _, err := ImportOfflineBundle(bundle, lock, dir, target); err != nil {
		t.Fatalf("verified retry failed: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBundleManifest(filepath.Join(target, "bundle.json"))
	if err != nil || !VerifyOfflineBundle(loaded, lock, target).Complete {
		t.Fatalf("independent import invalid: %v", err)
	}
	lockBytes, err := os.ReadFile(filepath.Join(target, "release.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	importedLock, err := DecodeReleaseLock(lockBytes)
	if err != nil || lockContentDigest(importedLock) != bundle.LockDigest {
		t.Fatal("import lost pinned release identity")
	}
}
