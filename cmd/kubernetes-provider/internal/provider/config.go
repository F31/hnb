package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress     string
	Kubeconfig        string
	AllowedNamespaces map[string]struct{}
	MaxReplicas       int32
}

func LoadConfig() (Config, error) {
	namespaces := make(map[string]struct{})
	for _, namespace := range strings.Split(os.Getenv("ALLOWED_NAMESPACES"), ",") {
		if namespace = strings.TrimSpace(namespace); namespace != "" {
			namespaces[namespace] = struct{}{}
		}
	}
	if len(namespaces) == 0 {
		return Config{}, fmt.Errorf("ALLOWED_NAMESPACES must contain at least one namespace")
	}
	maxReplicas := int64(10)
	if value := os.Getenv("MAX_REPLICAS"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed < 1 {
			return Config{}, fmt.Errorf("MAX_REPLICAS must be a positive integer")
		}
		maxReplicas = parsed
	}
	listen := os.Getenv("LISTEN_ADDRESS")
	if listen == "" {
		listen = ":18080"
	}
	return Config{
		ListenAddress: listen, Kubeconfig: os.Getenv("KUBECONFIG"),
		AllowedNamespaces: namespaces, MaxReplicas: int32(maxReplicas),
	}, nil
}
