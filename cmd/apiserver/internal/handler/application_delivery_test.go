package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/F31/hnb/pkg/iam"
)

func TestApplicationDeliveryListIsTenantScopedAndReturnsLatest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("WHERE tenant_id = $1")).
		WithArgs("tenant-a", "", "", "", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "target_id", "namespace", "release_id", "release_version", "revision", "resource_uid", "pvc_name", "ingress_name", "operation_id", "observation_generation", "observation_revision", "status", "created_at", "updated_at"}).
			AddRow("delivery-1", "tenant-a", "target-a", "app", "release-1", "1.0.0", 2, "uid-1", "data", "route", "op-1", 3, 7, "Ready", now, now))
	handler := NewApplicationDeliveryHandler(db)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/application-deliveries", nil)
	request = request.WithContext(iam.WithTrustedContext(request.Context(), iam.TrustedContext{
		SubjectID: "subject-a", SubjectType: "user", MembershipID: "membership-a", TenantID: "tenant-a",
		ScopedPermissions: []iam.ScopedPermission{{ResourceKind: string(iam.ResourceApplicationDelivery), Action: iam.ActionList, TenantID: "tenant-a"}},
	}))
	recorder := httptest.NewRecorder()
	handler.List(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Items []ApplicationDeliveryBinding `json:"items"`
			Total int                          `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Total != 1 || payload.Data.Items[0].TenantID != "tenant-a" || payload.Data.Items[0].Revision != 2 || payload.Data.Items[0].PVCName != "data" {
		t.Fatalf("unexpected delivery payload: %+v", payload)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationDeliveryRejectsInvalidTargetBeforeQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewApplicationDeliveryHandler(db)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/application-deliveries?targetId=not-a-uuid", nil)
	request = request.WithContext(iam.WithTrustedContext(request.Context(), iam.TrustedContext{TenantID: "tenant-a"}))
	recorder := httptest.NewRecorder()
	handler.List(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status = %d", recorder.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
