package main

import (
	"path/filepath"
	"testing"
)

func TestSanitizePathSegment(t *testing.T) {
	cases := map[string]string{
		"tenant-dev": "tenant-dev",
		"../evil":    "__evil",
		"a/../../b":  "__b",
		"foo:bar":    "foo_bar",
		"/abs":       "_abs",
		"..":         "_",
		".":          "_",
	}
	for input, want := range cases {
		if got := sanitizePathSegment(input); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSessionDirConfinement(t *testing.T) {
	g := &localTransferGateway{root: filepath.Clean("/tmp/staging")}
	for _, tc := range []struct{ tenant, session string }{
		{"tenant-dev", "a/../../b"},
		{"../evil", "session-1"},
		{"..", "../.."},
		{"/abs", "/etc"},
	} {
		dir := g.sessionDir(tc.tenant, tc.session)
		clean := filepath.Clean(dir)
		if clean != filepath.Join(g.root, sanitizePathSegment(tc.tenant), sanitizePathSegment(tc.session)) {
			t.Errorf("sessionDir(%q,%q) = %q escaped expected confinement", tc.tenant, tc.session, clean)
		}
		if !filepath.IsAbs(clean) || (clean != g.root && !hasPrefixDir(clean, g.root)) {
			t.Errorf("sessionDir(%q,%q) = %q not under root %q", tc.tenant, tc.session, clean, g.root)
		}
	}
}

func hasPrefixDir(path, root string) bool {
	return len(path) > len(root) && path[:len(root)] == root && path[len(root)] == filepath.Separator
}
