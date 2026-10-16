package toolkit

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPProbe performs bounded, diskless observations from the installer. No
// credential registration, artifact cache, image import or redirects are used.
type HTTPProbe struct{ client *http.Client }

func NewHTTPProbe(client *http.Client) HTTPProbe {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if copy.Timeout == 0 {
		copy.Timeout = 15 * time.Second
	}
	return HTTPProbe{client: &copy}
}

func validProbeURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && !strings.ContainsAny(raw, "\r\n\x00")
}

func (p HTTPProbe) request(ctx context.Context, method, address string) (*http.Response, string) {
	if !validProbeURL(address) {
		return nil, "HTTP_REFERENCE_INVALID"
	}
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, "HTTP_REFERENCE_INVALID"
	}
	// Hash the origin's byte representation, not transparent transfer decompression.
	req.Header.Set("Accept-Encoding", "identity")
	if strings.Contains(req.URL.Path, "/manifests/") {
		req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		var certInvalid x509.CertificateInvalidError
		var hostInvalid x509.HostnameError
		var unknownCA x509.UnknownAuthorityError
		if errors.As(err, &certInvalid) || errors.As(err, &hostInvalid) || errors.As(err, &unknownCA) {
			return nil, "TLS_VERIFICATION_FAILED"
		}
		return nil, "HTTP_CONNECTION_FAILED"
	}
	return resp, ""
}

func (p HTTPProbe) ProbeEntry(ctx context.Context, address string) PreflightCheck {
	resp, reason := p.request(ctx, http.MethodGet, address)
	if resp == nil {
		return observedCheck("platform.entry-tls", "platform", "installer -> HTTPS entry with valid chain/SAN/time", "entry/TLS observation failed", reason, "Check the real entry, DNS, route, trusted CA, SAN and validity window; no TLS bypass is used.", false)
	}
	defer resp.Body.Close()
	pass := resp.StatusCode >= 200 && resp.StatusCode < 300
	return observedCheck("platform.entry-tls", "platform", "installer -> HTTPS entry, HTTP 2xx; login not verified", fmt.Sprintf("installer HTTP %d, TLS verified; login not checked", resp.StatusCode), "PLATFORM_ENTRY_HTTP_FAILED", "Check entry routing and server status; redirects/auth responses are not treated as full entry success.", pass)
}

// ProbeContent returns distinct metadata and complete-body identity checks. A
// successful HEAD never substitutes for GET/content verification. The caller's
// budget caps traffic; exceeding it blocks rather than claiming bytes verified.
func (p HTTPProbe) ProbeContent(ctx context.Context, id, address, digest string, maxBytes int64) []PreflightCheck {
	checks, _, _ := p.probeContent(ctx, id, address, digest, maxBytes, false, false)
	return checks
}

func (p HTTPProbe) probeContent(ctx context.Context, id, address, digest string, maxBytes int64, retain, exactSize bool) ([]PreflightCheck, []byte, bool) {
	expected, valid := probeDigest(digest)
	if !valid || !validProbeURL(address) || maxBytes <= 0 || maxBytes > 64*1024*1024 {
		return []PreflightCheck{observedCheck(id+".reference", "artifacts", "HTTPS source, pinned SHA-256 and bounded traffic budget", "invalid source/digest/budget", "ARTIFACT_REFERENCE_INVALID", "Resolve a real content-pinned source before probing; credentials/query strings are not accepted.", false)}, nil, false
	}
	var checks []PreflightCheck
	resp, reason := p.request(ctx, http.MethodHead, address)
	actual, metadataPass := "installer metadata request failed", false
	if resp != nil {
		actual = fmt.Sprintf("installer metadata HTTP %d", resp.StatusCode)
		metadataPass = resp.StatusCode >= 200 && resp.StatusCode < 300
		reason = "ARTIFACT_METADATA_UNAVAILABLE"
		resp.Body.Close()
	}
	checks = append(checks, observedCheck(id+".metadata", "artifacts", "installer -> pinned source metadata", actual, reason, "Check source access; metadata availability does not prove content downloads.", metadataPass))
	resp, reason = p.request(ctx, http.MethodGet, address)
	actual, contentPass := "installer content request failed", false
	var body []byte
	if resp != nil {
		defer resp.Body.Close()
		reason = "ARTIFACT_CONTENT_UNAVAILABLE"
		actual = fmt.Sprintf("installer content HTTP %d", resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			hash := sha256.New()
			var count int64
			var err error
			if retain {
				body, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
				count = int64(len(body))
				_, _ = hash.Write(body)
			} else {
				count, err = io.Copy(hash, io.LimitReader(resp.Body, maxBytes+1))
			}
			switch {
			case count > maxBytes:
				actual, reason = "content exceeds read-only probe budget; not fully verified", "ARTIFACT_PROBE_LIMIT_EXCEEDED"
			case err != nil:
				actual, reason = "content stream incomplete", "ARTIFACT_CONTENT_INCOMPLETE"
			case exactSize && count != maxBytes:
				actual, reason = "content size differs from locked descriptor", "ARTIFACT_SIZE_MISMATCH"
			case fmt.Sprintf("sha256:%x", hash.Sum(nil)) != expected:
				actual, reason = "downloaded content identity differs from lock", "ARTIFACT_DIGEST_MISMATCH"
			default:
				contentPass, actual = true, fmt.Sprintf("installer streamed %d bytes; SHA-256 matches lock; not cached/imported", count)
			}
		}
	}
	checks = append(checks, observedCheck(id+".content", "artifacts", "installer -> complete pinned bytes without disk writes", actual, reason, "Check content access, transfer completeness and locked identity; use prepare for larger verified downloads.", contentPass))
	if !contentPass {
		body = nil
	}
	return checks, body, contentPass
}
