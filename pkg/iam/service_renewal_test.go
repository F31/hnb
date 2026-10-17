package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type renewalTestTimer struct {
	when time.Time
	ch   chan time.Time
}

type renewalTestClock struct {
	mu        sync.Mutex
	now       time.Time
	timers    []renewalTestTimer
	scheduled chan time.Duration
}

func (c *renewalTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *renewalTestClock) After(delay time.Duration) <-chan time.Time {
	c.mu.Lock()
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, renewalTestTimer{when: c.now.Add(delay), ch: ch})
	c.mu.Unlock()
	c.scheduled <- delay
	return ch
}

func (c *renewalTestClock) Advance(delay time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delay)
	pending := c.timers[:0]
	for _, timer := range c.timers {
		if !timer.when.After(c.now) {
			timer.ch <- c.now
		} else {
			pending = append(pending, timer)
		}
	}
	c.timers = pending
}

func nextRenewalDelay(t *testing.T, clock *renewalTestClock) time.Duration {
	t.Helper()
	select {
	case delay := <-clock.scheduled:
		return delay
	case <-time.After(5 * time.Second):
		t.Fatal("renewal did not reach its next wait point")
		return 0
	}
}

type renewalTestIssuer func(context.Context, ServiceCredentialScope) (string, error)

func (f renewalTestIssuer) IssueServiceCredential(ctx context.Context, scope ServiceCredentialScope) (string, error) {
	return f(ctx, scope)
}

type renewalFixture struct {
	clock    *renewalTestClock
	scope    ServiceCredentialScope
	verifier *TokenVerifier
	sign     func(func(*AccessTokenClaims)) (string, error)
}

func newRenewalFixture(t *testing.T) *renewalFixture {
	t.Helper()
	manager, _, _, keys, now := newTestManager(t)
	clock := &renewalTestClock{now: now, scheduled: make(chan time.Duration, 128)}
	base := serviceTokenClaims(t, manager, now)
	scope := ServiceCredentialScope{SubjectID: base.SubjectID, Audience: base.Audiences[0], Permission: base.ScopedPermissions[0]}
	verifier, err := NewTokenVerifier(TokenManagerConfig{
		Issuer: "https://issuer.example", Audience: scope.Audience, AccessTTL: MaxAccessTokenTTL, Now: clock.Now,
	}, keys)
	if err != nil {
		t.Fatal(err)
	}
	var sequence atomic.Int64
	return &renewalFixture{
		clock: clock, scope: scope, verifier: verifier,
		sign: func(mutate func(*AccessTokenClaims)) (string, error) {
			claims := base
			claims.Audiences = append([]string(nil), base.Audiences...)
			claims.ScopedPermissions = append([]ScopedPermission(nil), base.ScopedPermissions...)
			claims.AllowedActions = append([]AuthorizationAction(nil), base.AllowedActions...)
			claims.IssuedAt, claims.NotBefore = clock.Now().Unix(), clock.Now().Unix()
			claims.ExpiresAt = clock.Now().Add(MaxAccessTokenTTL).Unix()
			claims.TokenID = fmt.Sprintf("renewal-%d", sequence.Add(1))
			if mutate != nil {
				mutate(&claims)
			}
			return signAccessToken(keys.key, keys.kid, claims)
		},
	}
}

func (f *renewalFixture) source(t *testing.T, issue renewalTestIssuer) *RenewingTokenSource {
	t.Helper()
	source, err := NewRenewingTokenSource(context.Background(), f.verifier, issue, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	source.clock, source.random = f.clock, func() float64 { return 0.5 }
	t.Cleanup(func() { _ = source.Close() })
	return source
}

func TestRenewingTokenSourceRenewsWhileIdleAcrossTTLs(t *testing.T) {
	f := newRenewalFixture(t)
	var calls atomic.Int64
	source := f.source(t, func(ctx context.Context, scope ServiceCredentialScope) (string, error) {
		if scope != f.scope {
			return "", ErrServiceCredentialRejected
		}
		calls.Add(1)
		return f.sign(nil)
	})
	previous, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if delay := nextRenewalDelay(t, f.clock); delay != 30*time.Second {
			t.Fatalf("renewal delay = %v, want half the 60s lifetime", delay)
		}
		f.clock.Advance(30 * time.Second)
	}
	// A wait point means issuance has already completed. No Token call drove
	// these idle renewals, and this is now three original lifetimes later.
	_ = nextRenewalDelay(t, f.clock)
	got, err := source.Token(context.Background())
	if err != nil || got == previous || calls.Load() != 7 {
		t.Fatalf("idle renewal: err=%v, changed=%v, calls=%d", err, got != previous, calls.Load())
	}
}

