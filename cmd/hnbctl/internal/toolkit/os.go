package toolkit

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/F31/hnb/pkg/provision"
)

type NodeOSAdapter = provision.OSAdapter

// SupportedNodeOSAdapters is a defensive copy; knowing how to prepare an OS is
// intentionally independent from whether a release/route is certified on it.
func SupportedNodeOSAdapters() []NodeOSAdapter { return provision.SupportedOSAdapters() }

func LookupNodeOSAdapter(os string) (NodeOSAdapter, bool) { return provision.LookupOSAdapter(os) }

// HostOSReport is a read-only observation, not installation authorization. The
// same fixed probe/parser is used for local inspection and SSH node preflight.
type HostOSReport struct {
	SchemaVersion      string         `json:"schemaVersion"`
	OS                 string         `json:"os"`
	Arch               string         `json:"arch"`
	Kernel             string         `json:"kernel"`
	Cgroup             string         `json:"cgroup"`
	PackageManager     string         `json:"packageManager"`
	Adapted            bool           `json:"adapted"`
	OSKernelVerified   bool           `json:"osKernelVerified"`
	BootstrapCertified bool           `json:"bootstrapCertified"`
	Owner              string         `json:"owner"`
	ActiveSwapEntries  int64          `json:"activeSwapEntries"`
	CPUs               int64          `json:"cpus"`
	MemoryMi           int64          `json:"memoryMi"`
	AvailableDiskGi    int64          `json:"availableDiskGi"`
	Evidence           []OSAdaptation `json:"evidence"`
}

func ObserveLocalOS(ctx context.Context) (*HostOSReport, error) {
	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.Stdin = strings.NewReader(NodeProbeScript)
	output := &probeOutput{}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("local OS observation failed")
	}
	return hostOSReport(output.String(), CurrentEvidence)
}

func hostOSReport(raw string, evidence *EvidenceMatrix) (*HostOSReport, error) {
	obs, err := parseNodeObservation(raw)
	if err != nil {
		return nil, err
	}
	arch := obs.arch
	if arch == "x86_64" {
		arch = "amd64"
	} else if arch == "aarch64" {
		arch = "arm64"
	}
	os := obs.os + "-" + obs.osVersion
	adapter, adapted := LookupNodeOSAdapter(os)
	report := &HostOSReport{SchemaVersion: "host-os/v1", OS: os, Arch: arch,
		Kernel: obs.kernel, Cgroup: obs.cgroup, PackageManager: adapter.PackageManager,
		Adapted: adapted, OSKernelVerified: evidence.ObservedOSKernelSupported(os, obs.kernel, arch),
		Owner: obs.owner, ActiveSwapEntries: obs.swapEntries, CPUs: obs.cpu,
		MemoryMi: obs.memoryKiB / 1024, AvailableDiskGi: obs.diskKiB / (1024 * 1024), Evidence: []OSAdaptation{}}
	if evidence != nil {
		report.BootstrapCertified = containsStr(evidence.CertifiedOS, os) && containsStr(evidence.CertifiedArch, arch) && report.OSKernelVerified
		for _, entry := range evidence.OSAdaptations {
			if entry.OS == os && entry.Kernel == obs.kernel && entry.Arch == arch {
				report.Evidence = append(report.Evidence, entry)
			}
		}
	}
	return report, nil
}
