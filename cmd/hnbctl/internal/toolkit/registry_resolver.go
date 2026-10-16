package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// RegistryResolver resolves actual registry bytes. RedirectOrigins are the only
// non-origin hosts a registry may redirect content to, declared explicitly by the
// release input and recorded in the lock; content identity still comes from the
// pinned descriptor digest, never from the redirecting host.
type RegistryResolver struct {
	probe            HTTPProbe
	RedirectOrigins  map[string]bool
	ObservedRedirect map[string]bool
}

func NewRegistryResolver(client *http.Client) RegistryResolver {
	return RegistryResolver{probe: NewHTTPProbe(client), RedirectOrigins: map[string]bool{}, ObservedRedirect: map[string]bool{}}
}

// DeclareRedirectOrigins records the operator-accepted redirect endpoints. Each
// entry must be an explicit scheme://host without credentials, path or query.
func (r *RegistryResolver) DeclareRedirectOrigins(origins []string) error {
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("redirect origin must be an explicit scheme://host without credentials, path or query")
		}
		r.RedirectOrigins[strings.ToLower(origin)] = true
	}
	return nil
}

var redirectHostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (r RegistryResolver) redirectAllowed(target *url.URL) bool {
	if target == nil || !redirectHostPattern.MatchString(strings.ToLower(target.Hostname())) {
		return false
	}
	origin := strings.ToLower(target.Scheme + "://" + target.Host)
	return r.RedirectOrigins[origin]
}

func registryReference(ref string) (base, selector string, err error) {
	host, repo, ok := strings.Cut(ref, "/")
	if !ok {
		return "", "", fmt.Errorf("OCI ref requires explicit registry/repository and tag or digest")
	}
	if i := strings.LastIndex(repo, "@"); i >= 0 {
		selector, repo = repo[i+1:], repo[:i]
		if !digestPattern.MatchString(selector) || selector == placeholderDigest {
			return "", "", fmt.Errorf("invalid OCI digest selector")
		}
	} else {
		i := strings.LastIndex(repo, ":")
		if i < 0 {
			return "", "", fmt.Errorf("explicit OCI tag is required")
		}
		selector, repo = repo[i+1:], repo[:i]
		if !tagPattern.MatchString(selector) || strings.EqualFold(selector, "latest") {
			return "", "", fmt.Errorf("latest or invalid OCI tag is forbidden")
		}
	}
	for _, part := range strings.Split(repo, "/") {
		if !repositorySegment.MatchString(part) {
			return "", "", fmt.Errorf("invalid OCI repository")
		}
	}
	base = "https://" + host + "/v2/" + repo
	u, e := url.Parse(base)
	if e != nil || !validProbeURL(base) || u.Host != host {
		return "", "", fmt.Errorf("invalid OCI registry origin")
	}
	return base, selector, nil
}

