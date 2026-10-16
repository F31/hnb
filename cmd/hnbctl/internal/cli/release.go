package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

var releaseDigestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// runRelease dispatches the owning-plane release material directory commands.
// register and list talk to the platform; verify stays fully local so the
// installer can validate a closure before the platform is ever installed.
func runRelease(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "release subcommand required: register, list or verify")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "register":
		return runReleaseRegister(ctx, env, rest)
	case "list":
		return runReleaseList(ctx, env, rest)
	case "verify":
		return runReleaseVerify(ctx, env, rest)
	default:
		return fail(ReasonInvalidConfig, "unknown release subcommand: "+sub)
	}
}

type releaseFlags struct {
	bundle, lockPath, cacheDir, role, baseURL, tokenFile, format string
	timeout                                                      time.Duration
}

func parseReleaseRequestFlags(fs *flag.FlagSet, f *releaseFlags, args []string, needBundle bool) Result {
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&f.tokenFile, "token-file", "", "file holding the API bearer token (never on the command line)")
	fs.StringVar(&f.bundle, "bundle", "", "offline bundle manifest (bundle.json)")
	fs.StringVar(&f.lockPath, "lock", "", "pinned release lock the bundle was built from")
	fs.StringVar(&f.cacheDir, "cache-dir", "", "root of the content-addressed blob cache the bundle references")
	fs.StringVar(&f.role, "role", "", "install target role: console, control-plane or worker")
	fs.StringVar(&f.format, "format", "human", "stdout format: human or json")
	fs.DurationVar(&f.timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected arguments")
	}
	if f.role != toolkit.RoleConsole && f.role != string(toolkit.RoleControlPlane) && f.role != string(toolkit.RoleWorker) {
		return fail(ReasonInvalidConfig, "release requires --role console, control-plane or worker")
	}
	if needBundle {
		for _, missing := range []struct{ value, name string }{
			{f.bundle, "--bundle"}, {f.lockPath, "--lock"}, {f.cacheDir, "--cache-dir"},
		} {
			if missing.value == "" {
				return fail(ReasonInvalidConfig, "release requires "+missing.name)
			}
		}
	}
	return success("")
}

// loadVerifiedRelease loads the bundle and pinned lock, verifies the local
// content-addressed closure, and refuses when the requested role lacks any of
// its dependency materials.
func (f *releaseFlags) loadVerifiedRelease() (toolkit.OfflineBundle, *toolkit.ReleaseLock, Result) {
	bundle, err := toolkit.LoadBundleManifest(f.bundle)
	if err != nil {
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, err.Error())
	}
	if !releaseDigestRE.MatchString(bundle.LockDigest) || !releaseDigestRE.MatchString(bundle.ContentDigest) {
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, "bundle identity digests are invalid")
	}
	lockData, err := os.ReadFile(f.lockPath)
	if err != nil {
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, "read release lock: "+err.Error())
	}
	lock, err := toolkit.DecodeReleaseLock(lockData)
	if err != nil {
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, "parse release lock: "+err.Error())
	}
	verification := toolkit.VerifyOfflineBundle(bundle, lock, f.cacheDir)
	if !verification.Complete {
		reasons := verification.Reasons
		if len(reasons) == 0 {
			reasons = []string{"local material closure is incomplete"}
		}
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, "bundle verification failed: "+strings.Join(reasons, "; "))
	}
	if closure := toolkit.ValidateRoleClosure(bundle, lock, f.role); len(closure) > 0 {
		return toolkit.OfflineBundle{}, nil, fail(ReasonInvalidConfig, "bundle does not cover "+toolkit.CatalogRoleLabel(f.role)+": "+closure[0])
	}
	return bundle, lock, success("")
}

