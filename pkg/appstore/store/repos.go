package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/F31/hnb/pkg/appstore"
	"github.com/google/uuid"
)

var (
	ErrInvalidArtifactReference = errors.New("invalid artifact reference")
	releaseDigestPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type PublisherRepo struct{ db *sql.DB }

func NewPublisherRepo(db *sql.DB) *PublisherRepo { return &PublisherRepo{db: db} }

func (r *PublisherRepo) Create(p *appstore.Publisher) error {
	_, err := r.db.Exec(`INSERT INTO publishers (id, tenant_id, name, display_name, description, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8)`,
		p.ID, p.TenantID, p.Name, p.DisplayName, p.Description, string(p.Status), time.Now(), time.Now())
	return err
}

func (r *PublisherRepo) Get(id, tenantID string) (*appstore.Publisher, error) {
	var p appstore.Publisher
	var desc sql.NullString
	err := r.db.QueryRow(`SELECT id, tenant_id, name, display_name, description, status, created_at, updated_at FROM publishers WHERE id=$1 AND tenant_id=$2`, id, tenantID).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.DisplayName, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	return &p, nil
}

func (r *PublisherRepo) List(tenantID string) ([]appstore.Publisher, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, name, display_name, description, status, created_at, updated_at FROM publishers WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.Publisher
	for rows.Next() {
		var p appstore.Publisher
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.DisplayName, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			p.Description = desc.String
		}
		res = append(res, p)
	}
	return res, nil
}

