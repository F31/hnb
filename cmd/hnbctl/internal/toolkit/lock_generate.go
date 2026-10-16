package toolkit

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ReleaseInputSchema = "release-input/v1"

type ReleaseInput struct {
	SchemaVersion          string            `yaml:"schemaVersion"`
	ReleaseID              string            `yaml:"releaseId"`
	ReleaseVersion         string            `yaml:"releaseVersion"`
	CLIFile                string            `yaml:"cliFile"`
	ChartFile              string            `yaml:"chartFile"`
	Images                 []ImageSource     `yaml:"images"`
	Migrations             []MigrationSource `yaml:"migrations"`
	Compatibility          LockCompatibility `yaml:"compatibility"`
	AllowedRedirectOrigins []string          `yaml:"allowedRedirectOrigins,omitempty"`
}
type ImageSource struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref"`
}
type MigrationSource struct {
	Owner string `yaml:"owner"`
	File  string `yaml:"file"`
}

func DecodeReleaseInput(data []byte) (*ReleaseInput, error) {
	var input ReleaseInput
	if err := decodeStrict(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

func sourcePath(base, path string) (string, error) {
	if !validLocalPath(path) || strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("release source must be an explicit local path")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path), nil
}

func hashSource(path string) (string, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		return "", fmt.Errorf("release source must be a nonempty regular file, not a symlink")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("release source is not readable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "", fmt.Errorf("release source changed before hashing")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, file)
	after, statErr := file.Stat()
	if err != nil || statErr != nil || count != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return "", fmt.Errorf("release source changed during hashing")
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func chartMetadata(path string) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("chart package unavailable")
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", "", fmt.Errorf("chart must be a packaged gzip tar archive")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var name, version string
	var bytesRead int64
	for entries := 0; entries < 10000; entries++ {
		header, err := reader.Next()
		if err == io.EOF {
			if name != "" && version != "" {
				return name, version, nil
			}
			return "", "", fmt.Errorf("chart package has no root Chart.yaml")
		}
		if err != nil {
			return "", "", fmt.Errorf("invalid chart archive")
		}
		bytesRead += header.Size
		if header.Size < 0 || bytesRead > 64*1024*1024 {
			return "", "", fmt.Errorf("chart package exceeds metadata budget")
		}
		parts := strings.Split(header.Name, "/")
		if len(parts) != 2 || parts[1] != "Chart.yaml" {
			continue
		}
		if header.Typeflag != tar.TypeReg || name != "" || !validName(parts[0]) || header.Size > 64*1024 {
			return "", "", fmt.Errorf("invalid or duplicate root chart metadata")
		}
		data, err := io.ReadAll(io.LimitReader(reader, 64*1024+1))
		if err != nil {
			return "", "", fmt.Errorf("chart metadata unreadable")
		}
		var meta struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			APIVersion string `yaml:"apiVersion"`
		}
		if yaml.Unmarshal(data, &meta) != nil || meta.Name != parts[0] || meta.APIVersion != "v2" || !tagPattern.MatchString(meta.Version) || strings.EqualFold(meta.Version, "latest") {
			return "", "", fmt.Errorf("invalid root Chart.yaml identity")
		}
		name, version = meta.Name, meta.Version
	}
	return "", "", fmt.Errorf("chart package exceeds entry budget")
}

// GenerateCandidateLock hashes actual artifacts and resolves actual registry
// bytes. It never builds, executes CLI binaries, runs migrations or certifies
// the profile/recipe; dependency closure and delivery are separate gates.
func GenerateCandidateLock(ctx context.Context, input *ReleaseInput, base string, resolver RegistryResolver) (*ReleaseLock, error) {
	if input.SchemaVersion != ReleaseInputSchema || !validName(input.ReleaseID) || !tagPattern.MatchString(input.ReleaseVersion) || strings.EqualFold(input.ReleaseVersion, "latest") || input.CLIFile == "" || input.ChartFile == "" || len(input.Images) == 0 || len(input.Migrations) == 0 {
		return nil, fmt.Errorf("release input requires identity, CLI, packaged Chart, images and owner migrations")
	}
	seen := map[string]bool{}
	for _, image := range input.Images {
		if !validName(image.Name) || seen[image.Name] {
			return nil, fmt.Errorf("image names must be valid and unique")
		}
		seen[image.Name] = true
		if _, _, err := registryReference(image.Ref); err != nil {
			return nil, err
		}
	}
	if len(input.Compatibility.OS) == 0 || len(input.Compatibility.Arch) == 0 || len(input.Compatibility.KubernetesVersions) == 0 || len(input.Compatibility.Profiles) == 0 {
		return nil, fmt.Errorf("explicit candidate compatibility declarations are required")
	}
	if err := resolver.DeclareRedirectOrigins(input.AllowedRedirectOrigins); err != nil {
		return nil, err
	}
	for _, arch := range input.Compatibility.Arch {
		if arch != "amd64" {
			return nil, fmt.Errorf("only candidate amd64 artifact resolution is implemented")
		}
	}
	cliPath, err := sourcePath(base, input.CLIFile)
	if err != nil {
		return nil, err
	}
	cliDigest, err := hashSource(cliPath)
	if err != nil {
		return nil, err
	}
	info, err := buildinfo.ReadFile(cliPath)
	if err != nil || info.Main.Path != "github.com/F31/hnb/cmd/hnbctl" {
		return nil, fmt.Errorf("CLI artifact is not an identified hnbctl Go binary")
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != "linux" || settings["GOARCH"] != "amd64" {
		return nil, fmt.Errorf("CLI artifact must target linux/amd64")
	}
	if second, err := hashSource(cliPath); err != nil || second != cliDigest {
		return nil, fmt.Errorf("CLI changed during identity inspection")
	}
	chartPath, err := sourcePath(base, input.ChartFile)
	if err != nil {
		return nil, err
	}
	chartDigest, err := hashSource(chartPath)
	if err != nil {
		return nil, err
	}
	chartName, chartVersion, err := chartMetadata(chartPath)
	if err != nil {
		return nil, err
	}
	if second, err := hashSource(chartPath); err != nil || second != chartDigest {
		return nil, fmt.Errorf("chart changed during metadata inspection")
	}
	lock := &ReleaseLock{SchemaVersion: LockSchemaVersion, Status: "candidate", ReleaseID: input.ReleaseID, ReleaseVersion: input.ReleaseVersion, Compatibility: input.Compatibility, CLI: []PinnedCLI{{OS: "linux", Arch: "amd64", Ref: filepath.Base(cliPath), Digest: cliDigest}}, Charts: []PinnedChart{{Name: chartName, Version: chartVersion, Ref: filepath.Base(chartPath), Digest: chartDigest}}}
	seen = map[string]bool{}
	for _, migration := range input.Migrations {
		if migration.Owner != "platform" && migration.Owner != "market" && migration.Owner != "artifact" {
			return nil, fmt.Errorf("migration owner must be platform, market or artifact")
		}
		path, err := sourcePath(base, migration.File)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		if filepath.Ext(name) != ".sql" || strings.HasSuffix(name, ".rollback.sql") || seen[migration.Owner+"/"+name] {
			return nil, fmt.Errorf("unique owner forward SQL migrations are required")
		}
		seen[migration.Owner+"/"+name] = true
		digest, err := hashSource(path)
		if err != nil {
			return nil, err
		}
		lock.Migrations = append(lock.Migrations, Migration{Owner: migration.Owner, Name: name, Checksum: strings.TrimPrefix(digest, "sha256:")})
	}
	for _, image := range input.Images {
		resolved, err := resolver.Resolve(ctx, image.Name, image.Ref)
		if err != nil {
			return nil, fmt.Errorf("image %s: %w", image.Name, err)
		}
		lock.Images = append(lock.Images, resolved)
	}
	// Only endpoints actually used are recorded, so the lock states the real
	// transport surface instead of an aspirational allowlist.
	for origin := range resolver.ObservedRedirect {
		lock.SourceRedirectOrigins = append(lock.SourceRedirectOrigins, origin)
	}
	sort.Strings(lock.SourceRedirectOrigins)
	if problems := lock.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("generated candidate lock is invalid: %s", strings.Join(problems, "; "))
	}
	return lock, nil
}
