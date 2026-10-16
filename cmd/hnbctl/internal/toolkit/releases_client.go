package toolkit

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

// ReleaseMaterialEntry is one owning-plane registration of a validated offline
// closure for an install target role. Roles describe the install target, not a
// node bootstrap role inside the provider.
type ReleaseMaterialEntry struct {
	ID             string `json:"id"`
	ReleaseID      string `json:"releaseId"`
	ReleaseVersion string `json:"releaseVersion"`
	Role           string `json:"role"`
	BundleID       string `json:"bundleId"`
	LockDigest     string `json:"lockDigest"`
	ContentDigest  string `json:"contentDigest"`
	ImportedBy     string `json:"importedBy,omitempty"`
}

// RegisterReleaseMaterial submits a locally verified closure to the owning-plane
// release directory. Availability problems return ErrPlatformUnavailable; the
// server refusing a changed fixed release returns ErrRequestRejected.
func (p *PlatformClient) RegisterReleaseMaterial(ctx context.Context, entry ReleaseMaterialEntry) (ReleaseMaterialEntry, bool, error) {
	var result ReleaseMaterialEntry
	if p.BaseURL == "" {
		return result, false, fmt.Errorf("platform base URL is required")
	}
	if strings.TrimSpace(p.Token) == "" {
		return result, false, ErrNotAuthenticated
	}
	if err := validateReleaseMaterialEntry(entry); err != nil {
		return result, false, err
	}
	body, err := json.Marshal(entry)
	if err != nil {
		return result, false, fmt.Errorf("%w: cannot encode release material", ErrRequestRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/api/v1/releases", bytes.NewReader(body))
	if err != nil {
		return result, false, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, false, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusCreated:
		if err := json.Unmarshal(payload, &result); err != nil {
			return result, false, fmt.Errorf("%w: platform returned an unreadable release record", ErrPlatformUnavailable)
		}
		return result, true, nil
	case resp.StatusCode == http.StatusOK:
		if err := json.Unmarshal(payload, &result); err != nil {
			return result, false, fmt.Errorf("%w: platform returned an unreadable release record", ErrPlatformUnavailable)
		}
		return result, false, nil
	case resp.StatusCode == http.StatusConflict:
		return result, false, fmt.Errorf("%w: CONFLICT: fixed release already registered with a different content digest", ErrRequestRejected)
	case resp.StatusCode >= 500:
		return result, false, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	default:
		return result, false, fmt.Errorf("%w: platform returned HTTP %d", ErrRequestRejected, resp.StatusCode)
	}
}

type releaseListResponse struct {
	Releases    []ReleaseMaterialEntry `json:"releases"`
	Total       int                    `json:"total"`
	RetrievedAt time.Time              `json:"retrievedAt"`
}

// ListReleaseMaterials returns the owning-plane release directory entries.
func (p *PlatformClient) ListReleaseMaterials(ctx context.Context) ([]ReleaseMaterialEntry, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("platform base URL is required")
	}
	if strings.TrimSpace(p.Token) == "" {
		return nil, ErrNotAuthenticated
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/releases", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: platform returned HTTP %d", ErrRequestRejected, resp.StatusCode)
	}
	var decoded releaseListResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("%w: platform returned an unreadable release directory", ErrPlatformUnavailable)
	}
	return decoded.Releases, nil
}

func validateReleaseMaterialEntry(entry ReleaseMaterialEntry) error {
	if strings.TrimSpace(entry.ReleaseID) == "" || strings.TrimSpace(entry.ReleaseVersion) == "" {
		return fmt.Errorf("%w: releaseId and releaseVersion are required", ErrRequestRejected)
	}
	switch entry.Role {
	case RoleConsole, string(RoleControlPlane), string(RoleWorker):
	default:
		return fmt.Errorf("%w: role must be console, control-plane or worker", ErrRequestRejected)
	}
	if strings.TrimSpace(entry.BundleID) == "" {
		return fmt.Errorf("%w: bundleId is required", ErrRequestRejected)
	}
	for _, digest := range []string{entry.LockDigest, entry.ContentDigest} {
		if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
			return fmt.Errorf("%w: digests must be sha256 content digests", ErrRequestRejected)
		}
	}
	return nil
}
