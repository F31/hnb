package schema

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// uiPagePublishedSubject / uiPageRolledBackSubject 是 UI Registry 事件的 NATS
// subject（仓库事件命名约定 hnb.event.<domain>.<action>.v1，对应 UI 规范 §22.3
// 的 hnb.ui.page.published / hnb.ui.page.rolled-back 概念事件，由 Outbox Relay
// 投递到 JetStream）。
const (
	uiPagePublishedSubject = "hnb.event.ui.page-published.v1"
	uiPageRolledBackSubject = "hnb.event.ui.page-rolled-back.v1"
)

// PostgresRepository 从 UI Registry（migration 079）读取/发布 PageSchema。
// 读取以 ui_pages.active_revision 为权威，历史存于 ui_page_versions；
// 发布在同一事务内写入新 revision 与 Outbox 事件，满足 V2.6 §20.3 的
// “发布新 revision → Outbox + NATS 事件”闭环。
type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) GetPage(ctx context.Context, id string) (Page, bool) {
	var revision int
	var payload []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT pv.revision, pv.payload
		FROM ui_page_versions pv
		JOIN ui_pages p ON p.page_id = pv.page_id AND p.active_revision = pv.revision
		WHERE pv.page_id = $1`, id).Scan(&revision, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, false
	}
	if err != nil {
		log.Printf("[schema] get page %q: %v", id, err)
		return Page{}, false
	}
	var page Page
	if err := json.Unmarshal(payload, &page); err != nil {
		log.Printf("[schema] unmarshal page %q payload: %v", id, err)
		return Page{}, false
	}
	// 以表内 active revision 为准，防止 payload 与表漂移
	page.Metadata.Revision = revision
	return page, true
}

func (r *PostgresRepository) ActiveRevision(ctx context.Context, id string) (int, bool) {
	var revision int
	err := r.db.QueryRowContext(ctx,
		`SELECT active_revision FROM ui_pages WHERE page_id = $1`, id).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		log.Printf("[schema] active revision of %q: %v", id, err)
		return 0, false
	}
	return revision, true
}

// Publish 事务内：行锁计算下一 revision → 写不可变历史 → 提升 active_revision
// → 写 Outbox 事件（与业务事实同事务，Relay 确认 JetStream 持久化后才标记发布）。
func (r *PostgresRepository) Publish(ctx context.Context, page Page, actorID, tenantID string) (Page, error) {
	id := page.Metadata.ID
	if id == "" {
		return Page{}, errors.New("page id required")
	}
	page.Metadata.Revision = 0 // 由仓库分配

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Page{}, fmt.Errorf("begin publish transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextRevision int
	err = tx.QueryRowContext(ctx, `
		SELECT active_revision + 1
		FROM ui_pages
		WHERE page_id = $1
		FOR UPDATE`, id).Scan(&nextRevision)
	if errors.Is(err, sql.ErrNoRows) {
		// 首次发布
		nextRevision = 1
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ui_pages (page_id, plugin_id, min_shell_version, active_revision)
			VALUES ($1, $2, $3, 1)`,
			id, page.Metadata.PluginID, firstNonEmpty(page.Metadata.MinShellVersion, "2.5.0"),
		); err != nil {
			return Page{}, fmt.Errorf("insert ui_pages: %w", err)
		}
	} else if err != nil {
		return Page{}, fmt.Errorf("lock ui_pages: %w", err)
	}

	page.Metadata.Revision = nextRevision
	payload, err := json.Marshal(page)
	if err != nil {
		return Page{}, fmt.Errorf("marshal page payload: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ui_page_versions (page_id, revision, payload, created_by)
		VALUES ($1, $2, $3, NULLIF($4, ''))`,
		id, nextRevision, string(payload), actorID,
	); err != nil {
		return Page{}, fmt.Errorf("insert ui_page_versions: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE ui_pages
		SET active_revision = $2, updated_at = now()
		WHERE page_id = $1`, id, nextRevision); err != nil {
		return Page{}, fmt.Errorf("bump active revision: %w", err)
	}

	if err := insertPublishOutboxEvent(ctx, tx, id, nextRevision, actorID, tenantID); err != nil {
		return Page{}, fmt.Errorf("insert outbox event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Page{}, fmt.Errorf("commit publish transaction: %w", err)
	}
	return page, nil
}

// insertPublishOutboxEvent 与发布同事务写入 Outbox（V2.6 §20.3 / §15.3）。
func insertPublishOutboxEvent(ctx context.Context, tx *sql.Tx, pageID string, revision int, actorID, tenantID string) error {
	payload, err := json.Marshal(map[string]any{
		"pageId":   pageID,
		"revision": revision,
		"etag":     fmt.Sprintf("page-%s-r%d", pageID, revision),
	})
	if err != nil {
		return err
	}
	correlationID := uuid.NewString()
	idempotencyKey := fmt.Sprintf("ui-page-publish-%s-%d", pageID, revision)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, expected_version, payload
		) VALUES (
			$1, $2, 'v1', $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13
		)`,
		uuid.NewString(), uiPagePublishedSubject, uiPagePublishedSubject,
		time.Now().UTC(), tenantID, actorID, correlationID, idempotencyKey,
		pageID, revision, pageID, revision, string(payload),
	)
	return err
}

// Rollback 将页面切换回历史 revision（V2.6 §20.4）：目标 revision 必须存在；
// 只提升 active_revision，不覆盖历史；同事务写 Outbox 回滚事件。
func (r *PostgresRepository) Rollback(ctx context.Context, id string, targetRevision int, actorID, tenantID string) (Page, error) {
	if id == "" {
		return Page{}, errors.New("page id required")
	}
	if targetRevision < 1 {
		return Page{}, errors.New("target revision must be >= 1")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Page{}, fmt.Errorf("begin rollback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var payload []byte
	err = tx.QueryRowContext(ctx, `
		SELECT payload
		FROM ui_page_versions
		WHERE page_id = $1 AND revision = $2
		FOR UPDATE`, id, targetRevision).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, fmt.Errorf("%w: page %q revision %d", ErrRevisionNotFound, id, targetRevision)
	}
	if err != nil {
		return Page{}, fmt.Errorf("lock target revision: %w", err)
	}

	var page Page
	if err := json.Unmarshal(payload, &page); err != nil {
		return Page{}, fmt.Errorf("unmarshal target revision payload: %w", err)
	}
	page.Metadata.Revision = targetRevision

	if _, err := tx.ExecContext(ctx, `
		UPDATE ui_pages
		SET active_revision = $2, updated_at = now()
		WHERE page_id = $1`, id, targetRevision); err != nil {
		return Page{}, fmt.Errorf("switch active revision: %w", err)
	}

	if err := insertRollbackOutboxEvent(ctx, tx, id, targetRevision, actorID, tenantID); err != nil {
		return Page{}, fmt.Errorf("insert outbox event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Page{}, fmt.Errorf("commit rollback transaction: %w", err)
	}
	return page, nil
}

// insertRollbackOutboxEvent 与回滚同事务写入 Outbox（V2.6 §20.4 回滚事件触发缓存与前端刷新）。
func insertRollbackOutboxEvent(ctx context.Context, tx *sql.Tx, pageID string, revision int, actorID, tenantID string) error {
	payload, err := json.Marshal(map[string]any{
		"pageId":   pageID,
		"revision": revision,
		"etag":     fmt.Sprintf("page-%s-r%d", pageID, revision),
	})
	if err != nil {
		return err
	}
	correlationID := uuid.NewString()
	idempotencyKey := fmt.Sprintf("ui-page-rollback-%s-%d", pageID, revision)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, actor_id, correlation_id, idempotency_key,
			aggregate_id, aggregate_version, resource_id, expected_version, payload
		) VALUES (
			$1, $2, 'v1', $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13
		)`,
		uuid.NewString(), uiPageRolledBackSubject, uiPageRolledBackSubject,
		time.Now().UTC(), tenantID, actorID, correlationID, idempotencyKey,
		pageID, revision, pageID, revision, string(payload),
	)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
