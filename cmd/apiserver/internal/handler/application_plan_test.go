package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/F31/hnb/pkg/iam"
)

func planRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/application-plans:preview", bytes.NewBufferString(body))
	return request.WithContext(iam.WithTrustedContext(request.Context(), iam.TrustedContext{TenantID: "tenant-a", SubjectID: "subject-a"}))
}

func TestApplicationPlanPreviewIsTenantScopedAndOrdered(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM namespaces")).
		WithArgs("tenant-a", "app").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(quota")).
		WithArgs("tenant-a", "app").
		WillReturnRows(sqlmock.NewRows([]string{"quota"}).AddRow([]byte(`{"cpu":4,"memory":2048,"storage":2048}`)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT snapshot_json FROM capability_snapshots")).
		WithArgs("tenant-a", "target-a", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
		WillReturnRows(sqlmock.NewRows([]string{"snapshot_json"}).AddRow([]byte(`{}`)))
	body := `{"schemaVersion":"application-composition/v1","kind":"ApplicationComposition","namespace":"app","targetId":"target-a","releaseId":"release-1","releaseVersion":"1.0.0","snapshotDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","resources":{"cpu":2,"memory":1024,"storage":1024},"components":[{"name":"api","kind":"Deployment","image":"registry.example/app@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","dependsOn":["db"]},{"name":"db","kind":"StatefulSet","image":"registry.example/db@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}]}`
	recorder := httptest.NewRecorder()
	NewApplicationPlanHandler(db).Preview(recorder, planRequest(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Steps []struct {
				Name string `json:"name"`
			} `json:"steps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Steps) != 2 || payload.Data.Steps[0].Name != "db" || payload.Data.Steps[1].Name != "api" {
		t.Fatalf("unexpected steps: %+v", payload.Data.Steps)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationPlanRejectsUnknownManifestField(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	body := `{"schemaVersion":"application-composition/v1","kind":"ApplicationComposition","namespace":"app","targetId":"target-a","releaseId":"r","releaseVersion":"1","snapshotDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","components":[{"name":"api","kind":"Deployment","image":"registry.example/app@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"manifest":"apiVersion: v1"}`
	recorder := httptest.NewRecorder()
	NewApplicationPlanHandler(db).Preview(recorder, planRequest(body))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown manifest field status = %d", recorder.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
