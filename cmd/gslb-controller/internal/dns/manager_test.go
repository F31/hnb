package dns

import (
	"testing"
)

func TestSanitizeEndpointName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"app.hnb.cloud", "app-hnb-cloud"},
		{"*.hnb.cloud", "wildcard-hnb-cloud"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		got := sanitizeEndpointName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeEndpointName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func sanitizeEndpointName(name string) string {
	result := ""
	for _, c := range name {
		if c == '*' {
			result += "wildcard"
		} else if c == '.' {
			result += "-"
		} else {
			result += string(c)
		}
	}
	return result
}