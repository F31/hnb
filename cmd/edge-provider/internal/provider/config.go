package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress     string
	CloudCoreEndpoint string
	Kubeconfig        string
	AllowedNamespaces map[string]struct{}
	MaxReplicas       int32
	DiscoveryInterval int
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
		listen = ":18081"
	}

	cloudCoreEndpoint := os.Getenv("CLOUDCORE_ENDPOINT")
	if cloudCoreEndpoint == "" {
		return Config{}, fmt.Errorf("CLOUDCORE_ENDPOINT is required")
	}

	discoveryInterval := 60
	if value := os.Getenv("DISCOVERY_INTERVAL"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 10 {
			return Config{}, fmt.Errorf("DISCOVERY_INTERVAL must be a positive integer >= 10")
		}
		discoveryInterval = parsed
	}

	return Config{
		ListenAddress:     listen,
		CloudCoreEndpoint: cloudCoreEndpoint,
		Kubeconfig:        os.Getenv("KUBECONFIG"),
		AllowedNamespaces: namespaces,
		MaxReplicas:       int32(maxReplicas),
		DiscoveryInterval: discoveryInterval,
	}, nil
}