func TestRenewingTokenSourceCoalescesColdStartAndCallerCancellation(t *testing.T) {
	f := newRenewalFixture(t)
	var calls atomic.Int64
	started, release := make(chan struct{}), make(chan struct{})
	source := f.source(t, func(ctx context.Context, _ ServiceCredentialScope) (string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return f.sign(nil)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { _, err := source.Token(ctx); canceled <- err }()
	<-started
	const consumers = 100
	results := make(chan string, consumers)
	errorsCh := make(chan error, consumers)
	for i := 0; i < consumers; i++ {
		go func() {
			value, err := source.Token(context.Background())
			results <- value
			errorsCh <- err
		}()
	}
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller: %v", err)
	}
	close(release)
	first := ""
	for i := 0; i < consumers; i++ {
		value := <-results
		if err := <-errorsCh; err != nil || value == "" {
			t.Fatalf("shared issuance failed: %v", err)
		}
		if first == "" {
			first = value
		}
		if value != first {
			t.Fatal("cold callers received different credentials")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want one shared issuance", calls.Load())
	}
}

func TestRenewingTokenSourceTransientFailureExpiresAndRecovers(t *testing.T) {
	f := newRenewalFixture(t)
	var unavailable atomic.Bool
	source := f.source(t, func(context.Context, ServiceCredentialScope) (string, error) {
		if unavailable.Load() {
			return "", errors.New("sensitive issuer details: bearer PRIVATE")
		}
		return f.sign(nil)
	})
	old, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = nextRenewalDelay(t, f.clock)
	expires := f.clock.Now().Add(60 * time.Second)
	unavailable.Store(true)
	f.clock.Advance(30 * time.Second)
	for _, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		delay := nextRenewalDelay(t, f.clock)
		if delay != want {
			t.Fatalf("retry delay = %v, want %v", delay, want)
		}
		got, err := source.Token(context.Background())
		if err != nil || got != old {
			t.Fatal("temporary failure discarded a still-valid credential")
		}
		f.clock.Advance(delay)
	}
	for f.clock.Now().Before(expires) {
		delay := nextRenewalDelay(t, f.clock)
		if delay > 4*time.Second {
			t.Fatalf("retry was not bounded: %v", delay)
		}
		f.clock.Advance(delay)
	}
	_ = nextRenewalDelay(t, f.clock)
	got, err := source.Token(context.Background())
	if got != "" || !errors.Is(err, ErrServiceCredentialUnavailable) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("expired credential did not fail closed with a safe error: empty=%v err=%v", got == "", err)
	}
	unavailable.Store(false)
	f.clock.Advance(4 * time.Second)
	_ = nextRenewalDelay(t, f.clock)
	got, err = source.Token(context.Background())
	if err != nil || got == "" || got == old {
		t.Fatalf("fresh workload proof did not recover access: err=%v", err)
	}
}

