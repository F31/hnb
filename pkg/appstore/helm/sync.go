package helm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RepoEntry struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type IndexFile struct {
	APIVersion string            `json:"apiVersion"`
	Entries    map[string][]ChartVersion `json:"entries"`
	Generated  time.Time         `json:"generated"`
}

type ChartVersion struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description,omitempty"`
	APIVersion  string      `json:"apiVersion,omitempty"`
	AppVersion  string      `json:"appVersion,omitempty"`
	Created     time.Time   `json:"created"`
	Digest      string      `json:"digest"`
	URLs        []string    `json:"urls"`
	Icon        string      `json:"icon,omitempty"`
	Keywords    []string    `json:"keywords,omitempty"`
	KubeVersion string      `json:"kubeVersion,omitempty"`
	Deprecated  bool        `json:"deprecated,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Syncer struct {
	client *http.Client
}

func NewSyncer() *Syncer {
	return &Syncer{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Syncer) FetchIndex(repo *RepoEntry) (*IndexFile, error) {
	indexURL := strings.TrimSuffix(repo.URL, "/") + "/index.yaml"
	req, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if repo.Username != "" {
		req.SetBasicAuth(repo.Username, repo.Password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("index fetch failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Parse index.yaml using simple YAML-to-JSON line-based conversion
	index, err := parseIndexYAML(data)
	if err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}

	return index, nil
}

func (s *Syncer) Sync(repo *RepoEntry, onChart func(name, version string, cv *ChartVersion) error) error {
	index, err := s.FetchIndex(repo)
	if err != nil {
		return fmt.Errorf("fetch index: %w", err)
	}

	log.Printf("[helm-sync] syncing %s (%d charts)", repo.URL, len(index.Entries))

	for chartName, versions := range index.Entries {
		for _, cv := range versions {
			if cv.Deprecated {
				continue
			}
			if err := onChart(chartName, cv.Version, &cv); err != nil {
				log.Printf("[helm-sync] chart %s v%s: %v", chartName, cv.Version, err)
			}
		}
	}

	return nil
}

func (s *Syncer) ResolveChartURL(baseURL, chartURL string) string {
	if strings.HasPrefix(chartURL, "http://") || strings.HasPrefix(chartURL, "https://") {
		return chartURL
	}
	base := strings.TrimSuffix(baseURL, "/")
	if strings.HasPrefix(chartURL, "/") {
		return base + chartURL
	}
	u, _ := url.Parse(base + "/" + chartURL)
	return u.String()
}

func parseIndexYAML(data []byte) (*IndexFile, error) {
	// Simple YAML-to-JSON bridge for Helm index.yaml
	// Converts indented YAML key-value pairs to JSON
	lines := strings.Split(string(data), "\n")

	// Build a simplified JSON structure
	// This handles the basic structure of index.yaml
	type entry struct {
		Name     string          `json:"name"`
		Version  string          `json:"version"`
		URLs     []string        `json:"urls"`
		Digest   string          `json:"digest"`
		Created  string          `json:"created"`
		AppVersion string        `json:"appVersion,omitempty"`
		Description string       `json:"description,omitempty"`
		Keywords  []string       `json:"keywords,omitempty"`
		Icon      string         `json:"icon,omitempty"`
	}

	index := &IndexFile{
		Entries: make(map[string][]ChartVersion),
	}

	var currentChart string
	var currentVersion entry
	inVersion := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		// apiVersion / generated at root level
		if indent == 0 && strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if key == "apiVersion" {
				index.APIVersion = val
			}
			continue
		}

		// Chart name at indent 2 (e.g., "  nginx:")
		if indent == 2 && !inVersion && strings.HasSuffix(trimmed, ":") {
			currentChart = strings.TrimSuffix(strings.TrimSpace(trimmed), ":")
			inVersion = false
			continue
		}

		// Version entry at indent 4 (e.g., "    - version: 1.0.0")
		if indent == 4 && strings.HasPrefix(trimmed, "- ") {
			if currentChart != "" {
				// Save previous version
				if currentVersion.Name != "" {
					cv := ChartVersion{
						Name:        currentVersion.Name,
						Version:     currentVersion.Version,
						URLs:        currentVersion.URLs,
						Digest:      currentVersion.Digest,
						AppVersion:  currentVersion.AppVersion,
						Description: currentVersion.Description,
						Keywords:    currentVersion.Keywords,
						Icon:        currentVersion.Icon,
					}
					if currentVersion.Created != "" {
						if t, err := time.Parse(time.RFC3339, currentVersion.Created); err == nil {
							cv.Created = t
						}
					}
					index.Entries[currentChart] = append(index.Entries[currentChart], cv)
				}
				currentVersion = entry{}
				inVersion = true
			}
			continue
		}

		// Field within version entry
		if indent >= 6 && strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			val = strings.Trim(val, "\"'")
			currentVersion.Name = currentChart

			switch key {
			case "version":
				currentVersion.Version = val
			case "appVersion":
				currentVersion.AppVersion = val
			case "description":
				currentVersion.Description = val
			case "digest":
				currentVersion.Digest = val
			case "created":
				currentVersion.Created = val
			case "icon":
				currentVersion.Icon = val
			case "urls":
				// List value will be on subsequent lines
			case "keywords":
				// List value
			case "-":
				if currentVersion.URLs == nil {
					currentVersion.URLs = []string{}
				}
				currentVersion.URLs = append(currentVersion.URLs, val)
			}
			continue
		}

		// List items (- value) at indent 8
		if inVersion && indent == 8 && strings.HasPrefix(trimmed, "- ") {
			val := strings.TrimSpace(trimmed[2:])
			val = strings.Trim(val, "\"'")
			currentVersion.URLs = append(currentVersion.URLs, val)
		}
	}

	// Save last version
	if currentVersion.Name != "" && currentChart != "" {
		cv := ChartVersion{
			Name:        currentVersion.Name,
			Version:     currentVersion.Version,
			URLs:        currentVersion.URLs,
			Digest:      currentVersion.Digest,
			AppVersion:  currentVersion.AppVersion,
			Description: currentVersion.Description,
			Keywords:    currentVersion.Keywords,
			Icon:        currentVersion.Icon,
		}
		if currentVersion.Created != "" {
			if t, err := time.Parse(time.RFC3339, currentVersion.Created); err == nil {
				cv.Created = t
			}
		}
		index.Entries[currentChart] = append(index.Entries[currentChart], cv)
	}

	return index, nil
}

// Ensure json is used
var _ = json.Marshal

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}