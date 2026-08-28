package schema

import (
	"context"
	"testing"

	"github.com/F31/hnb/pkg/iam"
)

// stubRepo 内存仓库，用于验证 Service 的读取派生与发布语义（无需数据库）。
type stubRepo struct {
	pages     map[string]Page
	rev       map[string]int
	published []Page
}

func (s *stubRepo) GetPage(_ context.Context, id string) (Page, bool) {
	p, ok := s.pages[id]
	if !ok {
		return Page{}, false
	}
	// 与 PostgresRepository 一致：以表内 active revision 覆盖 payload 中的值
	p.Metadata.Revision = s.rev[id]
	return p, true
}

func (s *stubRepo) ActiveRevision(_ context.Context, id string) (int, bool) {
	r, ok := s.rev[id]
	return r, ok
}

func (s *stubRepo) Publish(_ context.Context, page Page, _, _ string) (Page, error) {
	s.published = append(s.published, page)
	rev := s.rev[page.Metadata.ID] + 1
	s.rev[page.Metadata.ID] = rev
	page.Metadata.Revision = rev
	s.pages[page.Metadata.ID] = page
	return page, nil
}

func (s *stubRepo) Rollback(_ context.Context, id string, targetRevision int, _, _ string) (Page, error) {
	current, ok := s.pages[id]
	if !ok {
		return Page{}, ErrNotFound
	}
	if s.rev[id] < targetRevision {
		return Page{}, ErrRevisionNotFound
	}
	s.rev[id] = targetRevision
	current.Metadata.Revision = targetRevision
	s.pages[id] = current
	return current, nil
}

func newStubRepo() *stubRepo {
	return &stubRepo{pages: map[string]Page{}, rev: map[string]int{}, published: []Page{}}
}

func testPage(id string) Page {
	page := Page{APIVersion: "ui.hnb.io/v1", Kind: "PageSchema"}
	page.Metadata.ID = id
	page.Spec.Template = "list"
	page.Spec.Regions = []Region{{ID: "r1", ComponentType: "DataTable"}}
	return page
}

func trustedWith(permissions ...iam.ScopedPermission) iam.TrustedContext {
	return iam.TrustedContext{
		SubjectID: "subject-a",
		TenantID:  "tenant-a",
		PolicyVersion: "p1",
		ScopedPermissions: permissions,
	}
}

func readPermission() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceSchema), Action: iam.ActionRead}
}

func updatePermission() iam.ScopedPermission {
	return iam.ScopedPermission{TenantID: "tenant-a", ResourceKind: string(iam.ResourceSchema), Action: iam.ActionUpdate}
}

func TestGetDecoratesEtagAndGeneratedAt(t *testing.T) {
	repo := newStubRepo()
	page := testPage("cluster.list")
	repo.rev["cluster.list"] = 3
	repo.pages["cluster.list"] = page
	service := NewService(repo)

	got, err := service.Get(context.Background(), "cluster.list", trustedWith(readPermission()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Revision != 3 {
		t.Fatalf("revision = %d", got.Metadata.Revision)
	}
	if got.Metadata.Etag != "page-cluster.list-r3" {
		t.Fatalf("etag = %q", got.Metadata.Etag)
	}
	if got.Metadata.GeneratedAt == "" {
		t.Fatal("generatedAt not decorated")
	}
}

func TestPublishBumpsRevisionAndWritesOutboxCompatibleEvent(t *testing.T) {
	repo := newStubRepo()
	service := NewService(repo)

	published, err := service.Publish(context.Background(), testPage("cluster.list"), trustedWith(updatePermission()))
	if err != nil {
		t.Fatal(err)
	}
	if published.Metadata.Revision != 1 {
		t.Fatalf("first revision = %d", published.Metadata.Revision)
	}
	if published.Metadata.Etag != "page-cluster.list-r1" {
		t.Fatalf("etag = %q", published.Metadata.Etag)
	}

	second, err := service.Publish(context.Background(), testPage("cluster.list"), trustedWith(updatePermission()))
	if err != nil {
		t.Fatal(err)
	}
	if second.Metadata.Revision != 2 {
		t.Fatalf("second revision = %d", second.Metadata.Revision)
	}
	if len(repo.published) != 2 {
		t.Fatalf("published count = %d", len(repo.published))
	}
}

func TestPublishRequiresUpdatePermission(t *testing.T) {
	service := NewService(newStubRepo())
	_, err := service.Publish(context.Background(), testPage("cluster.list"), trustedWith(readPermission()))
	if err != ErrForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishRejectsNonPageSchemaEnvelope(t *testing.T) {
	service := NewService(newStubRepo())
	page := testPage("cluster.list")
	page.Kind = "FormSchema"
	if _, err := service.Publish(context.Background(), page, trustedWith(updatePermission())); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestGetFailsClosedWithoutPermission(t *testing.T) {
	repo := newStubRepo()
	repo.rev["cluster.list"] = 1
	repo.pages["cluster.list"] = testPage("cluster.list")
	service := NewService(repo)

	if _, err := service.Get(context.Background(), "cluster.list", trustedWith()); err != ErrForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestRollbackSwitchesToTargetRevision(t *testing.T) {
	repo := newStubRepo()
	service := NewService(repo)

	if _, err := service.Publish(context.Background(), testPage("cluster.list"), trustedWith(updatePermission())); err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(context.Background(), testPage("cluster.list"), trustedWith(updatePermission()))
	if err != nil {
		t.Fatal(err)
	}
	if second.Metadata.Revision != 2 {
		t.Fatalf("revision = %d", second.Metadata.Revision)
	}

	rolledBack, err := service.Rollback(context.Background(), "cluster.list", 1, trustedWith(updatePermission()))
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Metadata.Revision != 1 {
		t.Fatalf("rolled back revision = %d", rolledBack.Metadata.Revision)
	}
	if rolledBack.Metadata.Etag != "page-cluster.list-r1" {
		t.Fatalf("etag = %q", rolledBack.Metadata.Etag)
	}
	// 回滚后读取到 revision 1
	got, err := service.Get(context.Background(), "cluster.list", trustedWith(readPermission()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Revision != 1 {
		t.Fatalf("get after rollback revision = %d", got.Metadata.Revision)
	}
}

func TestRollbackRequiresUpdatePermission(t *testing.T) {
	service := NewService(newStubRepo())
	if _, err := service.Rollback(context.Background(), "cluster.list", 1, trustedWith(readPermission())); err != ErrForbidden {
		t.Fatalf("err = %v", err)
	}
}

func TestRollbackRejectsInvalidTarget(t *testing.T) {
	repo := newStubRepo()
	repo.rev["cluster.list"] = 1
	repo.pages["cluster.list"] = testPage("cluster.list")
	service := NewService(repo)

	if _, err := service.Rollback(context.Background(), "cluster.list", 0, trustedWith(updatePermission())); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
	if _, err := service.Rollback(context.Background(), "cluster.list", 99, trustedWith(updatePermission())); err != ErrRevisionNotFound {
		t.Fatalf("err = %v", err)
	}
}
