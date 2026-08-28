package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Scanner interface {
	Ping(ctx context.Context) error
	UpdateDB(ctx context.Context) (string, error)
	Scan(ctx context.Context, imageRef, imageDigest string) (*Findings, error)
}

type Findings struct {
	SeveritySummary map[string]int `json:"severity_summary"`
	Findings        []Finding      `json:"findings"`
}

type Finding struct {
	CVE            string  `json:"cve"`
	Severity       string  `json:"severity"`
	CVSS3          float64 `json:"cvss3,omitempty"`
	Package        string  `json:"package"`
	Version        string  `json:"version"`
	FixedVersion   string  `json:"fixed_version,omitempty"`
	Title          string  `json:"title"`
	Description    string  `json:"description,omitempty"`
	Exempted       bool    `json:"exempted,omitempty"`
	AffectedImages int     `json:"affected_images,omitempty"`
}

// ---------------------------------------------------------------------------
// HarborScanner — uses Harbor's built-in Trivy via Harbor REST API
// ---------------------------------------------------------------------------

type HarborScanner struct {
	BaseURL    string
	Username   string
	Password   string
	client     *http.Client
}

func NewHarborScanner(baseURL, username, password string) *HarborScanner {
	return &HarborScanner{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Username: username,
		Password: password,
		client:   &http.Client{Timeout: 2 * time.Minute},
	}
}

func (h *HarborScanner) basicAuth() func(*http.Request) {
	return func(req *http.Request) {
		req.SetBasicAuth(h.Username, h.Password)
	}
}

func (h *HarborScanner) do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(h.Username, h.Password)
	return h.client.Do(req)
}

func (h *HarborScanner) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+"/api/v2.0/ping", nil)
	if err != nil {
		return err
	}
	resp, err := h.do(req)
	if err != nil {
		return fmt.Errorf("harbor ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("harbor ping: status %d", resp.StatusCode)
	}
	return nil
}

func (h *HarborScanner) UpdateDB(ctx context.Context) (string, error) {
	// Harbor manages its own Trivy DB updates; no client action needed.
	return fmt.Sprintf("harbor:%s", time.Now().Format("2006-01-02")), nil
}

// parseHarborRef extracts project and repository name from a Harbor
// registry URL like "http://harbor:8080/library/my-image" or
// "http://172.17.0.1/hnb/jars@sha256:abc...".
func (h *HarborScanner) parseHarborRef(imageRef, imageDigest string) (project, repo string, err error) {
	// Try to strip the HarborURL prefix first
	ref := strings.TrimPrefix(imageRef, h.BaseURL)
	if ref == imageRef {
		// Prefix didn't match; try to parse the URL path directly
		u := imageRef
		if idx := strings.Index(u, "://"); idx >= 0 {
			u = u[idx+3:]
		}
		if idx := strings.Index(u, "/"); idx >= 0 {
			u = u[idx+1:]
		}
		ref = u
	}
	ref = strings.TrimPrefix(ref, "/")
	// Remove @sha256:... suffix if present
	if idx := strings.Index(ref, "@"); idx >= 0 {
		ref = ref[:idx]
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unable to parse harbor ref from %s (base=%s)", imageRef, h.BaseURL)
	}
	return parts[0], parts[1], nil
}

func (h *HarborScanner) Scan(ctx context.Context, imageRef, imageDigest string) (*Findings, error) {
	project, repo, err := h.parseHarborRef(imageRef, imageDigest)
	if err != nil {
		return nil, err
	}
	digest := imageDigest
	if !strings.HasPrefix(digest, "sha256:") && !strings.HasPrefix(digest, "sha512:") {
		digest = "sha256:" + digest
	}

	// 1. Trigger scan
	scanURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/scan",
		h.BaseURL, project, repo, digest)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scanURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.do(req)
	if err != nil {
		return nil, fmt.Errorf("harbor scan trigger: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		// 409 means already scanned or scanning
		if resp.StatusCode != http.StatusConflict {
			return nil, fmt.Errorf("harbor scan trigger: status %d", resp.StatusCode)
		}
	}

	// 2. Poll for scan completion (up to 120s)
	artifactURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s",
		h.BaseURL, project, repo, digest)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}

		req, _ = http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
		resp, err := h.do(req)
		if err != nil {
			continue
		}
		var art struct {
			ScanOverview map[string]struct {
				Status   string `json:"scan_status"`
				Severity string `json:"severity"`
			} `json:"scan_overview"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&art)
		resp.Body.Close()

		for _, overview := range art.ScanOverview {
			if overview.Status == "Finished" || overview.Status == "Success" {
				goto fetchVulns
			}
			if overview.Status == "Error" || overview.Status == "Stopped" {
				return nil, fmt.Errorf("harbor scan failed: status=%s", overview.Status)
			}
		}
		// Still pending, keep polling
	}
	return nil, fmt.Errorf("harbor scan timeout after 120s")

fetchVulns:
	// 3. Fetch vulnerability details
	vulnURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/additions/vulnerabilities",
		h.BaseURL, project, repo, digest)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, vulnURL, nil)
	resp, err = h.do(req)
	if err != nil {
		return nil, fmt.Errorf("harbor fetch vulns: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var wrapper map[string]harborVulnReport
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("harbor parse vulns: %w", err)
	}

	var report *harborVulnReport
	for _, r := range wrapper {
		report = &r
		break
	}
	if report == nil {
		return &Findings{
			SeveritySummary: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0},
			Findings:        []Finding{},
		}, nil
	}

	severitySummary := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0}
	findings := make([]Finding, 0, len(report.Vulnerabilities))
	for _, v := range report.Vulnerabilities {
		sev := toUpper(v.Severity)
		if sev == "" {
			sev = "Unknown"
		}
		severitySummary[toLower(sev)]++

		var cvss3 float64
		if v.CVSS != nil {
			for _, s := range v.CVSS {
				if s.ScoreV3 > cvss3 {
					cvss3 = s.ScoreV3
				}
			}
		}

		findings = append(findings, Finding{
			CVE:          v.ID,
			Severity:     sev,
			CVSS3:        cvss3,
			Package:      v.Package,
			Version:      v.Version,
			FixedVersion: v.FixVersion,
			Title:        v.Title,
			Description:  v.Description,
		})
	}
	return &Findings{SeveritySummary: severitySummary, Findings: findings}, nil
}

type harborVulnReport struct {
	GeneratedAt    time.Time          `json:"generated_at"`
	Severity       string             `json:"severity"`
	Vulnerabilities []harborVulnItem  `json:"vulnerabilities"`
}

type harborVulnItem struct {
	ID          string                 `json:"id"`
	Package     string                 `json:"package"`
	Version     string                 `json:"version"`
	FixVersion  string                 `json:"fix_version"`
	Severity    string                 `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Links       []string               `json:"links"`
	CVSS        map[string]harborCVSS  `json:"CVSS,omitempty"`
}