// runReleaseRegister verifies the local closure first, then registers the same
// content identity on the owning-plane release directory.
func runReleaseRegister(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("release register", flag.ContinueOnError)
	f := &releaseFlags{}
	if out := parseReleaseRequestFlags(fs, f, args, true); out.ExitCode != ExitSuccess {
		return out
	}
	if f.baseURL == "" {
		return fail(ReasonInvalidConfig, "release register requires --platform-url")
	}
	bundle, _, out := f.loadVerifiedRelease()
	if out.ExitCode != ExitSuccess {
		return out
	}
	client, err := newPlatformClient(clusterFlags{baseURL: f.baseURL, tokenFile: f.tokenFile, timeout: f.timeout})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	entry := toolkit.ReleaseMaterialEntry{
		ReleaseID: bundle.ReleaseID, ReleaseVersion: bundle.ReleaseVersion, Role: f.role,
		BundleID: bundle.BundleID, LockDigest: bundle.LockDigest, ContentDigest: bundle.ContentDigest,
	}
	requestCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	registered, created, err := client.RegisterReleaseMaterial(requestCtx, entry)
	if err != nil {
		return platformResult(err)
	}
	if f.format == "json" {
		payload := map[string]any{"entry": registered, "replayed": !created}
		if err := json.NewEncoder(env.Stdout).Encode(payload); err != nil {
			return fail(ReasonOutputFailed, "cannot write release registration")
		}
	} else {
		verb := "registered"
		if !created {
			verb = "already present"
		}
		fmt.Fprintf(env.Stdout, "release %s/%s role=%s %s\nid=%s bundle=%s lock=%s content=%s\n",
			bundle.ReleaseID, bundle.ReleaseVersion, f.role, verb,
			registered.ID, registered.BundleID, registered.LockDigest, registered.ContentDigest)
	}
	return success("")
}

// runReleaseList reads the owning-plane release directory.
func runReleaseList(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("release list", flag.ContinueOnError)
	f := &releaseFlags{}
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&f.tokenFile, "token-file", "", "file holding the API bearer token")
	fs.StringVar(&f.format, "format", "human", "stdout format: human or json")
	fs.DurationVar(&f.timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected arguments")
	}
	if f.baseURL == "" {
		return fail(ReasonInvalidConfig, "release list requires --platform-url")
	}
	client, err := newPlatformClient(clusterFlags{baseURL: f.baseURL, tokenFile: f.tokenFile, timeout: f.timeout})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	requestCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	entries, err := client.ListReleaseMaterials(requestCtx)
	if err != nil {
		return platformResult(err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(env.Stdout, "no release material registered")
		return success("")
	}
	if f.format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(map[string]any{"releases": entries, "total": len(entries)}); err != nil {
			return fail(ReasonOutputFailed, "cannot write release directory")
		}
		return success("")
	}
	for _, e := range entries {
		fmt.Fprintf(env.Stdout, "%s\t%s/%s\t%s\t%s\t%s\n", e.ID, e.ReleaseID, e.ReleaseVersion, e.Role, e.LockDigest, e.ContentDigest)
	}
	return success("")
}

// runReleaseVerify validates a closure purely from local material. Platform
// flags are refused so the check can never fall back to a network source.
func runReleaseVerify(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("release verify", flag.ContinueOnError)
	f := &releaseFlags{}
	if out := parseReleaseRequestFlags(fs, f, args, true); out.ExitCode != ExitSuccess {
		return out
	}
	for _, value := range []string{f.baseURL, f.tokenFile} {
		if value != "" {
			return fail(ReasonInvalidConfig, "release verify is offline and refuses platform flags")
		}
	}
	bundle, lock, out := f.loadVerifiedRelease()
	if out.ExitCode != ExitSuccess {
		return out
	}
	line := "release " + bundle.ReleaseID + "/" + bundle.ReleaseVersion + " role=" + f.role + " closure=ready"
	if f.format == "json" {
		payload := map[string]any{
			"releaseId": bundle.ReleaseID, "releaseVersion": bundle.ReleaseVersion,
			"role": f.role, "closure": "ready", "lockDigest": bundle.LockDigest,
			"contentDigest": bundle.ContentDigest, "compatibility": lock.Compatibility,
		}
		if err := json.NewEncoder(env.Stdout).Encode(payload); err != nil {
			return fail(ReasonOutputFailed, "cannot write release verification")
		}
	} else {
		fmt.Fprintln(env.Stdout, line)
	}
	return success("")
}
