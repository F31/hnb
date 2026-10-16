package cli

import (
	"context"
	"flag"
	"io"
	"strconv"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

// runRecipe shows the node dependency declaration derived from the shipped
// provision scripts, and which parts of it the selected lock actually pins.
//
// The node OS, architecture, image repository and sandbox image are passed
// explicitly: they are provision parameters today and are not yet DeploymentConfig
// fields. Deriving them here from config would invent inputs the contract does
// not carry.
func runRecipe(_ context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "show" {
		return fail(ReasonInvalidConfig, "recipe subcommand required: show")
	}
	var configPath, lockPath, nodeOS, arch, imageRepository, sandboxImage, cniRepository string
	fs := flag.NewFlagSet("recipe show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&configPath, "f", "", "DeploymentConfig path")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock override")
	fs.StringVar(&nodeOS, "os", "", "node OS adapter (see hnbctl os); adaptation does not imply release certification")
	fs.StringVar(&arch, "arch", "amd64", "node architecture")
	fs.StringVar(&imageRepository, "image-repository", "", "explicit kubeadm image repository")
	fs.StringVar(&sandboxImage, "sandbox-image", "", "explicit containerd sandbox image")
	fs.StringVar(&cniRepository, "cni-image-repository", "", "platform CNI image repository")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "recipe show requires -f <DeploymentConfig> plus recipe inputs")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "%v", err)
	}
	if cfg.Spec.Cluster.Strategy != toolkit.StrategyProvision {
		return fail(ReasonInvalidConfig, "node recipes apply to the bootstrap/provision route only")
	}
	recipe, err := toolkit.ResolveNodeRecipe(nodeOS, arch,
		cfg.Spec.Cluster.Network.Plugin, cfg.Spec.Cluster.Network.Version,
		cfg.Spec.Cluster.KubernetesVersion, sandboxImage, imageRepository, cniRepository)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	result, err := yaml.Marshal(recipe)
	if err != nil {
		return fail(ReasonOutputFailed, "node recipe serialization failed")
	}
	io.WriteString(env.Stdout, string(result))
	lock, _, err := readValidatedLock(cfg, lockPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "%v", err)
	}
	gaps := recipe.CheckClosure(lock)
	for _, gap := range gaps {
		io.WriteString(env.Stdout, "closure gap: "+toolkit.SafeReportText(gap)+"\n")
	}
	// An unpinned node dependency is a real gap, not a warning: the recipe is
	// reported truthfully and never presented as an installable closure.
	return fail(ReasonPreflightBlocked, "node recipe has "+strconv.Itoa(len(gaps))+" unpinned or unresolved dependencies")
}
