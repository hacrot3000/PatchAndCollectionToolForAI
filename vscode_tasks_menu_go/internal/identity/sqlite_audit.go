package identity

import (
	"context"
	"fmt"
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
