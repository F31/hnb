# Webhook SSRF Protection Design Evidence

## Defense Layers

### Layer 1: URL Validation at Save Time
- Webhook URL must parse as valid HTTPS URL
- Hostname must resolve to an allowed domain or CIDR range
- Blocked: private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8, ::1)
- Blocked: metadata endpoints (169.254.169.254, 100.100.100.200)

### Layer 2: DNS Resolution at Send Time
- Re-resolve the hostname at send time (prevent DNS rebinding)
- Verify resolved IP against allowlist and blocklist
- If IP changed since save time, reject and log

### Layer 3: Connection Validation
- Enforce HTTPS only (no plain HTTP)
- Verify TLS certificate chain
- Prevent redirects to blocked destinations
- Max redirect depth: 3

### Layer 4: Timeout and Rate Limiting
- Connection timeout: 5s
- Request timeout: 10s
- Per-endpoint rate limit: 120/min

## Allowlist Configuration

```yaml
webhook:
  allowedDomains:
    - "hooks.example.com"
    - "alert.example.org"
  allowedCIDRs:
    - "203.0.113.0/24"
  blockedIPs:
    - "169.254.169.254"
  enforceHttps: true
  maxRedirects: 3
```

## Implementation

```go
func validateWebhookURL(rawURL string, config *WebhookSecurityConfig) error {
    u, err := url.Parse(rawURL)
    if err != nil { return err }
    if u.Scheme != "https" { return ErrInsecureScheme }

    // Resolve at save time
    ips, err := net.LookupIP(u.Hostname())
    if err != nil { return err }
    for _, ip := range ips {
        if isPrivate(ip) || isBlocked(ip) { return ErrBlockedDestination }
        if !isAllowed(ip, config.AllowedCIDRs) { return ErrDestinationNotAllowed }
    }

    // Domain check
    if !matchesAllowedDomain(u.Hostname(), config.AllowedDomains) {
        return ErrDomainNotAllowed
    }
    return nil
}

func revalidateAtSendTime(rawURL string, config *WebhookSecurityConfig) error {
    // Re-resolve and check for DNS rebinding
    // Compare with saved resolution
    // Reject if changed
}
```

## Test Plan
- Private IP: 10.0.0.1, 192.168.1.1, 127.0.0.1 -> rejected
- Metadata: 169.254.169.254 -> rejected
- DNS rebinding: hostname resolves to public IP at save, private IP at send -> rejected
- Redirect: redirect from allowed to blocked destination -> rejected
- HTTP: http:// URL -> rejected (HTTPS only)
- Allowlist: hooks.example.com -> allowed, evil.com -> rejected
- Timeout: slow endpoint -> timeout after 10s