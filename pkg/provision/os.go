package provision

import "strings"

// OSAdapter describes distribution mechanics, not release certification. Keep it
// free of CLI flags, credentials and deployment evidence so all callers share it.
type OSAdapter struct {
	OS                string   `json:"os" yaml:"os"`
	PackageManager    string   `json:"packageManager" yaml:"packageManager"`
	DefaultSSHUser    string   `json:"defaultSSHUser" yaml:"defaultSSHUser"`
	BootstrapPackages []string `json:"bootstrapPackages" yaml:"bootstrapPackages"`
}

var osAdapters = []OSAdapter{
	{OS: "ubuntu-24.04", PackageManager: "apt", DefaultSSHUser: "ubuntu", BootstrapPackages: []string{"apt-transport-https", "ca-certificates", "curl", "gpg"}},
	// APT 3 provides HTTPS itself; apt-transport-https is not a prerequisite.
	{OS: "ubuntu-26.04", PackageManager: "apt", DefaultSSHUser: "ubuntu", BootstrapPackages: []string{"ca-certificates", "curl", "gpg"}},
	{OS: "rocky-10.1", PackageManager: "yum", DefaultSSHUser: "root"},
}

func SupportedOSAdapters() []OSAdapter {
	result := make([]OSAdapter, len(osAdapters))
	for i, adapter := range osAdapters {
		adapter.BootstrapPackages = append([]string(nil), adapter.BootstrapPackages...)
		result[i] = adapter
	}
	return result
}

func LookupOSAdapter(os string) (OSAdapter, bool) {
	for _, adapter := range SupportedOSAdapters() {
		if adapter.OS == os {
			return adapter, true
		}
	}
	return OSAdapter{}, false
}

// aptBootstrapPackagesScript resolves the target OS on the target, never from the
// installer's own /etc/os-release. Unknown Debian derivatives keep the legacy
// package list; their recognition here does not grant an acceptance verdict.
func aptBootstrapPackagesScript() string {
	var script strings.Builder
	script.WriteString("apt_bootstrap_packages='apt-transport-https ca-certificates curl gpg'\n")
	script.WriteString("if [ -r /etc/os-release ]; then\n  . /etc/os-release\n  case \"${ID:-}-${VERSION_ID:-}\" in\n")
	for _, adapter := range osAdapters {
		if adapter.PackageManager == "apt" {
			script.WriteString("    " + adapter.OS + ") apt_bootstrap_packages='" + strings.Join(adapter.BootstrapPackages, " ") + "' ;;\n")
		}
	}
	script.WriteString("  esac\nfi\n")
	return script.String()
}
