package config

import (
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/router"
	"gopkg.in/yaml.v3"
)

type RoutesConfig struct {
	Routes     []router.RouteConfig    `yaml:"routes"`
	Middleware router.MiddlewareConfig `yaml:"middleware"`
}

func LoadRoutes(path string) (*RoutesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read routes file: %w", err)
	}

	var cfg RoutesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse routes: %w", err)
	}

	for i := range cfg.Routes {
		if cfg.Routes[i].Timeout == 0 {
			cfg.Routes[i].Timeout = 30 * time.Second
		}
	}

	return &cfg, nil
}

func (c *RoutesConfig) ToRouteFile() *router.RouteFile {
	return &router.RouteFile{
		Routes:     c.Routes,
		Middleware: c.Middleware,
	}
}
