package schema

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

var (
	ErrNotFound          = errors.New("schema page not found")
	ErrRevisionNotFound  = errors.New("schema page revision not found")
	ErrForbidden         = errors.New("schema page forbidden")
	ErrInvalid           = errors.New("schema page invalid")
	pageIDPattern        = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
)

const (
	apiVersion = "ui.hnb.io/v1"
	kind       = "PageSchema"
)

type EndpointDefinition struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Method string `json:"method,omitempty"`
}

type DataSourceDefinition struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	EndpointID      string            `json:"endpointId"`
	QueryBindings   []string          `json:"queryBindings,omitempty"`
	ResponseMapping map[string]string `json:"responseMapping,omitempty"`
}

type Condition struct {
	All []ConditionTerm `json:"all,omitempty"`
	Any []ConditionTerm `json:"any,omitempty"`
}

// ConditionTerm 仅允许 V2.6 §4.3 保留的类型（permission/feature/license/capability/context）。
type ConditionTerm struct {
	Permission string `json:"permission,omitempty"`
	Capability string `json:"capability,omitempty"`
	Feature    string `json:"feature,omitempty"`
	License    string `json:"license,omitempty"`
	Context    string `json:"context,omitempty"`
}

type Action struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	LabelKey    string     `json:"labelKey,omitempty"`
	Permission  string     `json:"permission,omitempty"`
	EnabledWhen *Condition `json:"enabledWhen,omitempty"`
	Request     *struct {
		Method      string   `json:"method"`
		EndpointID  string   `json:"endpointId"`
		PathParams  []string `json:"pathParams,omitempty"`
		QueryParams []string `json:"queryParams,omitempty"`
	} `json:"request,omitempty"`
	Route *struct {
		Name   string            `json:"name"`
		Params map[string]string `json:"params,omitempty"`
	} `json:"route,omitempty"`
}

type Region struct {
	ID            string         `json:"id"`
	ComponentType string         `json:"componentType"`
	Span          int            `json:"span,omitempty"`
	Props         map[string]any `json:"props,omitempty"`
	Condition     *Condition     `json:"condition,omitempty"`
}

type Page struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		ID              string            `json:"id"`
		Revision        int               `json:"revision"`
		Etag            string            `json:"etag,omitempty"`
		GeneratedAt     string            `json:"generatedAt,omitempty"`
		MinShellVersion string            `json:"minShellVersion,omitempty"`
		PluginID        string            `json:"pluginId,omitempty"`
		Texts           map[string]string `json:"texts,omitempty"`
	} `json:"metadata"`
	Spec struct {
		Template       string                 `json:"template"`
		TitleKey       string                 `json:"titleKey,omitempty"`
		DescriptionKey string                 `json:"descriptionKey,omitempty"`
		Layout         map[string]any         `json:"layout,omitempty"`
		Endpoints      []EndpointDefinition   `json:"endpoints,omitempty"`
		DataSources    []DataSourceDefinition `json:"dataSources,omitempty"`
		Actions        []Action               `json:"actions,omitempty"`
		Regions        []Region               `json:"regions"`
	} `json:"spec"`
}

