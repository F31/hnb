package config

import "strings"

var sensitiveKeyPatterns = []string{
	"password",
	"passwd",
	"secret",
	"api_key",
	"api_key",
	"token",
	"auth",
	"credential",
	"private_key",
	"access_key",
	"secret_key",
}

func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range sensitiveKeyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func Desensitize(value string) string {
	return "***REDACTED***"
}

func DesensitizeMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for k, v := range input {
		if IsSensitiveKey(k) {
			result[k] = Desensitize(v)
		} else {
			result[k] = v
		}
	}
	return result
}

func ResolveAndDesensitize(inputs map[string]string, sensitiveKeys []string) map[string]string {
	result := make(map[string]string, len(inputs))
	for k, v := range inputs {
		isSensitive := false
		for _, sk := range sensitiveKeys {
			if strings.EqualFold(k, sk) || strings.Contains(strings.ToLower(k), strings.ToLower(sk)) {
				isSensitive = true
				break
			}
		}
		if isSensitive {
			result[k] = Desensitize(v)
		} else {
			result[k] = v
		}
	}
	return result
}
