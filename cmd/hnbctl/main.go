// Command hnbctl is the unified HNB deployment and cluster-operation CLI.
//
// Design contract (see openspec/changes/standardize-hnb-deployment-toolkit):
//   - Local install stages (init/preflight/prepare/install/status/resume/verify/collect)
//     run on the operator's machine and never touch the owning-plane databases.
//   - Managed operations (cluster create/import, node, platform, operation) only
//     talk to the public versioned API; they never execute providers, write the
//     platform database, or fall back to local SSH when the API is unreachable.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/F31/hnb/cmd/hnbctl/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := cli.ExecuteWithInput(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	os.Exit(code)
}
