package toolkit

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ImportOfflineBundle copies verified objects into an independent private
// installer cache. It never imports into containerd, installs OS packages or
// changes a node. The output manifest is published only after re-verification.
func ImportOfflineBundle(bundle OfflineBundle, lock *ReleaseLock, sourceDir, targetDir string) (OfflineBundle, error) {
	var empty OfflineBundle
	if v := VerifyOfflineBundle(bundle, lock, sourceDir); !v.Complete {
		return empty, fmt.Errorf("offline closure invalid; import refused before destination creation")
	}
	sourceAbs, err := filepath.Abs(sourceDir)
	if err != nil {
		return empty, err
	}
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return empty, err
	}
	if sourceAbs == targetAbs {
		return empty, fmt.Errorf("import requires an independent destination cache")
	}
	if err := preparePrivateDir(targetDir); err != nil {
		return empty, err
	}
	out := bundle
	out.Entries = append([]BundleEntry{}, bundle.Entries...)
	seen := map[string]bool{}
	for i, e := range out.Entries {
		rel := contentPathFor(e.Digest)
		out.Entries[i].Path = rel
		if seen[e.Digest] {
			continue
		}
		seen[e.Digest] = true
		source, err := bundleSafePath(sourceDir, e.Path)
		if err != nil {
			return empty, fmt.Errorf("source changed during import")
		}
		target := filepath.Join(targetDir, rel)
		if _, err := os.Lstat(target); err == nil {
			path, err := bundleSafePath(targetDir, rel)
			if err != nil {
				return empty, fmt.Errorf("unsafe existing import object")
			}
			got, size, err := bundleFileIdentity(path)
			if err != nil || got != e.Digest || size != e.Size {
				return empty, fmt.Errorf("import destination contains corrupt content")
			}
			continue
		} else if !os.IsNotExist(err) {
			return empty, fmt.Errorf("import destination unreadable")
		}
		if err := copyBundleObject(source, target, e); err != nil {
			return empty, err
		}
	}
	if v := VerifyOfflineBundle(out, lock, targetDir); !v.Complete {
		return empty, fmt.Errorf("destination re-verification failed")
	}
	// Keep the pinned release identity beside the material cache, making the
	// imported directory usable without the original publisher's lock file.
	lockPath := filepath.Join(targetDir, "release.lock.json")
	if fi, err := os.Lstat(lockPath); err == nil {
		if !fi.Mode().IsRegular() || fi.Size() > 8<<20 {
			return empty, fmt.Errorf("unsafe destination release lock")
		}
		data, err := os.ReadFile(lockPath)
		if err != nil {
			return empty, err
		}
		stored, err := DecodeReleaseLock(data)
		if err != nil || lockContentDigest(stored) != out.LockDigest {
			return empty, fmt.Errorf("destination release lock conflicts with import")
		}
	} else if os.IsNotExist(err) {
		data, err := json.Marshal(lock)
		if err != nil {
			return empty, err
		}
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return empty, err
		}
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return empty, fmt.Errorf("cannot publish imported release identity")
		}
	} else {
		return empty, fmt.Errorf("destination release lock unreadable")
	}
	manifestPath := filepath.Join(targetDir, "bundle.json")
	if _, err := os.Lstat(manifestPath); err == nil {
		stored, err := LoadBundleManifest(manifestPath)
		if err != nil || stored.ContentDigest != out.ContentDigest || !VerifyOfflineBundle(stored, lock, targetDir).Complete {
			return empty, fmt.Errorf("destination manifest conflicts with import")
		}
	} else if os.IsNotExist(err) {
		if err := WriteBundleManifest(manifestPath, out); err != nil {
			return empty, err
		}
	} else {
		return empty, fmt.Errorf("destination manifest unreadable")
	}
	return out, nil
}

func copyBundleObject(source, target string, entry BundleEntry) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("source object unreadable")
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".bundle-part-")
	if err != nil {
		return fmt.Errorf("cannot stage import object")
	}
	defer func() { out.Close(); os.Remove(out.Name()) }()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(in, entry.Size+1))
	if err != nil || size != entry.Size || fmt.Sprintf("sha256:%x", hash.Sum(nil)) != entry.Digest {
		return fmt.Errorf("source object changed or transfer incomplete")
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("cannot sync import object")
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("cannot finish import object")
	}
	if err := os.Rename(out.Name(), target); err != nil {
		return fmt.Errorf("cannot publish import object")
	}
	return nil
}