func TestRenewingTokenSourceRejectsBroadenedOrUnverifiableCredentials(t *testing.T) {
	for name, mutate := range map[string]func(*AccessTokenClaims){
		"wrong audience": func(c *AccessTokenClaims) { c.Audiences = []string{"other-provider"} },
		"extra audience": func(c *AccessTokenClaims) { c.Audiences = append(c.Audiences, "other-provider") },
		"user identity":  func(c *AccessTokenClaims) { c.SubjectType = "user" },
		"wrong subject":  func(c *AccessTokenClaims) { c.SubjectID = "other-workload" },
		"wrong tenant":   func(c *AccessTokenClaims) { c.TenantID = "other-tenant" },
		"extra action":   func(c *AccessTokenClaims) { c.AllowedActions = append(c.AllowedActions, ActionRead) },
		"broader resource": func(c *AccessTokenClaims) {
			c.ScopedPermissions[0].ResourceID = ""
		},
		"extra permission": func(c *AccessTokenClaims) {
			c.ScopedPermissions = append(c.ScopedPermissions, ScopedPermission{
				TenantID: c.TenantID, ResourceKind: "cluster", ResourceID: "another-target", Action: ActionRead,
			})
		},
		"expired":       func(c *AccessTokenClaims) { c.ExpiresAt = c.IssuedAt },
		"future issued": func(c *AccessTokenClaims) { c.IssuedAt++ },
		"long TTL":      func(c *AccessTokenClaims) { c.ExpiresAt++ },
		"wrong profile": func(c *AccessTokenClaims) { c.ProfileVersion = ObserverProfileVersion },
	} {
		t.Run(name, func(t *testing.T) {
			f := newRenewalFixture(t)
			source := f.source(t, func(context.Context, ServiceCredentialScope) (string, error) { return f.sign(mutate) })
			if value, err := source.Token(context.Background()); value != "" || !errors.Is(err, ErrServiceCredentialRejected) {
				t.Fatalf("invalid issuer response accepted: empty=%v err=%v", value == "", err)
			}
		})
	}
}

func TestRenewingTokenSourcePermanentRejectionInvalidatesCachedCredential(t *testing.T) {
	for _, permanent := range []error{ErrServiceIdentityRevoked, ErrServiceCredentialRejected} {
		t.Run(permanent.Error(), func(t *testing.T) {
			f := newRenewalFixture(t)
			var reject atomic.Bool
			source := f.source(t, func(context.Context, ServiceCredentialScope) (string, error) {
				if reject.Load() {
					return "", fmt.Errorf("secret debug details: %w", permanent)
				}
				return f.sign(nil)
			})
			if _, err := source.Token(context.Background()); err != nil {
				t.Fatal(err)
			}
			_ = nextRenewalDelay(t, f.clock)
			source.mu.Lock()
			changed := source.changed
			source.mu.Unlock()
			reject.Store(true)
			f.clock.Advance(30 * time.Second)
			select {
			case <-changed:
			case <-time.After(5 * time.Second):
				t.Fatal("issuer did not publish rejection")
			}
			if value, err := source.Token(context.Background()); value != "" || err != permanent {
				t.Fatalf("permanent rejection kept old credential: empty=%v err=%v", value == "", err)
			}
		})
	}
}

func TestRenewingTokenSourceCloseCancelsSharedIssuance(t *testing.T) {
	f := newRenewalFixture(t)
	started, stopped := make(chan struct{}), make(chan struct{})
	source := f.source(t, func(ctx context.Context, _ ServiceCredentialScope) (string, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return "", ctx.Err()
	})
	done := make(chan error, 1)
	go func() { _, err := source.Token(context.Background()); done <- err }()
	<-started
	_ = source.Close()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("close did not cancel consumers: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not cancel issuer")
	}
}

func TestRenewingTokenSourceRPCTimeoutAndRedaction(t *testing.T) {
	f := newRenewalFixture(t)
	source := f.source(t, func(ctx context.Context, _ ServiceCredentialScope) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > ServiceCredentialRPCTimeout {
			return "", ErrServiceCredentialRejected
		}
		<-ctx.Done()
		return "", errors.New("PRIVATE issuer body")
	})
	if value, err := source.Token(context.Background()); value != "" || err != ErrServiceCredentialUnavailable {
		t.Fatalf("timed-out issuer: empty=%v err=%v", value == "", err)
	}
	if formatted := fmt.Sprintf("%+v %#v", source, source); strings.Contains(formatted, "PRIVATE") || !strings.Contains(formatted, "redacted") {
		t.Fatal("source formatting leaked internal state")
	}
}

