package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

func runCapacity(_ context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("capacity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var profile, manifestPath, nodesPath, cacheSource, backupSource, scope, evidencePath string
	var materialVerified bool
	fs.StringVar(&profile, "profile", "Minimal", "deployment profile")
	fs.StringVar(&manifestPath, "manifest", "", "rendered Helm manifests")
	fs.StringVar(&nodesPath, "nodes", "", "operator-declared capacity YAML list")
	fs.StringVar(&cacheSource, "cache-source", "local", "download cache placement: local or external")
	fs.StringVar(&backupSource, "backup-source", "local", "backup placement: local or external")
	fs.StringVar(&scope, "scope", "delivery", "planning scope: delivery or startup")
	fs.StringVar(&evidencePath, "evidence", "", "DeploymentConfig whose capability evidence gates install authorization")
	fs.BoolVar(&materialVerified, "material-verified", false, "declare complete locked material was verified (offline closure)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || manifestPath == "" || nodesPath == "" {
		return fail(ReasonInvalidConfig, "capacity requires --manifest and --nodes")
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read rendered manifests")
	}
	file, err := os.Open(nodesPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read capacity declarations")
	}
	defer file.Close()
	var nodes []toolkit.CapacityNode
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&nodes); err != nil {
		return fail(ReasonInvalidConfig, "invalid capacity declarations")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fail(ReasonInvalidConfig, "capacity declarations must be one YAML document")
	}
	opts := toolkit.CapacityPlanningOptions{CacheSource: cacheSource, BackupSource: backupSource, Scope: scope}
	if evidencePath != "" {
		cfg, err := readConfig(evidencePath)
		if err != nil {
			return failf(ReasonInvalidConfig, "cannot read evidence config: %v", err)
		}
		if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
			return failf(ReasonInvalidConfig, "evidence config is invalid: %s", strings.Join(problems, "; "))
		}
		rejections := toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence)
		// Certified is derived from the live evidence gate, never assumed; the
		// material-verified fact is caller-declared (like --db-compatible).
		opts.Authorization = &toolkit.InstallAuthorization{
			Certified:        len(rejections) == 0,
			MaterialVerified: materialVerified,
			Rejections:       rejections,
		}
	}
	report, err := toolkit.BuildCapacityReportWithOptions(toolkit.Profile(profile), manifest, nodes, opts)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
		return fail(ReasonOutputFailed, "cannot write capacity report")
	}
	if !report.FitsDeclaredCapacity {
		return fail(ReasonPreflightBlocked, "capacity planning blocked; see report reason codes")
	}
	return success("")
}