// Repository 访问数据库化 UI Registry（migration 079）。
// PageSchema 只保存声明式元数据与受信标识符，不包含任何可执行代码（V2.6 §2.2）。
type Repository interface {
	GetPage(ctx context.Context, id string) (Page, bool)
	ActiveRevision(ctx context.Context, id string) (int, bool)
	Publish(ctx context.Context, page Page, actorID, tenantID string) (Page, error)
	Rollback(ctx context.Context, id string, targetRevision int, actorID, tenantID string) (Page, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Get(ctx context.Context, id string, trusted iam.TrustedContext) (Page, error) {
	if !pageIDPattern.MatchString(id) {
		return Page{}, ErrNotFound
	}
	page, ok := s.repo.GetPage(ctx, id)
	if !ok {
		return Page{}, ErrNotFound
	}
	if !hasPermission(trusted, string(iam.ResourceSchema), iam.ActionRead) {
		return Page{}, ErrForbidden
	}
	decorate(&page)
	return page, nil
}

// ActiveRevision 返回页面当前 revision（用于 ETag 条件请求），
// 页面不存在时返回 false。
func (s *Service) ActiveRevision(ctx context.Context, id string) (int, bool) {
	if !pageIDPattern.MatchString(id) {
		return 0, false
	}
	return s.repo.ActiveRevision(ctx, id)
}

// Publish 将 PageSchema 作为新 revision 发布：递增 revision、写入不可变历史、
// 同一事务内写入 Outbox 事件（hnb.event.ui.page-published.v1）。
func (s *Service) Publish(ctx context.Context, page Page, trusted iam.TrustedContext) (Page, error) {
	id := page.Metadata.ID
	if !pageIDPattern.MatchString(id) {
		return Page{}, ErrInvalid
	}
	if page.APIVersion != apiVersion || page.Kind != kind {
		return Page{}, ErrInvalid
	}
	if len(page.Spec.Regions) == 0 {
		return Page{}, ErrInvalid
	}
	if !hasPermission(trusted, string(iam.ResourceSchema), iam.ActionUpdate) {
		return Page{}, ErrForbidden
	}
	published, err := s.repo.Publish(ctx, page, trusted.SubjectID, trusted.TenantID)
	if err != nil {
		return Page{}, err
	}
	decorate(&published)
	return published, nil
}

// Rollback 将页面切换回历史 revision（V2.6 §20.4）：只切换 active_revision，
// 不覆盖历史记录；同一事务内写入 Outbox 事件（hnb.event.ui.page-rolled-back.v1）。
func (s *Service) Rollback(ctx context.Context, id string, targetRevision int, trusted iam.TrustedContext) (Page, error) {
	if !pageIDPattern.MatchString(id) {
		return Page{}, ErrNotFound
	}
	if targetRevision < 1 {
		return Page{}, ErrInvalid
	}
	if !hasPermission(trusted, string(iam.ResourceSchema), iam.ActionUpdate) {
		return Page{}, ErrForbidden
	}
	page, err := s.repo.Rollback(ctx, id, targetRevision, trusted.SubjectID, trusted.TenantID)
	if err != nil {
		return Page{}, err
	}
	decorate(&page)
	return page, nil
}

// decorate 补全读取侧派生字段：generatedAt（缺失时）与 etag。
func decorate(page *Page) {
	if page.Metadata.GeneratedAt == "" {
		page.Metadata.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if page.Metadata.Revision >= 1 {
		page.Metadata.Etag = fmt.Sprintf("page-%s-r%d", page.Metadata.ID, page.Metadata.Revision)
	}
}

func hasPermission(trusted iam.TrustedContext, resource string, action iam.AuthorizationAction) bool {
	for _, permission := range trusted.ScopedPermissions {
		if permission.TenantID == trusted.TenantID && permission.Action == action && (permission.ResourceKind == resource || permission.ResourceKind == "*") {
			return true
		}
	}
	return false
}

// StaticRepository 保留为测试/兜底用内存仓库（单页面 fixture）。
type StaticRepository struct{ pages map[string]Page }

func NewStaticRepository() *StaticRepository {
	page := Page{APIVersion: apiVersion, Kind: kind}
	page.Metadata.ID = "cluster-list"
	page.Metadata.Revision = 1
	page.Metadata.Texts = map[string]string{"cluster.title": "Clusters", "cluster.description": "Trusted schema runtime fixture", "cluster.refresh": "Refresh"}
	page.Spec.Template = "list"
	page.Spec.TitleKey = "cluster.title"
	page.Spec.DescriptionKey = "cluster.description"
	page.Spec.Layout = map[string]any{"type": "grid", "columns": 12, "gap": "md"}
	page.Spec.Endpoints = []EndpointDefinition{{ID: "clusters.list", Path: "/api/v1/clusters", Method: "GET"}}
	page.Spec.DataSources = []DataSourceDefinition{{ID: "clusters", Type: "paginatedQuery", EndpointID: "clusters.list", QueryBindings: []string{"status", "provider"}, ResponseMapping: map[string]string{"items": "data.items", "total": "data.total"}}}
	page.Spec.Actions = []Action{{ID: "refresh", Type: "api", LabelKey: "cluster.refresh", Permission: "schema:read", Request: &struct {
		Method      string   `json:"method"`
		EndpointID  string   `json:"endpointId"`
		PathParams  []string `json:"pathParams,omitempty"`
		QueryParams []string `json:"queryParams,omitempty"`
	}{Method: "GET", EndpointID: "clusters.list"}}}
	page.Spec.Regions = []Region{{ID: "cluster-table", ComponentType: "DataTable", Span: 12, Props: map[string]any{"dataSource": "clusters", "actions": []string{"refresh"}, "columns": []map[string]string{{"key": "id", "title": "ID"}, {"key": "name", "title": "Name"}}}, Condition: &Condition{All: []ConditionTerm{{Permission: "schema:read"}}}}}
	return &StaticRepository{pages: map[string]Page{page.Metadata.ID: page}}
}

func (r *StaticRepository) GetPage(_ context.Context, id string) (Page, bool) {
	page, ok := r.pages[id]
	if !ok {
		return Page{}, false
	}
	if page.Metadata.ID != id {
		return Page{}, false
	}
	if len(page.Spec.Regions) == 0 {
		panic(fmt.Sprintf("static schema page %q has no regions", id))
	}
	return page, true
}

func (r *StaticRepository) ActiveRevision(_ context.Context, id string) (int, bool) {
	page, ok := r.pages[id]
	if !ok {
		return 0, false
	}
	return page.Metadata.Revision, true
}

func (r *StaticRepository) Publish(_ context.Context, _ Page, _, _ string) (Page, error) {
	return Page{}, errors.New("static repository does not support publish")
}

func (r *StaticRepository) Rollback(_ context.Context, _ string, _ int, _, _ string) (Page, error) {
	return Page{}, errors.New("static repository does not support rollback")
}
