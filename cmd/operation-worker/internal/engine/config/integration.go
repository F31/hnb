package config

import (
	"context"
	"strings"
)

const SecretRefPrefix = "ref://secrets/"

func IsSecretReference(value string) bool {
	return strings.HasPrefix(value, SecretRefPrefix)
}

func ParseSecretReference(value string) *SecretReference {
	if !IsSecretReference(value) {
		return nil
	}
	parts := strings.SplitN(strings.TrimPrefix(value, SecretRefPrefix), ":", 2)
	id := parts[0]
	version := 0
	if len(parts) > 1 {
		version = 1
	}
	return &SecretReference{
		ID:      id,
		Name:    id,
		Version: version,
	}
}

type ResolvedInputs struct {
	Resolved map[string]string
	Audit    map[string]string
}

func ResolveStepInputs(ctx context.Context, resolver *SecretResolver, inputs map[string]string) (*ResolvedInputs, error) {
	resolved := make(map[string]string, len(inputs))
	audit := make(map[string]string, len(inputs))

	for key, value := range inputs {
		if IsSecretReference(value) {
			ref := ParseSecretReference(value)
			if ref == nil {
				resolved[key] = value
				audit[key] = value
				continue
			}
			plaintext, err := resolver.Resolve(ctx, ref)
			if err != nil {
				return nil, err
			}
			resolved[key] = string(plaintext)
			audit[key] = value
		} else {
			resolved[key] = value
			if IsSensitiveKey(key) {
				audit[key] = Desensitize(value)
			} else {
				audit[key] = value
			}
		}
	}

	return &ResolvedInputs{
		Resolved: resolved,
		Audit:    audit,
	}, nil
}
