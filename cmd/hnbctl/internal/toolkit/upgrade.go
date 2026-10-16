package toolkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// UpgradePlanSchema is the pinned, reviewable platform change plan.
const UpgradePlanSchema = "platform-change-plan/v1"

// ArtifactChange describes one artifact that differs between two release locks.
type ArtifactChange struct {
	Kind string `json:"kind"` // image | chart | cli | node-artifact
	Name string `json:"name"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// MigrationChange records a forward-migration delta between locks.
type MigrationChange struct {
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	Change  string `json:"change"` // added | removed | checksum-changed
	FromSum string `json:"fromChecksum,omitempty"`
	ToSum   string `json:"toChecksum,omitempty"`
}

// LockDiff is the full artifact/migration delta between two release locks.
type LockDiff struct {
	FromReleaseID string            `json:"fromReleaseId"`
	FromVersion   string            `json:"fromVersion"`
	ToReleaseID   string            `json:"toReleaseId"`
	ToVersion     string            `json:"toVersion"`
	Changed       []ArtifactChange  `json:"changed"`
	Migrations    []MigrationChange `json:"migrations"`
	// DestructiveMigration is true when migrations were removed or an applied
	// migration's checksum changed. That can never be undone by rolling back
	// resources and always forces a backup check.
	DestructiveMigration bool `json:"destructiveMigration"`
}

// DiffLocks compares two validated release locks. Both must already pass
// Validate(); this function never repairs or guesses.
func DiffLocks(from, to *ReleaseLock) (LockDiff, error) {
	diff := LockDiff{}
	if from == nil || to == nil {
		return diff, fmt.Errorf("both release locks are required for a version diff")
	}
	diff.FromReleaseID, diff.FromVersion = from.ReleaseID, from.ReleaseVersion
	diff.ToReleaseID, diff.ToVersion = to.ReleaseID, to.ReleaseVersion

	compare := func(kind string, fromItems, toItems map[string]string) {
		for name, toDigest := range toItems {
			fromDigest, existed := fromItems[name]
			switch {
			case !existed:
				diff.Changed = append(diff.Changed, ArtifactChange{Kind: kind, Name: name, To: toDigest})
			case fromDigest != toDigest:
				diff.Changed = append(diff.Changed, ArtifactChange{Kind: kind, Name: name, From: fromDigest, To: toDigest})
			}
		}
		for name, fromDigest := range fromItems {
			if _, still := toItems[name]; !still {
				diff.Changed = append(diff.Changed, ArtifactChange{Kind: kind, Name: name, From: fromDigest})
			}
		}
	}
	images := func(l *ReleaseLock) map[string]string {
		out := map[string]string{}
		for _, item := range l.Images {
			out[item.Name] = item.Digest
		}
		return out
	}
	charts := func(l *ReleaseLock) map[string]string {
		out := map[string]string{}
		for _, item := range l.Charts {
			out[item.Name] = item.Digest
		}
		return out
	}
	cli := func(l *ReleaseLock) map[string]string {
		out := map[string]string{}
		for _, item := range l.CLI {
			key := fmt.Sprintf("%s/%s/%s", item.OS, item.Arch, item.Ref)
			out[key] = item.Digest
		}
		return out
	}
	nodeArtifacts := func(l *ReleaseLock) map[string]string {
		out := map[string]string{}
		for _, item := range l.NodeArtifacts {
			out[item.Ref] = item.Digest
		}
		return out
	}
	compare("image", images(from), images(to))
	compare("chart", charts(from), charts(to))
	compare("cli", cli(from), cli(to))
	compare("node-artifact", nodeArtifacts(from), nodeArtifacts(to))

	key := func(m Migration) string { return m.Owner + "/" + m.Name }
	fromMigrations := map[string]Migration{}
	for _, m := range from.Migrations {
		fromMigrations[key(m)] = m
	}
	toMigrations := map[string]Migration{}
	for _, m := range to.Migrations {
		toMigrations[key(m)] = m
	}
	for k, toM := range toMigrations {
		fromM, existed := fromMigrations[k]
		switch {
		case !existed:
			diff.Migrations = append(diff.Migrations, MigrationChange{Owner: toM.Owner, Name: toM.Name, Change: "added", ToSum: toM.Checksum})
		case fromM.Checksum != toM.Checksum:
			// An already-applied migration whose bytes changed is destructive.
			diff.DestructiveMigration = true
			diff.Migrations = append(diff.Migrations, MigrationChange{Owner: toM.Owner, Name: toM.Name, Change: "checksum-changed", FromSum: fromM.Checksum, ToSum: toM.Checksum})
		}
	}
	for k, fromM := range fromMigrations {
		if _, still := toMigrations[k]; !still {
			// Removing a migration from the set means it can no longer be applied.
			diff.DestructiveMigration = true
			diff.Migrations = append(diff.Migrations, MigrationChange{Owner: fromM.Owner, Name: fromM.Name, Change: "removed", FromSum: fromM.Checksum})
		}
	}

	sort.Slice(diff.Changed, func(i, j int) bool {
		if diff.Changed[i].Kind != diff.Changed[j].Kind {
			return diff.Changed[i].Kind < diff.Changed[j].Kind
		}
		return diff.Changed[i].Name < diff.Changed[j].Name
	})
	sort.Slice(diff.Migrations, func(i, j int) bool {
		if diff.Migrations[i].Owner != diff.Migrations[j].Owner {
			return diff.Migrations[i].Owner < diff.Migrations[j].Owner
		}
		return diff.Migrations[i].Name < diff.Migrations[j].Name
	})
	return diff, nil
}

// PlatformChangePlan is a pinned upgrade or resource-rollback plan.
type PlatformChangePlan struct {
	SchemaVersion string `json:"schemaVersion"`
	Direction     string `json:"direction"` // upgrade | resource-rollback
	// The target is pinned by content identity. A plan never carries a mutable
	// tag or a re-resolvable version string.
	FromLockDigest string   `json:"fromLockDigest"`
	ToLockDigest   string   `json:"toLockDigest"`
	ToReleaseID    string   `json:"toReleaseId"`
	ToVersion      string   `json:"toVersion"`
	Diff           LockDiff `json:"diff"`
	// RequiresBackup is forced by a destructive migration delta.
	RequiresBackup bool   `json:"requiresBackup"`
	BackupDigest   string `json:"backupDigest,omitempty"`
	BackupVerified bool   `json:"backupVerified"`
	DBCompatible   bool   `json:"dbCompatible"`
	// ClaimsDataRestored may only be set on a backup-restore direction.
	ClaimsDataRestored bool      `json:"claimsDataRestored"`
	Authorized         bool      `json:"authorized"`
	Blocked            []string  `json:"blocked,omitempty"`
	GeneratedAt        time.Time `json:"generatedAt"`
}

// BuildPlatformChangePlan derives an authorized-or-blocked change plan. The
// caller supplies observed facts (backup availability, schema compatibility);
// the planner never assumes them.
func BuildPlatformChangePlan(direction string, from, to *ReleaseLock, fromLockDigest string,
	dbCompatible bool, backupDigest, availableBackupDigest string, now time.Time) (PlatformChangePlan, error) {
	plan := PlatformChangePlan{SchemaVersion: UpgradePlanSchema, Direction: direction,
		DBCompatible: dbCompatible, GeneratedAt: now}
	if plan.GeneratedAt.IsZero() {
		plan.GeneratedAt = time.Now().UTC()
	}
	if direction != "upgrade" && direction != "resource-rollback" {
		return plan, fmt.Errorf("unsupported platform change direction %q", direction)
	}
	diff, err := DiffLocks(from, to)
	if err != nil {
		return plan, err
	}
	plan.Diff = diff
	plan.ToReleaseID, plan.ToVersion = to.ReleaseID, to.ReleaseVersion
	plan.ToLockDigest = lockContentDigest(to)
	if fromLockDigest == "" {
		return plan, fmt.Errorf("current release lock digest is required; the plan must pin what it upgrades from")
	}
	plan.FromLockDigest = fromLockDigest
	if !digestPattern.MatchString(plan.FromLockDigest) || !digestPattern.MatchString(plan.ToLockDigest) {
		return plan, fmt.Errorf("release lock digests must be pinned content identities")
	}
	if plan.FromLockDigest == plan.ToLockDigest {
		plan.Blocked = append(plan.Blocked, "NO_CHANGE: target lock is identical to the current lock")
	}

	// A destructive migration delta forces a verified backup.
	if diff.DestructiveMigration {
		plan.RequiresBackup = true
	}
	if plan.RequiresBackup {
		if backupDigest == "" {
			plan.Blocked = append(plan.Blocked, "BACKUP_REQUIRED: the migration delta is destructive and requires a backup")
		} else if backupDigest != availableBackupDigest {
			plan.Blocked = append(plan.Blocked, "BACKUP_UNVERIFIED: the recorded backup is not available or does not match")
		} else {
			plan.BackupVerified = true
			plan.BackupDigest = backupDigest
		}
	}
	// Database compatibility must be established explicitly before changing
	// platform resources.
	if !dbCompatible {
		plan.Blocked = append(plan.Blocked, "DB_COMPATIBILITY_UNVERIFIED: schema compatibility with the target release was not established")
	}
	// A resource rollback can never claim it restored data.
	if direction == "resource-rollback" {
		plan.ClaimsDataRestored = false
		if diff.DestructiveMigration {
			plan.Blocked = append(plan.Blocked, "RESOURCE_ROLLBACK_CANNOT_RESTORE_DATA: recover data via backup-restore or forward-migrate")
		}
	}
	plan.Blocked = uniqueSorted(plan.Blocked)
	plan.Authorized = len(plan.Blocked) == 0
	return plan, nil
}

// lockContentDigest pins a lock by its validated content identity.
func lockContentDigest(lock *ReleaseLock) string {
	if lock == nil {
		return ""
	}
	encoded, err := encodeCanonical(lock)
	if err != nil {
		return ""
	}
	return digestOf(encoded)
}

// AuthorizeGenericTargetChange refuses generic target execution while the
// platform API is unavailable. There is deliberately no local fallback.
func AuthorizeGenericTargetChange(apiReachable bool) error {
	if !apiReachable {
		return fmt.Errorf("%w: platform API unavailable; generic target execution and local fallback are refused", ErrPlatformUnavailable)
	}
	return nil
}

// encodeCanonical produces a stable byte encoding for content identity.
func encodeCanonical(lock *ReleaseLock) ([]byte, error) {
	return json.Marshal(lock)
}

// digestOf returns the pinned sha256 content identity of data.
func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// LockContentDigest exposes the pinned content identity of a validated lock.
func LockContentDigest(lock *ReleaseLock) string { return lockContentDigest(lock) }

// FormatPlatformChangeHuman renders a pinned change plan for review.
func FormatPlatformChangeHuman(w io.Writer, plan PlatformChangePlan) {
	fmt.Fprintf(w, "direction=%s authorized=%v target=%s@%s\n", plan.Direction, plan.Authorized, plan.ToReleaseID, plan.ToVersion)
	fmt.Fprintf(w, "  fromLock=%s\n  toLock=%s\n", plan.FromLockDigest, plan.ToLockDigest)
	fmt.Fprintf(w, "  dbCompatible=%v requiresBackup=%v backupVerified=%v claimsDataRestored=%v\n",
		plan.DBCompatible, plan.RequiresBackup, plan.BackupVerified, plan.ClaimsDataRestored)
	for _, change := range plan.Diff.Changed {
		fmt.Fprintf(w, "  %s %s %s -> %s\n", change.Kind, change.Name, change.From, change.To)
	}
	for _, migration := range plan.Diff.Migrations {
		fmt.Fprintf(w, "  migration %s/%s %s\n", migration.Owner, migration.Name, migration.Change)
	}
	for _, reason := range plan.Blocked {
		fmt.Fprintf(w, "  blocked: %s\n", reason)
	}
}
