package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func runRecovery(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "recovery verify subcommand required")
	}
	if args[0] != "verify" {
		return fail(ReasonTargetActionMissing, "recovery execution is unavailable; use verify for local inputs only; no SQL, SSH or Operation history writes performed")
	}
	fs := flag.NewFlagSet("recovery verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath, lockPath, receiptPath, backupPath, dir, format string
	fs.StringVar(&configPath, "f", "", "original DeploymentConfig")
	fs.StringVar(&lockPath, "lock", "", "original release lock override")
	fs.StringVar(&receiptPath, "receipt", "", "private initialization receipt")
	fs.StringVar(&backupPath, "backup-manifest", "", "private recovery-backup/v1 manifest")
	fs.StringVar(&dir, "backup-dir", "", "private backup material directory")
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || configPath == "" || receiptPath == "" || backupPath == "" || dir == "" {
		return fail(ReasonInvalidConfig, "recovery verify requires -f, --receipt, --backup-manifest and --backup-dir; no positional arguments")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if _, err := toolkit.ReadRecoveryInput(configPath); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "invalid original DeploymentConfig")
	}
	if lockPath == "" {
		lockPath = cfg.Spec.ReleaseLock
	}
	lockData, err := toolkit.ReadRecoveryInput(lockPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	data, err := toolkit.ReadRecoveryInput(receiptPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	receipt, err := toolkit.DecodeBootstrapReceipt(data)
	if err != nil {
		return fail(ReasonInvalidConfig, "invalid initialization receipt")
	}
	data, err = toolkit.ReadRecoveryInput(backupPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	backup, err := toolkit.DecodeRecoveryBackup(data)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	report, err := toolkit.VerifyRecoveryBackup(ctx, cfg, lockData, receipt, backup, dir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
			return fail(ReasonOutputFailed, "cannot write recovery verification")
		}
	} else {
		if _, err := fmt.Fprintf(env.Stdout, "scope=%s run=%s backup=%s inputsVerified=%v restoreAuthorized=false applied=false\n", report.Scope, report.RunID, report.BackupID, report.InputsVerified); err != nil {
			return fail(ReasonOutputFailed, "cannot write recovery verification")
		}
		for _, blocker := range report.Blockers {
			if _, err := fmt.Fprintf(env.Stdout, "  execution blocked: %s\n", blocker); err != nil {
				return fail(ReasonOutputFailed, "cannot write recovery verification")
			}
		}
	}
	return success("")
}
