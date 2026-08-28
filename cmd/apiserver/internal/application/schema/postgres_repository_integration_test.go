package schema

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// TestPostgresRepositoryPublishAndGet 验证 DB 化 UI Registry 的发布链路：
// 发布写不可变历史 + 提升 active revision + 同事务 Outbox 事件（V2.6 §20.3），
// 读取以 active revision 为权威并覆盖 payload 中的 revision 字段。
// 运行方式：HNB_TEST_POSTGRES_DSN=<dsn> go test ./internal/application/schema/ -run PostgresRepository
func TestPostgresRepositoryPublishAndGet(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	pageID := "it.page." + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM ui_pages WHERE page_id = $1`, pageID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, pageID)
	})

	repo := NewPostgresRepository(db)

	// 首次发布 → revision 1
	first, err := repo.Publish(ctx, testPage(pageID), "actor-a", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if first.Metadata.Revision != 1 {
		t.Fatalf("first revision = %d", first.Metadata.Revision)
	}

	// 读取以 active revision 为权威
	got, ok := repo.GetPage(ctx, pageID)
	if !ok {
		t.Fatal("page not found after publish")
	}
	if got.Metadata.ID != pageID || got.Metadata.Revision != 1 {
		t.Fatalf("got page: id=%s revision=%d", got.Metadata.ID, got.Metadata.Revision)
	}
	if rev, found := repo.ActiveRevision(ctx, pageID); !found || rev != 1 {
		t.Fatalf("active revision = %d found=%v", rev, found)
	}

	// 第二次发布 → revision 2，且两版历史都在
	second, err := repo.Publish(ctx, testPage(pageID), "actor-b", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if second.Metadata.Revision != 2 {
		t.Fatalf("second revision = %d", second.Metadata.Revision)
	}
	var versionCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM ui_page_versions WHERE page_id = $1`, pageID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 {
		t.Fatalf("version history count = %d", versionCount)
	}

	// 同事务 Outbox 事件：message_type 指向 UI 发布事件 subject
	var outboxCount int
	var messageType string
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), min(message_type) FROM outbox_events WHERE aggregate_id = $1`, pageID).Scan(&outboxCount, &messageType); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 2 {
		t.Fatalf("outbox events = %d", outboxCount)
	}
	if messageType != uiPagePublishedSubject {
		t.Fatalf("outbox message_type = %q", messageType)
	}

	// 回滚到 revision 1：active revision 切换、读取回到 rev1、写回滚 Outbox 事件
	rolledBack, err := repo.Rollback(ctx, pageID, 1, "actor-c", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Metadata.Revision != 1 {
		t.Fatalf("rolled back revision = %d", rolledBack.Metadata.Revision)
	}
	gotAfterRollback, ok := repo.GetPage(ctx, pageID)
	if !ok || gotAfterRollback.Metadata.Revision != 1 {
		t.Fatalf("get after rollback: ok=%v revision=%d", ok, gotAfterRollback.Metadata.Revision)
	}
	if rev, _ := repo.ActiveRevision(ctx, pageID); rev != 1 {
		t.Fatalf("active revision after rollback = %d", rev)
	}
	var rolledBackEvents int
	var rolledBackType string
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), min(message_type) FROM outbox_events
		WHERE aggregate_id = $1 AND message_type = $2`, pageID, uiPageRolledBackSubject).Scan(&rolledBackEvents, &rolledBackType); err != nil {
		t.Fatal(err)
	}
	if rolledBackEvents != 1 || rolledBackType != uiPageRolledBackSubject {
		t.Fatalf("rolled back outbox events = %d type=%q", rolledBackEvents, rolledBackType)
	}

	// 回滚到不存在的 revision 报错且不改变状态
	if _, err := repo.Rollback(ctx, pageID, 99, "actor-d", "tenant-a"); err == nil {
		t.Fatal("expected rollback to unknown revision to fail")
	}
	if rev, _ := repo.ActiveRevision(ctx, pageID); rev != 1 {
		t.Fatalf("active revision after failed rollback = %d", rev)
	}
}

// TestPostgresRepositoryPublishIsAtomic 验证失败事务不留半成品：
// 无效 payload（非法 JSON 内容不应发生，这里用不可回滚错误模拟）回滚后
// active revision 与历史均不变。实际以 revision 冲突/约束失败路径覆盖。
func TestPostgresRepositoryPublishRollbackOnOutboxFailure(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	pageID := "it.page." + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM ui_pages WHERE page_id = $1`, pageID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, pageID)
	})

	repo := NewPostgresRepository(db)
	first, err := repo.Publish(ctx, testPage(pageID), "actor-a", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if first.Metadata.Revision != 1 {
		t.Fatalf("revision = %d", first.Metadata.Revision)
	}

	// 手动破坏：直接占用下一 revision，使 Publish 的 INSERT 违反主键 → 整事务回滚
	page := testPage(pageID)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO ui_page_versions (page_id, revision, payload, created_by) VALUES ($1, 2, $2, 'intruder')`,
		pageID, `{"broken":true}`); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Publish(ctx, page, "actor-b", "tenant-a"); err == nil {
		t.Fatal("expected publish to fail on revision conflict")
	}
	// 回滚后 active revision 仍为 1，未产生新的 outbox 事件
	if rev, _ := repo.ActiveRevision(ctx, pageID); rev != 1 {
		t.Fatalf("active revision after rollback = %d", rev)
	}
	var outboxCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, pageID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox events after rollback = %d", outboxCount)
	}
}
