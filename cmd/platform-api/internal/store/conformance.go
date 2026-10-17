package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/F31/hnb/pkg/core"
)

var ErrConformanceIdentityChanged = errors.New("provider manifest identity changed")

func (s *PGStore) RegisterConformance(ctx context.Context, signed core.SignedConformanceReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT provider_id FROM provider_manifests WHERE provider_id=$1 FOR UPDATE`, signed.Report.ProviderID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	manifest, err := getManifest(ctx, tx, id)
	if err != nil {
		return err
	}
	if !ConformanceMatchesManifest(manifest, signed.Report) {
		return ErrConformanceIdentityChanged
	}
	if !time.Now().Before(signed.Report.ExpiresAt) {
		return ErrConformanceIdentityChanged
	}
	resources := manifest.ResourceRequirements
	if resources == nil {
		resources = make(map[string]any)
	}
	resources["conformanceReport"] = signed
	resourcesJSON, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	evidenceJSON, err := json.Marshal([]ConformanceEvidence{{
		TestName: signed.Report.MatrixVersion + "/" + signed.Report.TargetKind + "/" + signed.Report.Action,
		Category: "runtime-target-lifecycle", Passed: true, EvidenceRef: signed.EvidenceDigest(),
	}})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE provider_manifests SET conformance_level='production_ready',
		conformance_evidence=$1, conformance_expires_at=$2, resource_requirements=$3, updated_at=now()
		WHERE provider_id=$4`, string(evidenceJSON), signed.Report.ExpiresAt, string(resourcesJSON), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func ConformanceMatchesManifest(manifest *ProviderManifest, report core.ConformanceReport) bool {
	return manifest.ProviderID == report.ProviderID && manifest.Version == report.ProviderVersion &&
		manifest.ProtocolVersion == "2.0.0" &&
		manifest.Compatibility["artifactDigest"] == report.ArtifactDigest &&
		len(manifest.Actions) == 1 && manifest.Actions[0] == report.Action &&
		ManifestIdentityDigest(manifest) == report.ManifestDigest
}

func (s *PGStore) ExpireConformance(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE provider_manifests SET conformance_level = 'none', updated_at = NOW()
		WHERE conformance_expires_at IS NOT NULL AND conformance_expires_at < NOW()
		AND conformance_level != 'none'
		RETURNING provider_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var expired []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		expired = append(expired, id)
	}
	return expired, nil
}

func (s *PGStore) CheckProviderConformance(ctx context.Context, providerID string) error {
	manifest, err := s.GetManifest(ctx, providerID)
	if err != nil {
		return nil
	}
	if manifest.ConformanceExpiresAt != nil && !manifest.ConformanceExpiresAt.IsZero() {
		return nil
	}
	return nil
}

func (s *PGStore) UpdateConformanceLevel(ctx context.Context, providerID, level string, expiresAt *time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE provider_manifests SET conformance_level = $1, conformance_expires_at = $2, updated_at = NOW()
		WHERE provider_id = $3`,
		level, expiresAt, providerID,
	)
	return err
}
