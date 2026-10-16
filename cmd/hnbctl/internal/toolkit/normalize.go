package toolkit

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Normalize canonicalizes input identities and resolves local paths against the
// config directory. It never reads credentials or writes files, and is idempotent.
func (c *DeploymentConfig) Normalize(baseDir, homeDir string) error {
	if !filepath.IsAbs(baseDir) {
		return fmt.Errorf("configuration base directory must be absolute")
	}
	paths := []*string{&c.Spec.ReleaseLock, &c.Spec.Cluster.KubeconfigFile, &c.Spec.Artifacts.BundlePath}
	for i := range c.Spec.Cluster.Nodes {
		node := &c.Spec.Cluster.Nodes[i]
		paths = append(paths, &node.SSHKeyFile)
		for _, address := range []*string{&node.SSHAddress, &node.InternalAddress} {
			if ip := net.ParseIP(*address); ip != nil {
				*address = ip.String()
			}
		}
	}
	for _, path := range paths {
		if *path == "" {
			continue
		}
		if !validLocalPath(*path) {
			return fmt.Errorf("credential/artifact reference must be a local file path, not inline content or a URL")
		}
		if strings.HasPrefix(*path, "~") {
			if !strings.HasPrefix(*path, "~/") || !filepath.IsAbs(homeDir) {
				return fmt.Errorf("only ~/ expansion with an absolute home directory is supported")
			}
			*path = filepath.Join(homeDir, strings.TrimPrefix(*path, "~/"))
		}
		if !filepath.IsAbs(*path) {
			*path = filepath.Join(baseDir, *path)
		}
		*path = filepath.Clean(*path)
	}
	network := &c.Spec.Cluster.Network
	for _, cidr := range []*string{&network.PodCIDR, &network.ServiceCIDR} {
		if _, prefix, err := net.ParseCIDR(*cidr); err == nil {
			*cidr = prefix.String()
		}
	}
	if host, port, err := net.SplitHostPort(c.Spec.Cluster.APIEndpoint); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			c.Spec.Cluster.APIEndpoint = net.JoinHostPort(ip.String(), port)
		}
	}
	if u, err := url.Parse(c.Spec.Platform.PublicBaseURL); err == nil {
		u.Host = strings.ToLower(u.Host)
		if u.Path == "/" {
			u.Path = ""
		}
		c.Spec.Platform.PublicBaseURL = u.String()
	}
	return nil
}

// ValidateCredentialFiles checks only local file metadata and read access. It
// does not print file content, install keys, or contact any host/API.
func (c *DeploymentConfig) ValidateCredentialFiles() []string {
	var paths []string
	if c.Spec.Cluster.Strategy == StrategyExisting {
		paths = append(paths, c.Spec.Cluster.KubeconfigFile)
	}
	for _, node := range c.Spec.Cluster.Nodes {
		if node.SSHAuth != "password" {
			paths = append(paths, node.SSHKeyFile)
		}
	}
	var problems []string
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			problems = append(problems, fmt.Sprintf("credential file %d must exist, be a regular file and deny group/other access", i))
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("credential file %d is not readable", i))
			continue
		}
		if err := file.Close(); err != nil {
			problems = append(problems, fmt.Sprintf("credential file %d cannot be closed", i))
		}
	}
	return problems
}