// Registry content starts on HTTPS. Cross-origin content redirects require an
// explicitly declared HTTP(S) origin, at most four hops, and registry tokens
// never leave their scheme+host origin. Integrity is enforced by verifying the
// response bytes against the pinned descriptor digest, never by trusting a host.
func (r RegistryResolver) get(ctx context.Context, address string, token string) (*http.Response, error) {
	current := address
	origin, _ := url.Parse(address)
	for hop := 0; ; hop++ {
		req, e := http.NewRequestWithContext(ctx, "GET", current, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
		target, _ := url.Parse(current)
		// Credentials stay bound to the registry origin that issued them.
		if token != "" && origin != nil && target != nil && target.Host == origin.Host && target.Scheme == origin.Scheme {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, e := r.probe.client.Do(req)
		if e != nil {
			return nil, e
		}
		switch resp.StatusCode {
		case 301, 302, 307, 308:
			if hop >= 4 {
				resp.Body.Close()
				return nil, fmt.Errorf("registry redirect budget exceeded")
			}
			location := resp.Header.Get("Location")
			resp.Body.Close()
			next, e := url.Parse(location)
			if e != nil || !next.IsAbs() || next.User != nil || next.Fragment != "" || (next.Scheme != "https" && next.Scheme != "http") || strings.ContainsAny(next.Host, "\r\n\x00 ") {
				return nil, fmt.Errorf("registry redirect target is not a credential-free absolute URL")
			}
			if (next.Host != origin.Host || next.Scheme != origin.Scheme) && !r.redirectAllowed(next) {
				return nil, fmt.Errorf("registry redirect origin is not declared in the release source policy")
			}
			r.ObservedRedirect[strings.ToLower(next.Scheme+"://"+next.Host)] = true
			current = next.String()
			continue
		}
		return resp, nil
	}
}

// Anonymous Bearer negotiation uses only the registry challenge's read-only
// service/scope. No operator credentials are used, and acquired tokens are held
// only for this single resolution.
// open negotiates a repository-pull Bearer token and returns a streaming body.
// Identity budgets belong to fetch; layer budgets belong to preparation.
func (r RegistryResolver) open(ctx context.Context, address string, token *string) (*http.Response, error) {
	if !validProbeURL(address) {
		return nil, fmt.Errorf("invalid registry source URL")
	}
	origin, _ := url.Parse(address)
	get := func() (*http.Response, error) { return r.get(ctx, address, *token) }
	resp, err := get()
	if err != nil {
		return nil, fmt.Errorf("registry request failed")
	}
	// A CDN must not cause token acquisition for the original registry.
	if resp.StatusCode == 401 && *token == "" && resp.Request != nil && resp.Request.URL.Host == origin.Host && resp.Request.URL.Scheme == origin.Scheme {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		_, parameters, e := mime.ParseMediaType(strings.ReplaceAll(strings.Replace(challenge, "Bearer ", "Bearer; ", 1), ",", ";"))
		if e != nil || !strings.HasPrefix(challenge, "Bearer ") {
			return nil, fmt.Errorf("unsupported registry authentication challenge")
		}
		parts := strings.SplitN(address, "/v2/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid registry path")
		}
		repository := strings.Split(strings.Split(parts[1], "/manifests/")[0], "/blobs/")[0]
		if scope := parameters["scope"]; scope != "" && scope != "repository:"+repository+":pull" {
			return nil, fmt.Errorf("registry challenge is not repository pull scoped")
		}
		realm := parameters["realm"]
		if !validProbeURL(realm) {
			return nil, fmt.Errorf("registry token realm must be a credential-free HTTPS URL")
		}
		u, _ := url.Parse(realm)
		query := u.Query()
		for _, key := range []string{"service", "scope"} {
			if parameters[key] != "" {
				query.Set(key, parameters[key])
			}
		}
		query.Set("scope", "repository:"+repository+":pull")
		u.RawQuery = query.Encode()
		tokenReq, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if e != nil {
			return nil, fmt.Errorf("invalid token request")
		}
		tokenResp, e := r.probe.client.Do(tokenReq)
		if e != nil {
			return nil, fmt.Errorf("anonymous registry token request failed")
		}
		body, e := io.ReadAll(io.LimitReader(tokenResp.Body, 64*1024+1))
		tokenResp.Body.Close()
		if e != nil || tokenResp.StatusCode != 200 || len(body) > 64*1024 {
			return nil, fmt.Errorf("anonymous registry token unavailable")
		}
		var auth struct {
			Token       string `json:"token"`
			AccessToken string `json:"access_token"`
		}
		if json.Unmarshal(body, &auth) != nil {
			return nil, fmt.Errorf("invalid token response")
		}
		*token = auth.Token
		if *token == "" {
			*token = auth.AccessToken
		}
		if *token == "" || strings.ContainsAny(*token, "\r\n") {
			return nil, fmt.Errorf("invalid anonymous token")
		}
		resp, err = get()
		if err != nil {
			return nil, fmt.Errorf("authenticated registry request failed")
		}
	}
	return resp, nil
}

func (r RegistryResolver) fetch(ctx context.Context, address string, token *string, expected string, expectedSize int64) ([]byte, string, error) {
	resp, err := r.open(ctx, address, token)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return nil, "", fmt.Errorf("registry content incomplete or exceeds identity budget")
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
	if (expected != "" && digest != expected) || (expectedSize > 0 && int64(len(body)) != expectedSize) {
		return nil, "", fmt.Errorf("registry bytes do not match pinned descriptor")
	}
	if header := resp.Header.Get("Docker-Content-Digest"); header != "" && header != digest {
		return nil, "", fmt.Errorf("registry digest header differs from response bytes")
	}
	return body, digest, nil
}

func (r RegistryResolver) Resolve(ctx context.Context, name, ref string) (PinnedImage, error) {
	var result PinnedImage
	if !validName(name) {
		return result, fmt.Errorf("invalid image name")
	}
	base, selector, err := registryReference(ref)
	if err != nil {
		return result, err
	}
	token := ""
	expected := ""
	if strings.HasPrefix(selector, "sha256:") {
		expected = selector
	}
	body, rootDigest, err := r.fetch(ctx, base+"/manifests/"+selector, &token, expected, 0)
	if err != nil {
		return result, err
	}
	var manifest ociManifest
	if json.Unmarshal(body, &manifest) != nil || manifest.SchemaVersion != 2 {
		return result, fmt.Errorf("invalid image manifest")
	}
	manifestDigest := rootDigest
	if manifest.MediaType == "application/vnd.oci.image.index.v1+json" || manifest.MediaType == "application/vnd.docker.distribution.manifest.list.v2+json" {
		var selected []ociDescriptor
		for _, descriptor := range manifest.Manifests {
			if p := descriptor.Platform; p != nil && p.OS == "linux" && p.Architecture == "amd64" && p.Variant == "" {
				selected = append(selected, descriptor)
			}
		}
		if len(selected) != 1 || !digestPattern.MatchString(selected[0].Digest) || selected[0].Digest == placeholderDigest || selected[0].Size <= 0 {
			return result, fmt.Errorf("one verified linux/amd64 manifest is required")
		}
		body, manifestDigest, err = r.fetch(ctx, base+"/manifests/"+selected[0].Digest, &token, selected[0].Digest, selected[0].Size)
		if err != nil {
			return result, err
		}
		if json.Unmarshal(body, &manifest) != nil {
			return result, fmt.Errorf("invalid target manifest")
		}
	}
	if manifest.SchemaVersion != 2 || (manifest.MediaType != "application/vnd.oci.image.manifest.v1+json" && manifest.MediaType != "application/vnd.docker.distribution.manifest.v2+json") {
		return result, fmt.Errorf("unsupported target manifest type")
	}
	if !digestPattern.MatchString(manifest.Config.Digest) || manifest.Config.Digest == placeholderDigest || manifest.Config.Size <= 0 {
		return result, fmt.Errorf("invalid image config descriptor")
	}
	config, _, err := r.fetch(ctx, base+"/blobs/"+manifest.Config.Digest, &token, manifest.Config.Digest, manifest.Config.Size)
	if err != nil {
		return result, err
	}
	var platform ociPlatform
	if json.Unmarshal(config, &platform) != nil || platform.OS != "linux" || platform.Architecture != "amd64" || platform.Variant != "" {
		return result, fmt.Errorf("image config does not identify linux/amd64")
	}
	for _, layer := range manifest.Layers {
		if !digestPattern.MatchString(layer.Digest) || layer.Digest == placeholderDigest || layer.Size <= 0 {
			return result, fmt.Errorf("invalid layer descriptor")
		}
	}
	return PinnedImage{Name: name, Ref: ref, Digest: rootDigest, ManifestDigest: manifestDigest, Platform: "linux/amd64"}, nil
}
