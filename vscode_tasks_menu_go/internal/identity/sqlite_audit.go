package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

var _ AuditStore = (*sqliteDatabase)(nil)

func (d *sqliteDatabase) AppendAudit(ctx context.Context, event AuditEvent) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("identity DB is not open")
	}
	if event.ID == "" || event.Timestamp.IsZero() || event.Action == "" || event.Result == "" {
		return fmt.Errorf("audit event requires id, timestamp, action and result")
	}

	var userID any
	if event.UserID != nil {
		if *event.UserID == "" {
			return fmt.Errorf("audit event user_id cannot be empty when present")
		}
		userID = string(*event.UserID)
	}
	var projectID any
	if event.ProjectID != nil {
		if *event.ProjectID == "" {
			return fmt.Errorf("audit event project_id cannot be empty when present")
		}
		projectID = string(*event.ProjectID)
	}

	_, err := d.db.ExecContext(ctx, `
INSERT INTO audit_log(
    id, timestamp, user_id, project_id, action,
    resource_type, resource_id, result, client_ip, details
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		string(event.ID),
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		userID,
		projectID,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		event.Result,
		event.ClientIP,
		event.Details,
	)
	if err != nil {
		return fmt.Errorf("append identity audit event: %w", err)
	}
	return nil
}

func (d *sqliteDatabase) ListAudit(ctx context.Context, query AuditQuery) ([]AuditEvent, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("identity DB is not open")
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var where []string
	var args []any
	if query.ProjectID != "" {
		where = append(where, "project_id = ?")
		args = append(args, string(query.ProjectID))
	}
	if query.UserID != "" {
		where = append(where, "user_id = ?")
		args = append(args, string(query.UserID))
	}
	if action := strings.TrimSpace(query.Action); action != "" {
		where = append(where, "action = ?")
		args = append(args, action)
	}
	if query.Before != nil && !query.Before.IsZero() {
		where = append(where, "timestamp < ?")
		args = append(args, query.Before.UTC().Format(time.RFC3339Nano))
	}
	statement := `
SELECT id, timestamp, user_id, project_id, action,
       resource_type, resource_id, result, client_ip, details
FROM audit_log`
	if len(where) > 0 {
		statement += " WHERE " + strings.Join(where, " AND ")
	}
	statement += " ORDER BY timestamp DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := d.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("query identity audit events: %w", err)
	}
	defer rows.Close()
	events := make([]AuditEvent, 0, limit)
	for rows.Next() {
		var event AuditEvent
		var id, timestamp string
		var userID, projectID sql.NullString
		if err := rows.Scan(
			&id, &timestamp, &userID, &projectID, &event.Action,
			&event.ResourceType, &event.ResourceID, &event.Result, &event.ClientIP, &event.Details,
		); err != nil {
			return nil, fmt.Errorf("scan identity audit event: %w", err)
		}
		event.ID = ID(id)
		if event.Timestamp, err = parseDBTime(timestamp); err != nil {
			return nil, fmt.Errorf("parse identity audit timestamp: %w", err)
		}
		if userID.Valid {
			value := ID(userID.String)
			event.UserID = &value
		}
		if projectID.Valid {
			value := ID(projectID.String)
			event.ProjectID = &value
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity audit events: %w", err)
	}
	return events, nil
}
