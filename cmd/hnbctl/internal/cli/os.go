package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

type osCatalogItem struct {
	toolkit.NodeOSAdapter
	BootstrapCertified bool                   `json:"bootstrapCertified"`
	Evidence           []toolkit.OSAdaptation `json:"evidence"`
}

// runOS is read-only and needs no deployment config, release lock or network.
// Its versioned JSON contract is independent from the human table layout.
func runOS(ctx context.Context, env *Env, args []string) Result {
	inspect := len(args) > 0 && args[0] == "inspect"
	if inspect {
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("os", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var format string
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "usage: hnbctl os [list|inspect] [--format human|json]")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if inspect {
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		report, err := toolkit.ObserveLocalOS(probeCtx)
		if err != nil {
			return fail(ReasonPreflightBlocked, err.Error())
		}
		if format == "json" {
			if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
				return fail(ReasonOutputFailed, "cannot write host OS observation")
			}
		} else {
			fmt.Fprintf(env.Stdout, "OS: %s/%s kernel=%s cgroup=%s\nadapted: %t; OS/kernel verified: %t; bootstrap certified: %t\nowner: %s; swap entries: %d\nresources: %d CPUs, %d Mi memory, %d Gi available root disk\n", report.OS, report.Arch, report.Kernel, report.Cgroup, report.Adapted, report.OSKernelVerified, report.BootstrapCertified, report.Owner, report.ActiveSwapEntries, report.CPUs, report.MemoryMi, report.AvailableDiskGi)
		}
		if !report.Adapted || !report.OSKernelVerified || report.Cgroup != "cgroup2fs" {
			return fail(ReasonPreflightBlocked, "host OS/kernel/cgroup is not verified; see hnbctl os")
		}
		return success("")
	}
	items := make([]osCatalogItem, 0)
	for _, adapter := range toolkit.SupportedNodeOSAdapters() {
		item := osCatalogItem{NodeOSAdapter: adapter, Evidence: []toolkit.OSAdaptation{}}
		for _, os := range toolkit.CurrentEvidence.CertifiedOS {
			if os == adapter.OS {
				item.BootstrapCertified = true
			}
		}
		for _, evidence := range toolkit.CurrentEvidence.OSAdaptations {
			if evidence.OS == adapter.OS {
				item.Evidence = append(item.Evidence, evidence)
			}
		}
		items = append(items, item)
	}
	if format == "json" {
		report := struct {
			SchemaVersion string          `json:"schemaVersion"`
			Items         []osCatalogItem `json:"items"`
		}{"os-catalog/v1", items}
		if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
			return fail(ReasonOutputFailed, "cannot write OS catalog")
		}
		return success("")
	}
	fmt.Fprintln(env.Stdout, "OS\tPACKAGES\tSSH USER\tBOOTSTRAP CERTIFIED\tVERIFIED SCOPE")
	for _, item := range items {
		var scopes []string
		for _, evidence := range item.Evidence {
			scopes = append(scopes, evidence.Route+"/"+evidence.Distribution+"/"+evidence.Arch+" kernel="+evidence.Kernel)
		}
		fmt.Fprintf(env.Stdout, "%s\t%s\t%s\t%t\t%s\n", item.OS, item.PackageManager,
			item.DefaultSSHUser, item.BootstrapCertified, strings.Join(scopes, "; "))
	}
	fmt.Fprintln(env.Stdout, "OS adaptation and scoped verification do not certify full platform delivery; see hnbctl matrix.")
	return success("")
}