// DefaultPublisher returns the first active publisher in the tenant. It is
// used by product creation when the caller did not supply a publisher_id.
// A publisher is created lazily as `hnb-official` when the tenant has none.
func (r *PublisherRepo) DefaultPublisher(tenantID string) (*appstore.Publisher, error) {
	var p appstore.Publisher
	var desc sql.NullString
	err := r.db.QueryRow(`SELECT id, tenant_id, name, display_name, description, status, created_at, updated_at FROM publishers WHERE tenant_id=$1 AND status='active' ORDER BY name LIMIT 1`, tenantID).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.DisplayName, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == nil {
		if desc.Valid {
			p.Description = desc.String
		}
		return &p, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now()
	displayName := "HNB Official"
	if _, err := r.db.Exec(`INSERT INTO publishers (id, tenant_id, name, display_name, description, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NULL,$5,$6,$7)`,
		id, tenantID, "hnb-official", displayName, "active", now, now); err != nil {
		if err2 := r.db.QueryRow(`SELECT id, tenant_id, name, display_name, description, status, created_at, updated_at FROM publishers WHERE tenant_id=$1 AND status='active' ORDER BY name LIMIT 1`, tenantID).
			Scan(&p.ID, &p.TenantID, &p.Name, &p.DisplayName, &desc, &p.Status, &p.CreatedAt, &p.UpdatedAt); err2 == nil {
			if desc.Valid {
				p.Description = desc.String
			}
			return &p, nil
		}
		return nil, err
	}
	p.ID = id
	p.TenantID = tenantID
	p.Name = "hnb-official"
	p.DisplayName = displayName
	p.Status = "active"
	p.CreatedAt = now
	p.UpdatedAt = now
	return &p, nil
}

type ProductRepo struct{ db *sql.DB }

func NewProductRepo(db *sql.DB) *ProductRepo { return &ProductRepo{db: db} }

func (r *ProductRepo) Create(p *appstore.Product, tenantID string) error {
	labelsJSON, err := json.Marshal(p.Labels)
	if err != nil {
		return err
	}
	visibility := p.Visibility
	if visibility == "" {
		visibility = "tenant"
	}
	result, err := r.db.Exec(`INSERT INTO products (id, publisher_id, name, display_name, description, category, labels, status, visibility, created_at, updated_at)
		SELECT $1,pub.id,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11 FROM publishers pub
		WHERE pub.id=$2 AND pub.tenant_id=$12`,
		p.ID, p.PublisherID, p.Name, p.DisplayName, p.Description, string(p.Category), labelsJSON, string(p.Status), visibility, time.Now(), time.Now(), tenantID)
	return requireAffected(result, err)
}

func (r *ProductRepo) Get(id, tenantID string) (*appstore.Product, error) {
	var p appstore.Product
	var desc sql.NullString
	var labelsJSON []byte
	var visChangedAt sql.NullTime
	var visChangedBy sql.NullString
	err := r.db.QueryRow(`SELECT p.id, p.publisher_id, p.name, p.display_name, p.description, p.category, p.labels, p.status,
		p.visibility, p.visibility_changed_at, p.visibility_changed_by, p.created_at, p.updated_at
		FROM products p JOIN publishers pub ON pub.id=p.publisher_id WHERE p.id=$1 AND pub.tenant_id=$2`, id, tenantID).
		Scan(&p.ID, &p.PublisherID, &p.Name, &p.DisplayName, &desc, &p.Category, &labelsJSON, &p.Status,
			&p.Visibility, &visChangedAt, &visChangedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if visChangedAt.Valid {
		p.VisibilityChangedAt = &visChangedAt.Time
	}
	if visChangedBy.Valid {
		p.VisibilityChangedBy = visChangedBy.String
	}
	if err := json.Unmarshal(labelsJSON, &p.Labels); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) List(publisherID, tenantID string) ([]appstore.Product, error) {
	rows, err := r.db.Query(`SELECT p.id, p.publisher_id, p.name, p.display_name, p.description, p.category, p.labels, p.status, p.created_at, p.updated_at
		FROM products p JOIN publishers pub ON pub.id=p.publisher_id WHERE p.publisher_id=$1 AND pub.tenant_id=$2 ORDER BY p.name`, publisherID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.Product
	for rows.Next() {
		var p appstore.Product
		var desc sql.NullString
		var labelsJSON []byte
		if err := rows.Scan(&p.ID, &p.PublisherID, &p.Name, &p.DisplayName, &desc, &p.Category, &labelsJSON, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			p.Description = desc.String
		}
		if err := json.Unmarshal(labelsJSON, &p.Labels); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, nil
}

func (r *ProductRepo) Search(tenantID, query, category, scope string, limit, offset int) ([]appstore.Product, int, error) {
	where := "JOIN publishers pub ON pub.id=p.publisher_id WHERE "
	args := []any{}
	argIdx := 1
	switch scope {
	case "public":
		where += fmt.Sprintf("p.visibility='public'")
	case "all":
		where += fmt.Sprintf("(pub.tenant_id=$%d OR p.visibility='public')", argIdx)
		args = append(args, tenantID)
		argIdx++
	default:
		where += fmt.Sprintf("pub.tenant_id=$%d AND p.visibility='tenant'", argIdx)
		args = append(args, tenantID)
		argIdx++
	}

	if query != "" {
		where += fmt.Sprintf(" AND (p.name ILIKE $%d OR p.display_name ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+query+"%")
		argIdx++
	}
	if category != "" {
		where += fmt.Sprintf(" AND p.category=$%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	var total int
	if err := r.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM products p %s`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(fmt.Sprintf(`SELECT p.id, p.publisher_id, p.name, p.display_name, p.description, p.category, p.labels, p.status,
		p.visibility, p.visibility_changed_at, p.visibility_changed_by,
		(SELECT COUNT(*) FROM releases r WHERE r.product_id=p.id), p.created_at, p.updated_at
		FROM products p %s ORDER BY p.name LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var res []appstore.Product
	for rows.Next() {
		var p appstore.Product
		var desc sql.NullString
		var labelsJSON []byte
		var visChangedAt sql.NullTime
		var visChangedBy sql.NullString
		if err := rows.Scan(&p.ID, &p.PublisherID, &p.Name, &p.DisplayName, &desc, &p.Category, &labelsJSON, &p.Status,
			&p.Visibility, &visChangedAt, &visChangedBy, &p.ReleaseCount, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, total, err
		}
		if desc.Valid {
			p.Description = desc.String
		}
		if visChangedAt.Valid {
			p.VisibilityChangedAt = &visChangedAt.Time
		}
		if visChangedBy.Valid {
			p.VisibilityChangedBy = visChangedBy.String
		}
		if err := json.Unmarshal(labelsJSON, &p.Labels); err != nil {
			return nil, total, err
		}
		res = append(res, p)
	}
	return res, total, nil
}

func (r *ProductRepo) Update(p *appstore.Product, tenantID, subjectID string) error {
	labelsJSON, err := json.Marshal(p.Labels)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(`UPDATE products p SET name=$3, display_name=$4, description=NULLIF($5,''), category=$6, labels=$7, status=$8,
		visibility=$9, visibility_changed_at=CASE WHEN $9 IS NOT NULL AND $9!=p.visibility THEN NOW() ELSE p.visibility_changed_at END,
		visibility_changed_by=CASE WHEN $9 IS NOT NULL AND $9!=p.visibility THEN $10 ELSE p.visibility_changed_by END,
		updated_at=NOW()
		WHERE p.id=$1 AND EXISTS (SELECT 1 FROM publishers pub WHERE pub.id=p.publisher_id AND pub.tenant_id=$2)`,
		p.ID, tenantID, p.Name, p.DisplayName, p.Description, string(p.Category), labelsJSON, string(p.Status), p.Visibility, subjectID)
	return requireAffected(result, err)
}

func (r *ProductRepo) Delete(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM products p WHERE p.id=$1 AND EXISTS (SELECT 1 FROM publishers pub WHERE pub.id=p.publisher_id AND pub.tenant_id=$2)`, id, tenantID)
	return requireAffected(result, err)
}

func (r *ProductRepo) UpdateStatus(id, tenantID string, status appstore.ProductStatus) error {
	result, err := r.db.Exec(`UPDATE products p SET status=$3, updated_at=NOW() WHERE p.id=$1 AND EXISTS (SELECT 1 FROM publishers pub WHERE pub.id=p.publisher_id AND pub.tenant_id=$2)`, id, tenantID, string(status))
	return requireAffected(result, err)
}

type ReleaseRepo struct{ db *sql.DB }

func NewReleaseRepo(db *sql.DB) *ReleaseRepo { return &ReleaseRepo{db: db} }

func (r *ReleaseRepo) Create(rel *appstore.Release, tenantID string) error {
	refs, err := extractReleaseArtifactRefs(rel.Manifest)
	if err != nil {
		return err
	}
	manifestJSON, manifestDigest, err := appstore.EncodeReleaseManifest(rel.Manifest)
	if err != nil {
		return err
	}
	if rel.Manifest == nil {
		rel.Manifest = map[string]any{}
	}
	rel.ManifestDigest = manifestDigest
	if len(refs) > 0 {
		tx, err := r.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		result, err := tx.Exec(`INSERT INTO releases (id, product_id, version, release_notes, manifest, manifest_digest, status, created_by, created_at)
			SELECT $1,p.id,$3,NULLIF($4,''),$5,$6,$7,$8,$9 FROM products p
			JOIN publishers pub ON pub.id=p.publisher_id WHERE p.id=$2 AND pub.tenant_id=$10`,
			rel.ID, rel.ProductID, rel.Version, rel.ReleaseNotes, manifestJSON, rel.ManifestDigest, string(rel.Status), rel.CreatedBy, time.Now(), tenantID)
		if err := requireAffected(result, err); err != nil {
			return err
		}
		for i, ref := range refs {
			if err := insertReleaseArtifact(tx, rel.ID, tenantID, ref, i); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	result, err := r.db.Exec(`INSERT INTO releases (id, product_id, version, release_notes, manifest, manifest_digest, status, created_by, created_at)
		SELECT $1,p.id,$3,NULLIF($4,''),$5,$6,$7,$8,$9 FROM products p
		JOIN publishers pub ON pub.id=p.publisher_id WHERE p.id=$2 AND pub.tenant_id=$10`,
		rel.ID, rel.ProductID, rel.Version, rel.ReleaseNotes, manifestJSON, rel.ManifestDigest, string(rel.Status), rel.CreatedBy, time.Now(), tenantID)
	return requireAffected(result, err)
}

func insertReleaseArtifact(tx *sql.Tx, releaseID, tenantID string, ref releaseArtifactRef, position int) error {
	purpose := ref.Purpose
	if purpose == "" {
		purpose = "runtime"
	}
	result, err := tx.Exec(`INSERT INTO release_artifacts (release_id, artifact_id, purpose, position, digest)
		SELECT $1, a.id, $4, $5, a.digest FROM artifacts a
		WHERE a.tenant_id=$2 AND a.digest=$3 AND a.verification_status='verified' AND a.lifecycle_state='active'`,
		releaseID, tenantID, ref.Digest, purpose, position)
	if err := requireAffected(result, err); err != nil {
		return fmt.Errorf("%w: artifact %q", ErrInvalidArtifactReference, ref.Name)
	}
	return nil
}

func (r *ReleaseRepo) Get(id, tenantID string) (*appstore.Release, error) {
	var rel appstore.Release
	var notes sql.NullString
	var manifestJSON []byte
	err := r.db.QueryRow(`SELECT r.id, r.product_id, r.version, r.release_notes, r.manifest, r.manifest_digest, r.status, r.created_by, r.created_at, r.published_at
		FROM releases r JOIN products p ON p.id=r.product_id JOIN publishers pub ON pub.id=p.publisher_id
		WHERE r.id=$1 AND pub.tenant_id=$2`, id, tenantID).
		Scan(&rel.ID, &rel.ProductID, &rel.Version, &notes, &manifestJSON, &rel.ManifestDigest, &rel.Status, &rel.CreatedBy, &rel.CreatedAt, &rel.PublishedAt)
	if err != nil {
		return nil, err
	}
	if notes.Valid {
		rel.ReleaseNotes = notes.String
	}
	if err := json.Unmarshal(manifestJSON, &rel.Manifest); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (r *ReleaseRepo) ListByProduct(productID, tenantID string) ([]appstore.Release, error) {
	rows, err := r.db.Query(`SELECT r.id, r.product_id, r.version, r.release_notes, r.manifest, r.manifest_digest, r.status, r.created_by, r.created_at, r.published_at
		FROM releases r JOIN products p ON p.id=r.product_id JOIN publishers pub ON pub.id=p.publisher_id
		WHERE r.product_id=$1 AND pub.tenant_id=$2 ORDER BY r.created_at DESC`, productID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.Release
	for rows.Next() {
		var rel appstore.Release
		var notes sql.NullString
		var manifestJSON []byte
		if err := rows.Scan(&rel.ID, &rel.ProductID, &rel.Version, &notes, &manifestJSON, &rel.ManifestDigest, &rel.Status, &rel.CreatedBy, &rel.CreatedAt, &rel.PublishedAt); err != nil {
			return nil, err
		}
		if notes.Valid {
			rel.ReleaseNotes = notes.String
		}
		if err := json.Unmarshal(manifestJSON, &rel.Manifest); err != nil {
			return nil, err
		}
		res = append(res, rel)
	}
	return res, nil
}

func (r *ReleaseRepo) Publish(id, tenantID string) error {
	now := time.Now()
	result, err := r.db.Exec(`UPDATE releases r SET status='published', published_at=$2
		WHERE r.id=$1 AND r.status='draft' AND EXISTS (
			SELECT 1 FROM products p JOIN publishers pub ON pub.id=p.publisher_id
			WHERE p.id=r.product_id AND pub.tenant_id=$3
		) AND EXISTS (
			SELECT 1 FROM release_artifacts ra WHERE ra.release_id=r.id
		) AND COALESCE(jsonb_array_length(r.manifest->'artifacts'), 0) = (
			SELECT COUNT(*) FROM release_artifacts ra WHERE ra.release_id=r.id
		) AND NOT EXISTS (
			SELECT 1 FROM release_artifacts ra JOIN artifacts a ON a.id=ra.artifact_id
			WHERE ra.release_id=r.id AND (a.verification_status <> 'verified' OR a.lifecycle_state <> 'active')
		)`, id, now, tenantID)
	return requireAffected(result, err)
}

func (r *ReleaseRepo) Update(rel *appstore.Release, tenantID string) error {
	refs, err := extractReleaseArtifactRefs(rel.Manifest)
	if err != nil {
		return err
	}
	manifestJSON, manifestDigest, err := appstore.EncodeReleaseManifest(rel.Manifest)
	if err != nil {
		return err
	}
	if rel.Manifest == nil {
		rel.Manifest = map[string]any{}
	}
	rel.ManifestDigest = manifestDigest
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE releases r SET version=$3, release_notes=NULLIF($4,''), manifest=$5, manifest_digest=$6, status=$7
		WHERE r.id=$1 AND EXISTS (
			SELECT 1 FROM products p JOIN publishers pub ON pub.id=p.publisher_id
			WHERE p.id=r.product_id AND pub.tenant_id=$2
		)`, rel.ID, tenantID, rel.Version, rel.ReleaseNotes, manifestJSON, rel.ManifestDigest, string(rel.Status))
	if err := requireAffected(result, err); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM release_artifacts WHERE release_id=$1`, rel.ID); err != nil {
		return err
	}
	for i, ref := range refs {
		if err := insertReleaseArtifact(tx, rel.ID, tenantID, ref, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *ReleaseRepo) Delete(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM releases r WHERE r.id=$1 AND EXISTS (
		SELECT 1 FROM products p JOIN publishers pub ON pub.id=p.publisher_id
		WHERE p.id=r.product_id AND pub.tenant_id=$2
	)`, id, tenantID)
	return requireAffected(result, err)
}

type releaseArtifactRef struct {
	Name    string `json:"name"`
	Digest  string `json:"digest"`
	Purpose string `json:"purpose"`
}

func extractReleaseArtifactRefs(manifest any) ([]releaseArtifactRef, error) {
	if manifest == nil {
		return nil, nil
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Artifacts []releaseArtifactRef `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, ref := range doc.Artifacts {
		if !releaseDigestPattern.MatchString(ref.Digest) {
			return nil, fmt.Errorf("%w: artifact %q must use lowercase sha256 digest", ErrInvalidArtifactReference, ref.Name)
		}
	}
	return doc.Artifacts, nil
}

type ApplicationRepo struct{ db *sql.DB }

func NewApplicationRepo(db *sql.DB) *ApplicationRepo { return &ApplicationRepo{db: db} }

func (r *ApplicationRepo) Create(app *appstore.Application, tenantID string) error {
	configJSON, err := json.Marshal(app.Config)
	if err != nil {
		return err
	}
	result, err := r.db.Exec(`INSERT INTO applications (id, tenant_id, workspace_id, group_id, product_id, release_id, name, namespace, status, config, created_at, updated_at)
		SELECT $1,$2,NULLIF($3,''),NULLIF($4,'')::uuid,p.id,r.id,$7,NULLIF($8,''),$9,$10,$11,$12
		FROM releases r JOIN products p ON p.id=r.product_id JOIN publishers pub ON pub.id=p.publisher_id
		WHERE p.id=$5 AND r.id=$6 AND pub.tenant_id=$13`,
		app.ID, tenantID, app.WorkspaceID, app.GroupID, app.ProductID, app.ReleaseID, app.Name, app.Namespace, string(app.Status), configJSON, time.Now(), time.Now(), tenantID)
	return requireAffected(result, err)
}

func (r *ApplicationRepo) Get(id, tenantID string) (*appstore.Application, error) {
	var app appstore.Application
	var workspaceID, groupID, namespace sql.NullString
	var configJSON []byte
	err := r.db.QueryRow(`SELECT id, tenant_id, workspace_id, group_id, product_id, release_id, name, namespace, status, config, created_at, updated_at FROM applications WHERE id=$1 AND tenant_id=$2`, id, tenantID).
		Scan(&app.ID, &app.TenantID, &workspaceID, &groupID, &app.ProductID, &app.ReleaseID, &app.Name, &namespace, &app.Status, &configJSON, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if workspaceID.Valid {
		app.WorkspaceID = workspaceID.String
	}
	if groupID.Valid {
		app.GroupID = groupID.String
	}
	if namespace.Valid {
		app.Namespace = namespace.String
	}
	if err := json.Unmarshal(configJSON, &app.Config); err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *ApplicationRepo) ListByTenant(tenantID string) ([]appstore.Application, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, workspace_id, group_id, product_id, release_id, name, namespace, status, config, created_at, updated_at FROM applications WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.Application
	for rows.Next() {
		var app appstore.Application
		var workspaceID, groupID, namespace sql.NullString
		var configJSON []byte
		if err := rows.Scan(&app.ID, &app.TenantID, &workspaceID, &groupID, &app.ProductID, &app.ReleaseID, &app.Name, &namespace, &app.Status, &configJSON, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, err
		}
		if workspaceID.Valid {
			app.WorkspaceID = workspaceID.String
		}
		if groupID.Valid {
			app.GroupID = groupID.String
		}
		if namespace.Valid {
			app.Namespace = namespace.String
		}
		if err := json.Unmarshal(configJSON, &app.Config); err != nil {
			return nil, err
		}
		res = append(res, app)
	}
	return res, nil
}

func (r *ApplicationRepo) ListByGroup(groupID, tenantID string) ([]appstore.Application, error) {
	rows, err := r.db.Query(`SELECT id, tenant_id, workspace_id, group_id, product_id, release_id, name, namespace, status, config, created_at, updated_at FROM applications WHERE group_id=$1 AND tenant_id=$2 ORDER BY created_at DESC`, groupID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []appstore.Application
	for rows.Next() {
		var app appstore.Application
		var workspaceID, gid, namespace sql.NullString
		var configJSON []byte
		if err := rows.Scan(&app.ID, &app.TenantID, &workspaceID, &gid, &app.ProductID, &app.ReleaseID, &app.Name, &namespace, &app.Status, &configJSON, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, err
		}
		if workspaceID.Valid {
			app.WorkspaceID = workspaceID.String
		}
		if gid.Valid {
			app.GroupID = gid.String
		}
		if namespace.Valid {
			app.Namespace = namespace.String
		}
		if err := json.Unmarshal(configJSON, &app.Config); err != nil {
			return nil, err
		}
		res = append(res, app)
	}
	return res, nil
}

func (r *ApplicationRepo) UpdateStatus(id, tenantID string, status appstore.AppStatus) error {
	result, err := r.db.Exec(`UPDATE applications SET status=$2, updated_at=NOW() WHERE id=$1 AND tenant_id=$3`, id, string(status), tenantID)
	return requireAffected(result, err)
}

func (r *ApplicationRepo) Delete(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM applications WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return requireAffected(result, err)
}

func requireAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ApplicationGroupRepo
type ApplicationGroupRepo struct{ db *sql.DB }

func NewApplicationGroupRepo(db *sql.DB) *ApplicationGroupRepo { return &ApplicationGroupRepo{db: db} }

func (r *ApplicationGroupRepo) List(tenantID string) ([]appstore.ApplicationGroup, error) {
	rows, err := r.db.Query(`
		SELECT g.id, g.tenant_id, g.workspace_id, g.name, g.description, g.namespace,
		       g.group_type, g.labels, g.status, g.created_at, g.updated_at,
		       COALESCE((SELECT count(*) FROM applications a WHERE a.group_id = g.id), 0)
		FROM application_groups g WHERE g.tenant_id=$1 ORDER BY g.created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]appstore.ApplicationGroup, 0)
	for rows.Next() {
		var g appstore.ApplicationGroup
		var wsID, desc, ns, labelsJSON sql.NullString
		if err := rows.Scan(&g.ID, &g.TenantID, &wsID, &g.Name, &desc, &ns,
			&g.GroupType, &labelsJSON, &g.Status, &g.CreatedAt, &g.UpdatedAt, &g.AppCount); err != nil {
			return nil, err
		}
		if wsID.Valid {
			g.WorkspaceID = wsID.String
		}
		g.Description = desc.String
		g.Namespace = ns.String
		if labelsJSON.Valid {
			json.Unmarshal([]byte(labelsJSON.String), &g.Labels)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *ApplicationGroupRepo) Get(id, tenantID string) (*appstore.ApplicationGroup, error) {
	var g appstore.ApplicationGroup
	var wsID, desc, ns, labelsJSON sql.NullString
	err := r.db.QueryRow(`
		SELECT g.id, g.tenant_id, g.workspace_id, g.name, g.description, g.namespace,
		       g.group_type, g.labels, g.status, g.created_at, g.updated_at,
		       COALESCE((SELECT count(*) FROM applications a WHERE a.group_id = g.id), 0)
		FROM application_groups g WHERE g.id=$1 AND g.tenant_id=$2`, id, tenantID).
		Scan(&g.ID, &g.TenantID, &wsID, &g.Name, &desc, &ns,
			&g.GroupType, &labelsJSON, &g.Status, &g.CreatedAt, &g.UpdatedAt, &g.AppCount)
	if err != nil {
		return nil, err
	}
	g.WorkspaceID = wsID.String
	g.Description = desc.String
	g.Namespace = ns.String
	if labelsJSON.Valid {
		json.Unmarshal([]byte(labelsJSON.String), &g.Labels)
	}
	return &g, nil
}

func (r *ApplicationGroupRepo) Create(group *appstore.ApplicationGroup) error {
	labelsJSON, _ := json.Marshal(group.Labels)
	var wsID *string
	if group.WorkspaceID != "" {
		wsID = &group.WorkspaceID
	}
	result, err := r.db.Exec(`
		INSERT INTO application_groups (id, tenant_id, workspace_id, name, description, namespace, group_type, labels, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		group.ID, group.TenantID, wsID, group.Name, group.Description, group.Namespace,
		string(group.GroupType), labelsJSON, group.Status, group.CreatedAt, group.UpdatedAt)
	return requireAffected(result, err)
}

func (r *ApplicationGroupRepo) Update(group *appstore.ApplicationGroup) error {
	labelsJSON, _ := json.Marshal(group.Labels)
	result, err := r.db.Exec(`
		UPDATE application_groups SET name=$1, description=$2, namespace=$3, group_type=$4, labels=$5, status=$6, updated_at=NOW()
		WHERE id=$7 AND tenant_id=$8`,
		group.Name, group.Description, group.Namespace, string(group.GroupType), labelsJSON, group.Status, group.ID, group.TenantID)
	return requireAffected(result, err)
}

func (r *ApplicationGroupRepo) Delete(id, tenantID string) error {
	result, err := r.db.Exec(`DELETE FROM application_groups WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return requireAffected(result, err)
}

// Ensure all repos are used
var _ = &PublisherRepo{}
var _ = &ProductRepo{}
var _ = &ReleaseRepo{}
var _ = &ApplicationRepo{}
var _ = &ApplicationGroupRepo{}
