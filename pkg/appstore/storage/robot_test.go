package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateRobot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2.0/robots" || r.Method != "POST" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body CreateRobotRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Name != "upload-test1234" {
			t.Errorf("expected name 'upload-test1234', got %s", body.Name)
		}
		if body.Level != "project" {
			t.Errorf("expected level 'project', got %s", body.Level)
		}
		if len(body.Permissions) != 1 {
			t.Fatalf("expected 1 permission, got %d", len(body.Permissions))
		}
		if body.Permissions[0].Namespace != "hnb" {
			t.Errorf("expected namespace 'hnb', got %s", body.Permissions[0].Namespace)
		}
		if len(body.Permissions[0].Access) != 1 {
			t.Fatalf("expected 1 access entry, got %d", len(body.Permissions[0].Access))
		}
		if body.Permissions[0].Access[0].Resource != "repository" {
			t.Fatalf("expected repository resource, got %+v", body.Permissions[0].Access)
		}
		if body.Permissions[0].Access[0].Action != "push" {
			t.Fatalf("expected push-only access, got %+v", body.Permissions[0].Access)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(RobotResponse{
			ID:    42,
			Name:  "robot$upload-test1234",
			Token: "test-token-string",
		})
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL, Username: "admin", Password: "pass"})
	robot, err := client.CreateRobot(context.Background(), "upload-test1234", "hnb", 3600)
	if err != nil {
		t.Fatalf("CreateRobot failed: %v", err)
	}
	if robot.ID != 42 {
		t.Errorf("expected robot ID 42, got %d", robot.ID)
	}
	if robot.Token != "test-token-string" {
		t.Errorf("expected token 'test-token-string', got %s", robot.Token)
	}
	if robot.Name != "robot$upload-test1234" {
		t.Errorf("expected name 'robot$upload-test1234', got %s", robot.Name)
	}
}

func TestCreateRobot_QuotaExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL})
	_, err := client.CreateRobot(context.Background(), "upload-test", "hnb", 3600)
	if err == nil {
		t.Fatal("expected error for quota exceeded")
	}
}

func TestCreateRobot_PermissionDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL})
	_, err := client.CreateRobot(context.Background(), "upload-test", "hnb", 3600)
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
}

func TestCreateRobot_HarborUnavailable(t *testing.T) {
	client := NewRobotClient(StorageConfig{RegistryURL: "http://localhost:1"})
	_, err := client.CreateRobot(context.Background(), "upload-test", "hnb", 3600)
	if err == nil {
		t.Fatal("expected error for unreachable harbor")
	}
}

func TestDeleteRobot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2.0/robots/42" || r.Method != "DELETE" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL})
	if err := client.DeleteRobot(context.Background(), 42); err != nil {
		t.Fatalf("DeleteRobot failed: %v", err)
	}
}

func TestDeleteRobot_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL})
	if err := client.DeleteRobot(context.Background(), 999); err != nil {
		t.Fatalf("DeleteRobot should not return error for 404, got: %v", err)
	}
}

func TestDeleteRobot_ReportsCleanupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewRobotClient(StorageConfig{RegistryURL: server.URL})
	if err := client.DeleteRobot(context.Background(), 42); err == nil {
		t.Fatal("expected cleanup failure error")
	}
}
