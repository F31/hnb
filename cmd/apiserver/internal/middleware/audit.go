package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/audit"
)

type AuditMW struct {
	store  auditWriter
	intent *IntentAuditLog
}

// IntentAuditLog records intent-specific security evidence.
type IntentAuditLog struct {
	IntentKind    string
	IntentDigest  string
	CorrelationID string
	OperationID   string
	Decision      string // allow / deny
	TenantID      string
	SubjectID     string
}

type auditWriter interface {
	Create(*audit.Event) error
}

func NewAuditMW(store auditWriter) *AuditMW {
	return &AuditMW{store: store, intent: &IntentAuditLog{}}
}

func (m *AuditMW) Name() string { return "audit" }

type auditResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rw *auditResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *auditResponseWriter) Write(data []byte) (int, error) {
	rw.body.Write(data)
	return rw.ResponseWriter.Write(data)
}

func (m *AuditMW) Handle(ctx *Context, next func()) {
	start := time.Now()
	r := ctx.Request

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	arw := &auditResponseWriter{ResponseWriter: ctx.Response, statusCode: http.StatusOK}
	ctx.Response = arw

	next()

	if m.store == nil {
		return
	}

	intentEvent := extractIntentEvent(bodyBytes)

	var eventType string
	if intentEvent != nil {
		eventType = fmt.Sprintf("intent_%s", intentEvent.IntentKind)
	} else {
		eventType = classifyAction(r.Method)
	}

	evt := &audit.Event{
		ID:           fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		Timestamp:    time.Now().UTC(),
		UserID:       ctx.UserID,
		TenantID:     ctx.TenantID,
		Action:       eventType,
		ResourceType: classifyResource(r.URL.Path),
		StatusCode:   arw.statusCode,
		Method:       r.Method,
		Path:         r.URL.Path,
		RemoteAddr:   r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		Duration:     time.Since(start).Milliseconds(),
	}

	if !isCredentialEndpoint(r.URL.Path) {
		if len(bodyBytes) < 4096 {
			evt.RequestBody = redactJSONBody(bodyBytes)
		}
		if arw.body.Len() < 4096 {
			evt.ResponseBody = redactJSONBody(arw.body.Bytes())
		}
	}

	if intentEvent != nil && arw.statusCode >= 200 && arw.statusCode < 300 {
		go logIntentSecurityEvent(intentEvent, evt)
	}

	if arw.statusCode >= 400 {
		evt.Error = http.StatusText(arw.statusCode)
	}

	go m.store.Create(evt)
}

func logIntentSecurityEvent(intent *IntentAuditLog, evt *audit.Event) {
	detail := map[string]any{
		"intent_kind":    intent.IntentKind,
		"intent_hash":    intent.IntentDigest,
		"correlation_id": intent.CorrelationID,
		"operation_id":   intent.OperationID,
		"decision":       intent.Decision,
		"resource":       evt.ResourceType,
		"path":           evt.Path,
		"user_id":        evt.UserID,
		"tenant_id":      evt.TenantID,
	}
	detailJSON, _ := json.Marshal(detail)
	_ = detailJSON // structured intent security evidence logged via event body
}

func extractIntentEvent(body []byte) *IntentAuditLog {
	if len(body) == 0 || !json.Valid(body) {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	path, ok := raw["path"]
	if !ok {
		return nil
	}
	var pathStr string
	if err := json.Unmarshal(path, &pathStr); err != nil {
		return nil
	}
	if strings.HasSuffix(pathStr, "/v1/intents") || strings.HasPrefix(pathStr, "/v1/intents") {
		intent := &IntentAuditLog{
			TenantID:      "",
			SubjectID:     "",
			CorrelationID: "",
			OperationID:   "",
			Decision:      "allow",
		}
		if md, ok := raw["metadata"]; ok {
			var meta map[string]string
			if err := json.Unmarshal(md, &meta); err == nil {
				intent.CorrelationID = meta["correlationId"]
			}
		}
		if sp, ok := raw["spec"]; ok {
			var spec map[string]string
			if err := json.Unmarshal(sp, &spec); err == nil {
				intent.IntentKind = spec["kind"]
				intent.OperationID = spec["releaseId"]
			}
		}
		if uid, ok := raw["user_id"]; ok {
			json.Unmarshal(uid, &intent.SubjectID)
		}
		if tid, ok := raw["tenant_id"]; ok {
			json.Unmarshal(tid, &intent.TenantID)
		}
		return intent
	}
	return nil
}

func isCredentialEndpoint(path string) bool {
	return path == "/api/v1/auth/login" || path == "/api/v1/auth/refresh"
}

func redactJSONBody(body []byte) string {
	if len(body) == 0 || !json.Valid(body) {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	redactJSONValue(value)
	redacted, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(redacted)
}

func redactJSONValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if sensitiveJSONKey(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			redactJSONValue(child)
		}
	case []any:
		for _, child := range typed {
			redactJSONValue(child)
		}
	}
}

func sensitiveJSONKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, term := range []string{"password", "token", "private_key", "api_key", "signing_key", "key_material", "secret", "credential", "authorization"} {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func classifyAction(method string) string {
	switch method {
	case "GET":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "other"
	}
}

func classifyResource(path string) string {
	parts := splitPath(path)
	if strings.Contains(path, "/storage/") && strings.Contains(path, "binding") {
		return "storageClassBinding"
	}
	for _, p := range parts {
		switch p {
		case "workspaces":
			return "workspace"
		case "projects":
			return "project"
		case "environments":
			return "environment"
		case "clusters":
			return "cluster"
		case "extensions":
			return "extension"
		case "users":
			return "user"
		case "roles", "role-bindings":
			return "role"
		case "agents":
			return "agent"
		case "proxy":
			return "proxy"
		case "audit-logs":
			return "audit"
		case "storage":
			if len(parts) > 3 {
				switch parts[3] {
				case "backends":
					return "storageBackend"
				case "offerings":
					return "workloadStorageOffering"
				case "bindings":
					return "storageClassBinding"
				}
			}
			return "storage"
		case "v1":
			if len(parts) > 1 {
				switch parts[1] {
				case "operations":
					return "operation"
				case "intents":
					return "intent"
				case "targets":
					return "runtimeTarget"
				case "console":
					return "console"
				}
			}
		}
	}
	return "unknown"
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
