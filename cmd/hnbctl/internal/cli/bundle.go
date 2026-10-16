package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runBundle implements the offline closure lifecycle: prepare, verify, import.
// Verification runs before any node modification, so an incomplete closure can
// never be mistaken for an offline-capable installation.
func runBundle(_ context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "bundle subcommand required: prepare|verify|import")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "prepare", "verify", "import":
	default:
		return fail(ReasonInvalidConfig, "unknown bundle subcommand: "+sub)
	}
	fs := flag.NewFlagSet("bundle "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var lockPath, cacheDir, manifestPath, format, osIndexPath, sandboxPath, targetDir string
	fs.StringVar(&lockPath, "lock", "", "release lock")
	fs.StringVar(&cacheDir, "cache-dir", "", "prepared cache / bundle directory")
	fs.StringVar(&manifestPath, "manifest", "", "bundle manifest path")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.StringVar(&osIndexPath, "os-package-index", "", "OS dependency index file inside the cache")
	fs.StringVar(&sandboxPath, "sandbox-image", "", "sandbox image material inside the cache")
	fs.StringVar(&targetDir, "target-dir", "", "independent private destination cache (import only)")
	if err := fs.Parse(rest); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected bundle arguments")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if lockPath == "" || cacheDir == "" {
		return fail(ReasonInvalidConfig, "bundle requires --lock and --cache-dir")
	}
	if manifestPath == "" {
		return fail(ReasonInvalidConfig, "bundle requires --manifest")
	}
	if sub == "import" && targetDir == "" {
		return fail(ReasonInvalidConfig, "bundle import requires --target-dir")
	}
	if sub != "import" && targetDir != "" {
		return fail(ReasonInvalidConfig, "--target-dir applies only to import")
	}

	lock, err := readValidatedStandaloneLock(lockPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}

	// OS dependency closure and sandbox image are explicit inputs: an offline
	// install cannot proceed without them and they are never assumed present.
	var osPackages, sandboxImages []toolkit.BundleEntry
	for _, spec := range []struct{ path, role, name string }{
		{osIndexPath, "os-package-index", "os-package-index"},
		{sandboxPath, "sandbox-image", "sandbox-image"},
	} {
		if spec.path == "" {
			continue
		}
		digest, size, err := fileDigest(spec.path)
		if err != nil {
			return fail(ReasonInvalidConfig, fmt.Sprintf("%s material is unavailable", spec.role))
		}
		entry := toolkit.BundleEntry{Role: spec.role, Name: spec.name, Path: spec.path, Digest: digest, Size: size}
		if spec.role == "os-package-index" {
			osPackages = append(osPackages, entry)
		} else {
			sandboxImages = append(sandboxImages, entry)
		}
	}

	switch sub {
	case "prepare":
		bundle, err := toolkit.BuildOfflineBundle(lock, cacheDir, osPackages, sandboxImages, time.Now().UTC())
		if err != nil {
			return fail(ReasonTargetActionMissing, err.Error())
		}
		if err := toolkit.WriteBundleManifest(manifestPath, bundle); err != nil {
			return fail(ReasonOutputFailed, err.Error())
		}
		return emitBundleResult(env, format, bundle, map[string]any{"bundleId": bundle.BundleID,
			"contentDigest": bundle.ContentDigest, "entries": len(bundle.Entries), "manifest": manifestPath, "complete": true, "offlineInstallCertified": false}, true)

	default: // verify | import
		bundle, err := toolkit.LoadBundleManifest(manifestPath)
		if err != nil {
			return fail(ReasonInvalidConfig, "cannot read bundle manifest")
		}
		verification := toolkit.VerifyOfflineBundle(bundle, lock, cacheDir)
		// Import is gated on the same verification: an incomplete closure is
		// refused before any node modification.
		payload := map[string]any{"bundleId": bundle.BundleID, "lockDigest": bundle.LockDigest,
			"complete": verification.Complete, "missing": verification.Missing,
			"corrupt": verification.Corrupt, "reasons": verification.Reasons, "imported": false, "offlineInstallCertified": false}
		if sub == "verify" {
			return emitBundleResult(env, format, bundle, payload, verification.Complete)
		}
		if !verification.Complete {
			return emitBundleResult(env, format, bundle, payload, false)
		}
		imported, err := toolkit.ImportOfflineBundle(bundle, lock, cacheDir, targetDir)
		if err != nil {
			return fail(ReasonTargetActionMissing, err.Error())
		}
		payload["imported"] = true
		payload["entries"] = len(imported.Entries)
		payload["targetDir"] = targetDir
		payload["importedAt"] = time.Now().UTC().Format(time.RFC3339)
		return emitBundleResult(env, format, bundle, payload, true)
	}
}

func emitBundleResult(env *Env, format string, bundle toolkit.OfflineBundle, payload map[string]any, ok bool) Result {
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(payload); err != nil {
			return fail(ReasonOutputFailed, "cannot write bundle result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "bundle=%s scope=local-material complete=%v offlineInstallCertified=false\n", bundle.BundleID, payload["complete"])
		for _, reason := range reasonsOf(payload) {
			fmt.Fprintf(env.Stdout, "  blocked: %s\n", reason)
		}
	}
	if !ok {
		return fail(ReasonTargetActionMissing, "offline bundle closure is incomplete; node modification must not proceed")
	}
	return success("")
}

func reasonsOf(payload map[string]any) []string {
	raw, _ := payload["reasons"].([]string)
	return raw
}
