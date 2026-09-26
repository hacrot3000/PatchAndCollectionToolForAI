package identity

import "time"

// ID is an opaque stable identifier. Callers must not derive authorization
// decisions from workspace paths, ports, usernames or other mutable metadata.
type ID string

type User struct {
	ID                ID
	Username          string
	DisplayName       string
	PasswordHash      string
	Enabled           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastLoginAt       *time.Time
	PasswordChangedAt *time.Time
}

type Project struct {
	ID          ID
	Key         string
	DisplayName string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Role struct {
	ID          ID
	Name        string
	Description string
	SystemRole  bool
}

type Permission struct {
	ID          ID
	Key         string
	Description string
}

type ProjectMember struct {
	ProjectID ID
	UserID    ID
	RoleID    ID
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PermissionEffect string

const (
	PermissionAllow PermissionEffect = "ALLOW"
	PermissionDeny  PermissionEffect = "DENY"
)

type MemberPermission struct {
	ProjectID    ID
	UserID       ID
	PermissionID ID
	Effect       PermissionEffect
}

type AuthSession struct {
	ID             ID
	UserID         ID
	TokenHash      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastSeenAt     time.Time
	RevokedAt      *time.Time
	ClientMetadata string
}

type AuditEvent struct {
	ID           ID
	Timestamp    time.Time
	UserID       *ID
	ProjectID    *ID
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	ClientIP     string
	Details      string
}

// Principal is the request-scoped identity/authorization projection consumed by
// HTTP handlers. Permissions are project-scoped and already resolved.
type Principal struct {
	UserID      ID
	Username    string
	ProjectID   ID
	ProjectKey  string
	Permissions map[string]bool
}

func (p Principal) Allowed(permission string) bool {
	return KnownPermission(permission) && p.Permissions != nil && p.Permissions[permission]
}
