package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
)

type HealthHandler struct {
	startTime time.Time
}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{startTime: time.Now()}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]any{
		"status":    "ok",
		"uptime":    time.Since(h.startTime).String(),
		"startedAt": h.startTime.Format(time.RFC3339),
	})
}

func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]string{"status": "ready"})
}

type OpenAPISpec struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Title       string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
	} `json:"info"`
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths      map[string]any `json:"paths"`
	Components struct {
		SecuritySchemes map[string]any `json:"securitySchemes"`
		Schemas         map[string]any `json:"schemas"`
	} `json:"components"`
}

func (h *HealthHandler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	spec := generateOpenAPI()
	data, _ := json.MarshalIndent(spec, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func generateOpenAPI() *OpenAPISpec {
	spec := &OpenAPISpec{}
	spec.OpenAPI = "3.0.0"
	spec.Info.Title = "HNB Platform API"
	spec.Info.Version = "1.0.0"
	spec.Info.Description = "HNB Cloud Platform - Multi-cloud container management API"
	spec.Servers = []struct {
		URL string `json:"url"`
	}{{URL: "/"}}

	spec.Components.SecuritySchemes = map[string]any{
		"bearerAuth": map[string]any{
			"type":         "http",
			"scheme":       "bearer",
			"bearerFormat": "HMAC-SHA256",
		},
	}

	spec.Components.Schemas = map[string]any{
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{"type": "string"},
			},
		},
		"Workspace": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"name":         map[string]any{"type": "string"},
				"display_name": map[string]any{"type": "string"},
				"tenant_id":    map[string]any{"type": "string"},
				"is_active":    map[string]any{"type": "boolean"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"Cluster": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":          map[string]any{"type": "string"},
				"name":        map[string]any{"type": "string"},
				"target_type": map[string]any{"type": "string", "enum": []string{"kubernetes", "container_engine", "edge_runtime", "external_service"}},
				"status":      map[string]any{"type": "string", "enum": []string{"online", "offline", "unknown", "degraded"}},
				"is_active":   map[string]any{"type": "boolean"},
			},
		},
		"Extension": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "string"},
				"name":     map[string]any{"type": "string"},
				"version":  map[string]any{"type": "string"},
				"phase":    map[string]any{"type": "string", "enum": []string{"pending", "installing", "ready", "degraded", "uninstalling"}},
				"manifest": map[string]any{"type": "object"},
			},
		},
		"LoginRequest": map[string]any{
			"type":     "object",
			"required": []string{"username", "password"},
			"properties": map[string]any{
				"username": map[string]any{"type": "string"},
				"password": map[string]any{"type": "string", "format": "password"},
			},
		},
		"LoginResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"access_token":  map[string]any{"type": "string"},
				"refresh_token": map[string]any{"type": "string"},
				"expires_in":    map[string]any{"type": "integer"},
				"user_id":       map[string]any{"type": "string"},
				"username":      map[string]any{"type": "string"},
			},
		},
		"User": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"username":     map[string]any{"type": "string"},
				"email":        map[string]any{"type": "string"},
				"phone":        map[string]any{"type": "string"},
				"display_name": map[string]any{"type": "string"},
				"source":       map[string]any{"type": "string"},
				"is_active":    map[string]any{"type": "boolean"},
			},
		},
		"Role": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"name":         map[string]any{"type": "string"},
				"display_name": map[string]any{"type": "string"},
				"scope":        map[string]any{"type": "string", "enum": []string{"global", "workspace", "cluster", "project"}},
				"built_in":     map[string]any{"type": "boolean"},
			},
		},
		"RoleBinding": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "string"},
				"role_id":  map[string]any{"type": "string"},
				"user_id":  map[string]any{"type": "string"},
				"scope":    map[string]any{"type": "string"},
				"scope_id": map[string]any{"type": "string"},
			},
		},
		"Agent": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id":     map[string]any{"type": "string"},
				"hostname":       map[string]any{"type": "string"},
				"status":         map[string]any{"type": "string"},
				"connected_at":   map[string]any{"type": "string", "format": "date-time"},
				"last_heartbeat": map[string]any{"type": "string", "format": "date-time"},
			},
		},
	}

	spec.Paths = map[string]any{
		// Health
		"/health": map[string]any{
			"get": map[string]any{
				"summary": "Health check",
				"tags":    []string{"System"},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Service is healthy",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"status": map[string]any{"type": "string"},
										"uptime": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				},
			},
		},
		"/ready": map[string]any{
			"get": map[string]any{
				"summary": "Readiness check",
				"tags":    []string{"System"},
				"responses": map[string]any{
					"200": map[string]any{"description": "Service is ready"},
				},
			},
		},
		"/openapi.json": map[string]any{
			"get": map[string]any{
				"summary": "OpenAPI specification",
				"tags":    []string{"System"},
				"responses": map[string]any{
					"200": map[string]any{"description": "OpenAPI 3.0 spec"},
				},
			},
		},
		"/api/v1/schema/page/{id}": map[string]any{
			"get": map[string]any{
				"summary":  "Get Console schema page",
				"tags":     []string{"Console"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Versioned declarative schema page"},
					"401": map[string]any{"description": "Unauthenticated"},
					"403": map[string]any{"description": "Forbidden"},
					"404": map[string]any{"description": "Schema page not found"},
				},
			},
		},

		// Auth
		"/api/v1/auth/login": map[string]any{
			"post": map[string]any{
				"summary": "User login",
				"tags":    []string{"Authentication"},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/LoginRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Login successful",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/LoginResponse"},
							},
						},
					},
					"401": map[string]any{"description": "Invalid credentials"},
				},
			},
		},
		"/api/v1/auth/refresh": map[string]any{
			"post": map[string]any{
				"summary": "Refresh access token",
				"tags":    []string{"Authentication"},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"refresh_token": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Token refreshed"},
					"401": map[string]any{"description": "Invalid refresh token"},
				},
			},
		},
		"/api/v1/auth/logout": map[string]any{
			"post": map[string]any{
				"summary":  "Logout (revoke token)",
				"tags":     []string{"Authentication"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Logged out"},
				},
			},
		},

		// Users
		"/api/v1/users": map[string]any{
			"get": map[string]any{
				"summary":  "List users",
				"tags":     []string{"Users"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "User list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/User"},
								},
							},
						},
					},
				},
			},
			"post": map[string]any{
				"summary":  "Create user",
				"tags":     []string{"Users"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"username":     map[string]any{"type": "string"},
									"password":     map[string]any{"type": "string", "format": "password"},
									"email":        map[string]any{"type": "string"},
									"phone":        map[string]any{"type": "string"},
									"display_name": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{"description": "User created"},
				},
			},
		},

		// Roles
		"/api/v1/roles": map[string]any{
			"get": map[string]any{
				"summary":  "List roles",
				"tags":     []string{"RBAC"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Role list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Role"},
								},
							},
						},
					},
				},
			},
		},
		"/api/v1/role-bindings": map[string]any{
			"post": map[string]any{
				"summary":  "Bind role to user",
				"tags":     []string{"RBAC"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/RoleBinding"},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{"description": "Role bound"},
				},
			},
		},
		"/api/v1/role-bindings/{user_id}/{scope}/{scope_id}": map[string]any{
			"delete": map[string]any{
				"summary":  "Unbind role from user",
				"tags":     []string{"RBAC"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "user_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "scope", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "scope_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Role unbound"},
				},
			},
		},
		"/api/v1/check-permission": map[string]any{
			"get": map[string]any{
				"summary":  "Check user permission",
				"tags":     []string{"RBAC"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "verb", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "resource", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "scope", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "scope_id", "in": "query", "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Permission check result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"allowed": map[string]any{"type": "boolean"},
									},
								},
							},
						},
					},
				},
			},
		},

		// Workspaces
		"/api/v1/workspaces": map[string]any{
			"get": map[string]any{
				"summary":  "List workspaces",
				"tags":     []string{"Workspaces"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Workspace list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Workspace"},
								},
							},
						},
					},
				},
			},
			"post": map[string]any{
				"summary":  "Create workspace",
				"tags":     []string{"Workspaces"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":         map[string]any{"type": "string"},
									"display_name": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Workspace created",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Workspace"},
							},
						},
					},
				},
			},
		},
		"/api/v1/workspaces/{workspace_id}/projects": map[string]any{
			"get": map[string]any{
				"summary":  "List projects in workspace",
				"tags":     []string{"Workspaces"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "workspace_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Project list"},
				},
			},
		},
		"/api/v1/projects/{project_id}/environments": map[string]any{
			"get": map[string]any{
				"summary":  "List environments in project",
				"tags":     []string{"Workspaces"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "project_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Environment list"},
				},
			},
		},

		// Clusters
		"/api/v1/clusters": map[string]any{
			"get": map[string]any{
				"summary":  "List clusters",
				"tags":     []string{"Clusters"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Cluster list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Cluster"},
								},
							},
						},
					},
				},
			},
			"post": map[string]any{
				"summary":  "Register cluster",
				"tags":     []string{"Clusters"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":            map[string]any{"type": "string"},
									"target_type":     map[string]any{"type": "string"},
									"connection_type": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Cluster registered",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Cluster"},
							},
						},
					},
				},
			},
		},
		"/api/v1/clusters/{id}": map[string]any{
			"get": map[string]any{
				"summary":  "Get cluster details",
				"tags":     []string{"Clusters"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Cluster details",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Cluster"},
							},
						},
					},
					"404": map[string]any{"description": "Cluster not found"},
				},
			},
			"delete": map[string]any{
				"summary":  "Delete cluster",
				"tags":     []string{"Clusters"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Cluster deleted"},
				},
			},
		},

		// Extensions
		"/api/v1/extensions": map[string]any{
			"get": map[string]any{
				"summary":  "List extensions",
				"tags":     []string{"Extensions"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Extension list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Extension"},
								},
							},
						},
					},
				},
			},
			"post": map[string]any{
				"summary":  "Install extension",
				"tags":     []string{"Extensions"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name":     map[string]any{"type": "string"},
									"version":  map[string]any{"type": "string"},
									"manifest": map[string]any{"type": "object"},
								},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{
						"description": "Extension installed",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Extension"},
							},
						},
					},
				},
			},
		},
		"/api/v1/extensions/{id}": map[string]any{
			"delete": map[string]any{
				"summary":  "Uninstall extension",
				"tags":     []string{"Extensions"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Extension uninstalled"},
				},
			},
		},

		// Proxy
		"/api/v1/proxy/{cluster_id}/{path}": map[string]any{
			"get": map[string]any{
				"summary":  "Proxy GET request to downstream cluster",
				"tags":     []string{"Proxy"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "cluster_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "path", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Proxy response"},
					"502": map[string]any{"description": "Agent not connected"},
				},
			},
			"post": map[string]any{
				"summary":  "Proxy POST request to downstream cluster",
				"tags":     []string{"Proxy"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "cluster_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "path", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Proxy response"},
					"502": map[string]any{"description": "Agent not connected"},
				},
			},
		},

		// Agents
		"/api/v1/agents": map[string]any{
			"get": map[string]any{
				"summary":  "List connected agents",
				"tags":     []string{"Agents"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Agent list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/Agent"},
								},
							},
						},
					},
				},
			},
		},
		"/api/v1/agents/{cluster_id}": map[string]any{
			"get": map[string]any{
				"summary":  "Get agent details",
				"tags":     []string{"Agents"},
				"security": []map[string]any{{"bearerAuth": []string{}}},
				"parameters": []map[string]any{
					{"name": "cluster_id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Agent details",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/Agent"},
							},
						},
					},
					"404": map[string]any{"description": "Agent not connected"},
				},
			},
		},

		// Tunnel WebSocket
		"/tunnel": map[string]any{
			"get": map[string]any{
				"summary": "Agent tunnel WebSocket endpoint",
				"tags":    []string{"Tunnel"},
				"parameters": []map[string]any{
					{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"101": map[string]any{"description": "WebSocket upgrade"},
					"401": map[string]any{"description": "Authentication failed"},
				},
			},
		},
	}

	return spec
}
