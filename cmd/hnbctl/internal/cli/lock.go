package cli

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"gopkg.in/yaml.v3"
)

func runLock(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "generate" {
		return fail(ReasonInvalidConfig, "lock subcommand required: generate")
	}
	var inputPath, outputPath string
	fs := flag.NewFlagSet("lock generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&inputPath, "f", "", "release-input/v1 path")
	fs.StringVar(&outputPath, "output", "", "candidate release lock path")
	if err := fs.Parse(args[1:]); err != nil {
		return fail(ReasonInvalidConfig, "invalid lock generate flags")
	}
	if inputPath == "" || outputPath == "" || fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "-f and --output are required without positional arguments")
	}
	// Refuse existing output before hashing files or contacting registries.
	if _, err := os.Lstat(outputPath); err == nil || !os.IsNotExist(err) {
		return fail(ReasonInvalidConfig, "candidate output already exists or is inaccessible")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "release input unavailable")
	}
	input, err := toolkit.DecodeReleaseInput(data)
	if err != nil {
		return fail(ReasonInvalidConfig, "release input could not be strictly decoded")
	}
	absPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "release input path cannot be resolved")
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	lock, err := toolkit.GenerateCandidateLock(resolveCtx, input, filepath.Dir(absPath), toolkit.NewRegistryResolver(nil))
	if err != nil {
		return failf(ReasonInvalidConfig, "candidate resolution failed: %v", err)
	}
	result, err := yaml.Marshal(lock)
	if err != nil {
		return fail(ReasonOutputFailed, "candidate lock serialization failed")
	}
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fail(ReasonOutputFailed, "cannot create candidate lock output")
	}
	n, writeErr := file.Write(result)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || n != len(result) {
		return fail(ReasonOutputFailed, "candidate lock output incomplete")
	}
	return success("candidate release lock generated; artifact identity only, not certified or dependency-closure verified")
}
