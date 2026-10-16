package cli

// VersionString is the hnbctl version plus the evidence digest the CLI embeds.
// It is intentionally a candidate version; a certified release lock is required
// before install success can be reported.
const VersionString = "hnbctl 0.2.0 (candidate; not a certified HNB platform release)"

// Version of the toolkit schemas this build understands.
const ConfigSchemaVersion = "deployment.hnb.io/v1alpha1"
const LockSchemaVersion = "release-lock/v1"
