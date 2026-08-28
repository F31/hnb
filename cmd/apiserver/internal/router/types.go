package router

import "time"

type RouteConfig struct {
	Path       string        `yaml:"path"`
	Methods    []string      `yaml:"methods"`
	Handler    string        `yaml:"handler"`
	Auth       bool          `yaml:"auth"`
	RateLimit  string        `yaml:"rate_limit,omitempty"`
	Middleware []string      `yaml:"middleware,omitempty"`
	Upstream   *UpstreamCfg  `yaml:"upstream,omitempty"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
	CacheTTL   string        `yaml:"cache_ttl,omitempty"`
	Desc       string        `yaml:"desc,omitempty"`
}

type UpstreamCfg struct {
	Type    string `yaml:"type"`              // "nats" | "local" | "proxy"
	Subject string `yaml:"subject,omitempty"` // NATS subject
	Timeout string `yaml:"timeout,omitempty"` // "30s"
}

type MiddlewareConfig struct {
	Global []string `yaml:"global"`
	Auth   AuthCfg  `yaml:"auth"`
	Rate   RateCfg  `yaml:"rate_limit"`
}

type AuthCfg struct {
	Secret      string   `yaml:"secret"`
	BypassPaths []string `yaml:"bypass_paths"`
}

type RateCfg struct {
	Default   string            `yaml:"default"`
	PerTenant map[string]string `yaml:"per_tenant,omitempty"`
}

type RouteFile struct {
	Routes     []RouteConfig    `yaml:"routes"`
	Middleware MiddlewareConfig `yaml:"middleware"`
}

type MatchedRoute struct {
	Config *RouteConfig
	Params map[string]string
}
