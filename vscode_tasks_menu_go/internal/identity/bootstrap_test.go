package identity

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const testScryptHash = "$scrypt$v=1,ln=17,r=8,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestBootstrapFirstAdminIsAtomicAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), identityDBName)
	first := openRealSQLiteDatabase(t, path)
	second := openRealSQLiteDatabase(t, path)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, db := range []*sqliteDatabase{first, second} {
		wg.Add(1)
		go func(i int, db *sqliteDatabase) {
			defer wg.Done()
			_, err := db.BootstrapFirstAdmin(context.Background(), "test", []string{"alice", "bob"}[i], testScryptHash, time.Now())
			results <- err
		}(i, db)
	}
	wg.Wait()
	close(results)
	won, refused := 0, 0
	for err := range results {
		if err == nil {
			won++
		} else if errors.Is(err, ErrBootstrapUnavailable) {
			refused++
		} else {
			t.Fatal(err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("won=%d refused=%d", won, refused)
	}
	var userID string
	if err := first.db.QueryRow("SELECT id FROM users").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	user, err := first.UserByID(context.Background(), ID(userID))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ResolveUserPrincipal(context.Background(), first, "test", user)
	if err != nil || !p.Allowed(PermissionProjectAdmin) {
		t.Fatalf("admin missing: %v %+v", err, p)
	}
	if err := first.SetUserEnabled(context.Background(), user.ID, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.BootstrapFirstAdmin(context.Background(), "other", "newadmin", testScryptHash, time.Now()); !errors.Is(err, ErrBootstrapUnavailable) {
		t.Fatalf("disabled admin bypass: %v", err)
	}
}

func TestBootstrapFailureRollsBackAllIdentityChanges(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	if err := db.CreateRole(context.Background(), Role{ID: "conflict", Name: "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.BootstrapFirstAdmin(context.Background(), "test", "alice", testScryptHash, time.Now()); err == nil {
		t.Fatal("expected role conflict")
	}
	for _, table := range []string{"users", "projects", "permissions", "project_members", "audit_log"} {
		var count int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial bootstrap in %s: %d rows", table, count)
		}
	}
}
