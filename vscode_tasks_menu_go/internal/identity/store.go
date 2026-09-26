package identity

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("identity record not found")
	ErrConflict = errors.New("identity record conflict")
)

// Reader is the minimum identity surface needed to resolve users, projects and
// effective project permissions for authenticated requests.
type Reader interface {
	UserByID(ctx context.Context, id ID) (User, error)
	UserByUsername(ctx context.Context, username string) (User, error)
	ProjectByKey(ctx context.Context, projectKey string) (Project, error)
	ProjectMember(ctx context.Context, projectID, userID ID) (ProjectMember, error)
	EffectivePermissions(ctx context.Context, projectID, userID ID) (map[string]bool, error)
}

// SessionStore persists only hashes of opaque browser session tokens.
type SessionStore interface {
	CreateAuthSession(ctx context.Context, session AuthSession) error
	CreateLoginSession(ctx context.Context, session AuthSession, projectID ID, verifiedPasswordHash string) error
	AuthSessionByTokenHash(ctx context.Context, tokenHash string) (AuthSession, error)
	TouchAuthSession(ctx context.Context, sessionID ID, seenAt time.Time) error
	RevokeAuthSession(ctx context.Context, sessionID ID, revokedAt time.Time) error
	RevokeUserSessions(ctx context.Context, userID ID, revokedAt time.Time) error
}

// AuditStore stores structured security-relevant events. Callers must never put
// plaintext passwords, raw session tokens or other secrets in AuditEvent.
type AuditStore interface {
	AppendAudit(ctx context.Context, event AuditEvent) error
	ListAudit(ctx context.Context, query AuditQuery) ([]AuditEvent, error)
}

// AdminStore is intentionally project-aware from the first implementation so a
// later central manager can administer one user across multiple TaskDeck
// project daemons without changing the data model.
type AdminStore interface {
	CreateUser(ctx context.Context, user User) error
	CreateProjectUser(ctx context.Context, user User, member ProjectMember) error
	SetUserEnabled(ctx context.Context, userID ID, enabled bool, updatedAt time.Time) error
	SetUserDisplayName(ctx context.Context, userID ID, displayName string, updatedAt time.Time) error
	SetUserPasswordHash(ctx context.Context, userID ID, passwordHash string, changedAt time.Time) error

	EnsureProject(ctx context.Context, project Project) (Project, error)
	SetProjectEnabled(ctx context.Context, projectID ID, enabled bool, updatedAt time.Time) error

	CreateRole(ctx context.Context, role Role) error
	SetRolePermissions(ctx context.Context, roleID ID, permissionKeys []string) error
	UpsertProjectMember(ctx context.Context, member ProjectMember) error
	SetMemberPermission(ctx context.Context, override MemberPermission) error
	DeleteMemberPermission(ctx context.Context, projectID, userID, permissionID ID) error
}

type AdminReader interface {
	ListProjectMembers(ctx context.Context, projectID ID) ([]ProjectMemberDetails, error)
	ListRoles(ctx context.Context) ([]RoleDetails, error)
	ListProjectRoles(ctx context.Context, projectID ID) ([]RoleDetails, error)
	ListAuthSessions(ctx context.Context, query AuthSessionQuery) ([]AuthSession, error)
	AuthSessionForProject(ctx context.Context, projectID, sessionID ID) (AuthSession, error)
}

// Store is the complete initial shared identity capability. HTTP code should
// depend on the smallest embedded interface it needs where practical.
type Store interface {
	Reader
	SessionStore
	AuditStore
	AdminStore
	AdminReader
	BootstrapStore
	Close() error
}
