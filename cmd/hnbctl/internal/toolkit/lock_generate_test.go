package toolkit

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceIdentityAndChartMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hnb-0.1.0.tgz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	data := []byte("apiVersion: v2\nname: hnb\nversion: 0.1.0\n")
	if err := archive.WriteHeader(&tar.Header{Name: "hnb/Chart.yaml", Mode: 0o600, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(data); err != nil {
		t.Fatal(err)
	}
	archive.Close()
	compressed.Close()
	file.Close()
	name, version, err := chartMetadata(path)
	if err != nil || name != "hnb" || version != "0.1.0" {
		t.Fatal("actual Chart.yaml identity not read", err)
	}
	digest, err := hashSource(path)
	if err != nil || !digestPattern.MatchString(digest) {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := hashSource(link); err == nil {
		t.Fatal("symlink source accepted")
	}
	if err := os.WriteFile(path, []byte("not a chart"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := chartMetadata(path); err == nil {
		t.Fatal("unpackaged chart accepted")
	}
}

func TestCandidateInputRejectsMissingAndInjectedDigests(t *testing.T) {
	if _, err := DecodeReleaseInput([]byte("schemaVersion: release-input/v1\nimages:\n- name: fixture\n  ref: registry.invalid/fixture:v1\n  digest: sha256:fake\n")); err == nil {
		t.Fatal("caller-provided digest accepted")
	}
	if _, err := GenerateCandidateLock(context.Background(), &ReleaseInput{SchemaVersion: ReleaseInputSchema, ReleaseID: "candidate", ReleaseVersion: "0.2.0"}, t.TempDir(), NewRegistryResolver(nil)); err == nil {
		t.Fatal("missing artifact sources accepted")
	}
	lock := ReleaseLock{SchemaVersion: LockSchemaVersion, Status: "candidate", ReleaseID: "candidate", ReleaseVersion: "0.2.0", Images: []PinnedImage{{Name: "fixture", Ref: "registry.invalid/fixture:v1", Digest: bodyDigest("fixture")}}, Compatibility: LockCompatibility{OS: []string{"ubuntu-24.04"}, Arch: []string{"amd64"}}}
	if problems := lock.Validate(); !containsSubstring(problems, "requires CLI") || !containsSubstring(problems, "platform identity") {
		t.Fatal("incomplete candidate accepted", problems)
	}
	lock.Status = "certified"
	if !containsSubstring(lock.Validate(), "cannot certify") {
		t.Fatal("generator status could certify a release")
	}
	lock.Status = ""
	lock.Migrations = []Migration{{Name: "fixture", Checksum: strings.Repeat("0", 64)}}
	if !containsSubstring(lock.Validate(), "real checksum") {
		t.Fatal("zero migration checksum accepted")
	}
}
