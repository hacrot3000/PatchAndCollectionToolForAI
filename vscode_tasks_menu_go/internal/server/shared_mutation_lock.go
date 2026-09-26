package server

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

type sharedMutationOwner struct {
	UserID     identity.ID `json:"user_id"`
	Username   string      `json:"username"`
	Operation  string      `json:"operation"`
	ResourceID string      `json:"resource_id,omitempty"`
	AcquiredAt time.Time   `json:"acquired_at"`
}

type sharedMutationLease struct {
	token string
	owner sharedMutationOwner
}

// sharedMutationLock is process-local because one TaskDeck daemon owns one
// workspace. Its zero value is ready for use and contains no filesystem or
// network dependency.
type sharedMutationLock struct {
	mu     sync.Mutex
	token  string
	holder *sharedMutationOwner
}

func (l *sharedMutationLock) acquire(principal identity.Principal, operation, resourceID string, now time.Time) (sharedMutationLease, *sharedMutationOwner, error) {
	operation = strings.TrimSpace(operation)
	resourceID = strings.TrimSpace(resourceID)
	if principal.UserID == "" || operation == "" || now.IsZero() {
		return sharedMutationLease{}, nil, fmt.Errorf("shared mutation lock requires principal, operation and timestamp")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder != nil {
		conflict := *l.holder
		return sharedMutationLease{}, &conflict, nil
	}
	tokenID, err := identity.NewID()
	if err != nil {
		return sharedMutationLease{}, nil, err
	}
	owner := sharedMutationOwner{
		UserID: principal.UserID,
		Username: principal.Username,
		Operation: operation,
		ResourceID: resourceID,
		AcquiredAt: now.UTC(),
	}
	l.token = string(tokenID)
	l.holder = &owner
	return sharedMutationLease{token: l.token, owner: owner}, nil, nil
}

func (l *sharedMutationLock) bindResource(token, resourceID string) bool {
	resourceID = strings.TrimSpace(resourceID)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || token == "" || token != l.token {
		return false
	}
	l.holder.ResourceID = resourceID
	return true
}

func (l *sharedMutationLock) release(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || token == "" || token != l.token {
		return false
	}
	l.token = ""
	l.holder = nil
	return true
}

func (l *sharedMutationLock) snapshot() (sharedMutationOwner, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil {
		return sharedMutationOwner{}, false
	}
	return *l.holder, true
}
