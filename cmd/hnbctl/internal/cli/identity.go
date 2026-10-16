package cli

import (
	"context"
	"flag"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runIdentity generates instance identity, KMS material and a bootstrap admin
// credential into a private directory. It writes nothing into values, reports
// or release metadata.
func runIdentity(_ context.Context, env *Env, args []string) Result {
	if len(args) == 0 || args[0] != "generate" {
		return fail(ReasonInvalidConfig, "identity subcommand required: generate")
	}
	var issuer, audience, entry, dir string
	var validityDays int
	fs := flag.NewFlagSet("identity generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&issuer, "issuer", "", "HTTPS issuer origin with trailing slash")
	fs.StringVar(&audience, "audience", "", "single platform token audience")
	fs.StringVar(&entry, "entry", "", "public entry HTTPS origin")
	fs.StringVar(&dir, "dir", "", "absolute private output directory")
	fs.IntVar(&validityDays, "validity-days", 30, "identity key validity in days")
	if err := fs.Parse(args[1:]); err != nil || issuer == "" || audience == "" || entry == "" || dir == "" || fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "identity generate requires --issuer --audience --entry --dir")
	}
	plan, err := toolkit.NewIdentityPlan(issuer, audience, entry, filepath.Clean(dir),
		time.Duration(validityDays)*24*time.Hour, nil)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	material, err := plan.Generate()
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	// The plaintext password is shown once so the operator can hand it to the
	// first login; it is never written to evidence, state or JSON.
	var out string
	out += "identity material generated:\n"
	out += "  issuer:        " + material.Issuer + "\n"
	out += "  keyId:         " + material.KeyID + "\n"
	out += "  privateKey:    " + plan.PrivateKeyPath + "\n"
	out += "  manifest:      " + plan.ManifestPath + "\n"
	out += "  masterKey:     " + plan.MasterKeyPath + "\n"
	out += "  adminFile:     " + plan.AdminPasswordPath + "\n"
	out += "  adminUser:     " + material.AdminCredential.Username + "\n"
	out += "  adminExpires:  " + material.AdminCredential.ExpiresAt.UTC().Format(time.RFC3339) + "\n"
	out += "  adminPassword: " + material.AdminCredential.Plaintext + "\n"
	io.WriteString(env.Stdout, out)
	io.WriteString(env.Stderr, "identity material is written 0600; the bootstrap password is shown once and must be rotated after first login\n")
	_ = strconv.Itoa(validityDays)
	return success("")
}
