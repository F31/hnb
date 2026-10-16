package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// DiagnosticCollector assembles a support archive. Entries are added through
// typed methods so that only known-safe facts (digests, identities, reason
// codes, states) can be exported; free-form text is redacted before it is
// stored. Nothing here ever contacts a cluster or platform on its own.
type DiagnosticCollector struct {
	BundleID    string
	RunID       string
	CollectedAt time.Time
	entries     []DiagnosticEntry
	// MaxLogBytes bounds any single log excerpt so an archive cannot be used to
	// smuggle bulk customer data.
	MaxLogBytes int
}

// DiagnosticEntry is one redacted archive member.
type DiagnosticEntry struct {
	Path    string
	Content []byte
	// Digest is the SHA-256 of the *redacted* content, so it identifies what a
	// recipient actually received.
	Digest string
}

// BundleIdentity is the archive's own identity and collection time.
type BundleIdentity struct {
	SchemaVersion string    `json:"schemaVersion"`
	BundleID      string    `json:"bundleId"`
	RunID         string    `json:"runId"`
	CollectedAt   time.Time `json:"collectedAt"`
	Redacted      bool      `json:"redacted"`
	EntryPaths    []string  `json:"entryPaths"`
	ContentDigest string    `json:"contentDigest"`
}

// AddJSON stores a structured document. Values are marshalled and then passed
// through the map redactor, so credential-shaped fields are dropped even if a
// caller supplies them.
func (c *DiagnosticCollector) AddJSON(name string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode diagnostic entry %q", name)
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return fmt.Errorf("cannot normalize diagnostic entry %q", name)
	}
	if m, ok := generic.(map[string]any); ok {
		RedactMap(m)
		generic = m
	} else if list, ok := generic.([]any); ok {
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				RedactMap(m)
			}
		}
	}
	safe, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot normalize diagnostic entry %q", name)
	}
	return c.add(name, safe)
}

// AddText stores free-form text (log excerpts, failure notes). It is redacted
// and truncated; it is the only entry kind that can contain operator prose.
func (c *DiagnosticCollector) AddText(name, text string) error {
	limit := c.MaxLogBytes
	if limit <= 0 {
		limit = 64 * 1024
	}
	redacted := Redact(text)
	if len(redacted) > limit {
		redacted = redacted[:limit] + "\n[TRUNCATED]"
	}
	return c.add(name, []byte(redacted))
}

func (c *DiagnosticCollector) add(name string, content []byte) error {
	clean := strings.TrimSpace(name)
	if clean == "" || path.IsAbs(clean) || strings.Contains(clean, "..") || strings.ContainsAny(clean, "\r\n\x00") {
		return fmt.Errorf("invalid diagnostic entry name")
	}
	for _, existing := range c.entries {
		if existing.Path == clean {
			return fmt.Errorf("duplicate diagnostic entry %q", clean)
		}
	}
	sum := sha256.Sum256(content)
	c.entries = append(c.entries, DiagnosticEntry{Path: clean, Content: content,
		Digest: fmt.Sprintf("sha256:%x", sum)})
	return nil
}

// Identity computes the bundle identity over the redacted content.
func (c *DiagnosticCollector) Identity() BundleIdentity {
	sorted := append([]DiagnosticEntry(nil), c.entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	hash := sha256.New()
	paths := make([]string, 0, len(sorted))
	for _, entry := range sorted {
		hash.Write([]byte(entry.Path))
		hash.Write([]byte{0})
		hash.Write(entry.Content)
		hash.Write([]byte{0})
		paths = append(paths, entry.Path)
	}
	collected := c.CollectedAt
	if collected.IsZero() {
		collected = time.Now().UTC()
	}
	return BundleIdentity{SchemaVersion: "diagnostic-bundle/v1", BundleID: c.BundleID, RunID: c.RunID,
		CollectedAt: collected.UTC(), Redacted: true, EntryPaths: paths,
		ContentDigest: fmt.Sprintf("sha256:%x", hash.Sum(nil))}
}

// WriteArchive writes a deterministic gzip-compressed tar. Entries are sorted
// and use a fixed timestamp so identical inputs yield identical bytes.
func (c *DiagnosticCollector) WriteArchive(w io.Writer) error {
	identity := c.Identity()
	manifest, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode bundle identity")
	}
	sorted := append([]DiagnosticEntry(nil), c.entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	write := func(name string, content []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			return fmt.Errorf("cannot write bundle entry %q", name)
		}
		if _, err := tw.Write(content); err != nil {
			return fmt.Errorf("cannot write bundle entry %q", name)
		}
		return nil
	}
	if err := write("bundle-identity.json", append(manifest, '\n')); err != nil {
		return err
	}
	for _, entry := range sorted {
		if err := write(entry.Path, entry.Content); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("cannot finalize bundle")
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("cannot finalize bundle compression")
	}
	if _, err := w.Write(buffer.Bytes()); err != nil {
		return fmt.Errorf("cannot write bundle")
	}
	return nil
}

// WriteArchiveFile writes the bundle to a private file, refusing to overwrite.
func (c *DiagnosticCollector) WriteArchiveFile(target string) error {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create diagnostics archive: %w", err)
	}
	writeErr := c.WriteArchive(file)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("cannot write diagnostics archive")
	}
	return nil
}

// Entries returns the collected entries (redacted content). It exists for
// callers and tests that need to inspect the archive payload before writing.
func (c *DiagnosticCollector) Entries() []DiagnosticEntry {
	return append([]DiagnosticEntry(nil), c.entries...)
}