type harborCVSS struct {
	ScoreV3 float64 `json:"score_v3,omitempty"`
}

// ---------------------------------------------------------------------------
// TrivyClient — standalone Trivy Server (legacy, kept as fallback)
// ---------------------------------------------------------------------------

type TrivyClient struct {
	Endpoint string
	Timeout  time.Duration
	client   *http.Client
}

func NewTrivyClient(endpoint string) *TrivyClient {
	return &TrivyClient{
		Endpoint: endpoint,
		Timeout:  5 * time.Minute,
		client:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (t *TrivyClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Endpoint+"/v1/ping", nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("trivy ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("trivy ping: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (t *TrivyClient) UpdateDB(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint+"/v1/db/update", nil)
	if err != nil {
		return "", err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("trivy update db: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("trivy update db: status %d body=%s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("trivy:%s", time.Now().Format("2006-01-02")), nil
}

type trivyScanRequest struct {
	Image  string `json:"image"`
	Digest string `json:"digest,omitempty"`
}

type trivyResult struct {
	Target          string      `json:"target"`
	Vulnerabilities []trivyVuln `json:"vulnerabilities"`
}

type trivyVuln struct {
	VulnerabilityID  string                `json:"vulnerabilityID"`
	PkgName          string                `json:"pkgName"`
	InstalledVersion string                `json:"installedVersion"`
	FixedVersion     string                `json:"fixedVersion"`
	Severity         string                `json:"severity"`
	Title            string                `json:"title"`
	Description      string                `json:"description"`
	CVSS             map[string]trivyCVSS  `json:"cvss,omitempty"`
}

type trivyCVSS struct {
	V3Score float64 `json:"v3Score,omitempty"`
}

func (t *TrivyClient) Scan(ctx context.Context, imageRef, imageDigest string) (*Findings, error) {
	body := trivyScanRequest{Image: imageRef, Digest: imageDigest}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Endpoint+"/v1/scan", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trivy scan: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trivy scan: status %d body=%s", resp.StatusCode, string(raw))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trivy scan read: %w", err)
	}
	var results []trivyResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, fmt.Errorf("trivy scan parse: %w", err)
	}

	severitySummary := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0}
	var findings []Finding
	for _, r := range results {
		for _, v := range r.Vulnerabilities {
			severity := v.Severity
			if severity == "" {
				severity = "unknown"
			}
			severitySummary[toLower(severity)]++

			var cvss3 float64
			for _, s := range v.CVSS {
				if s.V3Score > cvss3 {
					cvss3 = s.V3Score
				}
			}

			findings = append(findings, Finding{
				CVE:          v.VulnerabilityID,
				Severity:     severity,
				CVSS3:        cvss3,
				Package:      v.PkgName,
				Version:      v.InstalledVersion,
				FixedVersion: v.FixedVersion,
				Title:        v.Title,
				Description:  v.Description,
			})
		}
	}
	return &Findings{SeveritySummary: severitySummary, Findings: findings}, nil
}

// ---------------------------------------------------------------------------
// NativeScanner — stub for environments without a scanner
// ---------------------------------------------------------------------------

type NativeScanner struct{}

func (n *NativeScanner) Ping(ctx context.Context) error { return nil }
func (n *NativeScanner) UpdateDB(ctx context.Context) (string, error) {
	return fmt.Sprintf("native:%s", time.Now().Format("2006-01-02")), nil
}
func (n *NativeScanner) Scan(ctx context.Context, imageRef, imageDigest string) (*Findings, error) {
	return &Findings{
		SeveritySummary: map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0},
		Findings:        []Finding{},
	}, nil
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

func NewScanner(endpoint string) Scanner {
	if endpoint != "" {
		return NewTrivyClient(endpoint)
	}
	return &NativeScanner{}
}

func NewScannerFromHarbor(harborURL, username, password string) Scanner {
	if harborURL != "" {
		return NewHarborScanner(harborURL, username, password)
	}
	return &NativeScanner{}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func toUpper(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}