func TestRenewingTokenSourceRestartAndParentCancellation(t *testing.T) {
	f := newRenewalFixture(t)
	issuer := renewalTestIssuer(func(context.Context, ServiceCredentialScope) (string, error) { return f.sign(nil) })
	first := f.source(t, issuer)
	old, err := first.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	parent, cancel := context.WithCancel(context.Background())
	second, err := NewRenewingTokenSource(parent, f.verifier, issuer, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.clock, second.random = f.clock, func() float64 { return 0.5 }
	value, err := second.Token(context.Background())
	if err != nil || value == old {
		t.Fatalf("restart must authenticate for a fresh token: err=%v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", second, second), value) {
		t.Fatal("cached credential leaked through formatting")
	}
	cancel()
	if value, err := second.Token(context.Background()); value != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled parent allowed cached token: empty=%v err=%v", value == "", err)
	}
}

func TestRenewingTokenSourceJitterBounds(t *testing.T) {
	for _, sample := range []float64{0, 1} {
		t.Run(fmt.Sprint(sample), func(t *testing.T) {
			f := newRenewalFixture(t)
			var fail atomic.Bool
			source := f.source(t, func(context.Context, ServiceCredentialScope) (string, error) {
				if fail.Load() {
					return "", ErrServiceCredentialUnavailable
				}
				return f.sign(nil)
			})
			source.random = func() float64 { return sample }
			if _, err := source.Token(context.Background()); err != nil {
				t.Fatal(err)
			}
			delay := nextRenewalDelay(t, f.clock)
			if delay < 27*time.Second || delay > 33*time.Second {
				t.Fatalf("renewal jitter outside 45-55%%: %v", delay)
			}
			fail.Store(true)
			f.clock.Advance(delay)
			delay = nextRenewalDelay(t, f.clock)
			if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
				t.Fatalf("retry jitter outside +/-20%%: %v", delay)
			}
			f.clock.Advance(delay)
			delay = nextRenewalDelay(t, f.clock)
			if delay < 1600*time.Millisecond || delay > 2400*time.Millisecond {
				t.Fatalf("second retry jitter outside +/-20%%: %v", delay)
			}
			f.clock.Advance(delay)
			delay = nextRenewalDelay(t, f.clock)
			if delay < 3200*time.Millisecond || delay > 4*time.Second {
				t.Fatalf("retry jitter exceeded the hard 4s cap: %v", delay)
			}
		})
	}
}

func TestRenewingTokenSourceRejectsUnboundedScope(t *testing.T) {
	f := newRenewalFixture(t)
	issuer := renewalTestIssuer(func(context.Context, ServiceCredentialScope) (string, error) { return f.sign(nil) })
	for name, mutate := range map[string]func(*ServiceCredentialScope){
		"wildcard subject":   func(s *ServiceCredentialScope) { s.SubjectID = "*" },
		"wrong audience":     func(s *ServiceCredentialScope) { s.Audience = "different-provider" },
		"wildcard tenant":    func(s *ServiceCredentialScope) { s.Permission.TenantID = "*" },
		"unbounded resource": func(s *ServiceCredentialScope) { s.Permission.ResourceID = "" },
		"wildcard resource":  func(s *ServiceCredentialScope) { s.Permission.ResourceKind = "*" },
		"invalid kind":       func(s *ServiceCredentialScope) { s.Permission.ResourceKind = "bad:kind" },
		"namespace without parent": func(s *ServiceCredentialScope) {
			s.Permission.NamespaceID = "namespace"
		},
	} {
		t.Run(name, func(t *testing.T) {
			scope := f.scope
			mutate(&scope)
			if source, err := NewRenewingTokenSource(context.Background(), f.verifier, issuer, scope); err == nil {
				_ = source.Close()
				t.Fatal("invalid scope was accepted")
			}
		})
	}
}
