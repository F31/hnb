package audit

import (
	"database/sql"
	"fmt"
	"time"
)

type Event struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username,omitempty"`
	TenantID     string    `json:"tenant_id,omitempty"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	ResourceName string    `json:"resource_name,omitempty"`
	StatusCode   int       `json:"status_code"`
	Method       string    `json:"method,omitempty"`
	Path         string    `json:"path,omitempty"`
	RemoteAddr   string    `json:"remote_addr,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
	Error        string    `json:"error,omitempty"`
	Duration     int64     `json:"duration_ms"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(event *Event) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (
			id, timestamp, user_id, username, tenant_id, workspace_id,
			action, resource_type, resource_id, resource_name,
			status_code, method, path, remote_addr, user_agent,
			request_body, response_body, error, duration_ms
		) VALUES (
			$1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''),
			$7, $8, NULLIF($9,''), NULLIF($10,''),
			$11, NULLIF($12,''), NULLIF($13,''), NULLIF($14,''), NULLIF($15,''),
			NULLIF($16,''), NULLIF($17,''), NULLIF($18,''), $19
		)`,
		event.ID, event.Timestamp, event.UserID, event.Username,
		event.TenantID, event.WorkspaceID,
		event.Action, event.ResourceType, event.ResourceID, event.ResourceName,
		event.StatusCode, event.Method, event.Path, event.RemoteAddr, event.UserAgent,
		event.RequestBody, event.ResponseBody, event.Error, event.Duration)
	return err
}

func (s *Store) List(limit, offset int, filters map[string]string) ([]Event, error) {
	query := `SELECT id, timestamp, user_id, username, tenant_id, workspace_id,
		action, resource_type, resource_id, resource_name,
		status_code, method, path, remote_addr, user_agent, duration_ms, error
		FROM audit_logs WHERE 1=1`
	args := []any{}
	argIdx := 1

	if userID, ok := filters["user_id"]; ok {
		query += fmt.Sprintf(" AND user_id=$%d", argIdx)
		args = append(args, userID)
		argIdx++
	}
	if action, ok := filters["action"]; ok {
		query += fmt.Sprintf(" AND action=$%d", argIdx)
		args = append(args, action)
		argIdx++
	}
	if resourceType, ok := filters["resource_type"]; ok {
		query += fmt.Sprintf(" AND resource_type=$%d", argIdx)
		args = append(args, resourceType)
		argIdx++
	}
	if tenantID, ok := filters["tenant_id"]; ok {
		query += fmt.Sprintf(" AND tenant_id=$%d", argIdx)
		args = append(args, tenantID)
		argIdx++
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, limit)
		argIdx++
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, offset)
		argIdx++
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var username, tenantID, workspaceID, resourceID, resourceName sql.NullString
		var method, path, remoteAddr, userAgent, errorStr sql.NullString

		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &username,
			&tenantID, &workspaceID,
			&e.Action, &e.ResourceType, &resourceID, &resourceName,
			&e.StatusCode, &method, &path, &remoteAddr, &userAgent,
			&e.Duration, &errorStr); err != nil {
			continue
		}
		if username.Valid {
			e.Username = username.String
		}
		if tenantID.Valid {
			e.TenantID = tenantID.String
		}
		if workspaceID.Valid {
			e.WorkspaceID = workspaceID.String
		}
		if resourceID.Valid {
			e.ResourceID = resourceID.String
		}
		if resourceName.Valid {
			e.ResourceName = resourceName.String
		}
		if method.Valid {
			e.Method = method.String
		}
		if path.Valid {
			e.Path = path.String
		}
		if remoteAddr.Valid {
			e.RemoteAddr = remoteAddr.String
		}
		if userAgent.Valid {
			e.UserAgent = userAgent.String
		}
		if errorStr.Valid {
			e.Error = errorStr.String
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *Store) Get(id string, tenantIDs ...string) (*Event, error) {
	var e Event
	var username, tenantID, workspaceID, resourceID, resourceName sql.NullString
	var method, path, remoteAddr, userAgent, requestBody, responseBody, errorStr sql.NullString

	query := `
		SELECT id, timestamp, user_id, username, tenant_id, workspace_id,
			action, resource_type, resource_id, resource_name,
			status_code, method, path, remote_addr, user_agent,
			request_body, response_body, error, duration_ms
		FROM audit_logs WHERE id = $1`
	args := []any{id}
	if len(tenantIDs) > 0 {
		query += ` AND tenant_id = $2`
		args = append(args, tenantIDs[0])
	}
	err := s.db.QueryRow(query, args...).
		Scan(&e.ID, &e.Timestamp, &e.UserID, &username,
			&tenantID, &workspaceID,
			&e.Action, &e.ResourceType, &resourceID, &resourceName,
			&e.StatusCode, &method, &path, &remoteAddr, &userAgent,
			&requestBody, &responseBody, &errorStr, &e.Duration)
	if err != nil {
		return nil, err
	}
	if username.Valid {
		e.Username = username.String
	}
	if tenantID.Valid {
		e.TenantID = tenantID.String
	}
	if workspaceID.Valid {
		e.WorkspaceID = workspaceID.String
	}
	if resourceID.Valid {
		e.ResourceID = resourceID.String
	}
	if resourceName.Valid {
		e.ResourceName = resourceName.String
	}
	if method.Valid {
		e.Method = method.String
	}
	if path.Valid {
		e.Path = path.String
	}
	if remoteAddr.Valid {
		e.RemoteAddr = remoteAddr.String
	}
	if userAgent.Valid {
		e.UserAgent = userAgent.String
	}
	if requestBody.Valid {
		e.RequestBody = requestBody.String
	}
	if responseBody.Valid {
		e.ResponseBody = responseBody.String
	}
	if errorStr.Valid {
		e.Error = errorStr.String
	}
	return &e, nil
}

func (s *Store) Migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
			user_id TEXT NOT NULL,
			username TEXT,
			tenant_id TEXT,
			workspace_id TEXT,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT,
			resource_name TEXT,
			status_code INTEGER NOT NULL DEFAULT 0,
			method TEXT,
			path TEXT,
			remote_addr TEXT,
			user_agent TEXT,
			request_body TEXT,
			response_body TEXT,
			error TEXT,
			duration_ms BIGINT NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id);
		CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
		CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type, resource_id);
	`)
	return err
}
