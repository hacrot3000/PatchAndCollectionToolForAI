package sshaskpass

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
)

type memoryStore struct {
	mu      sync.Mutex
	records map[string][]byte
}

func (s *memoryStore) Put(id string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = map[string][]byte{}
	}
	s.records[id] = append([]byte(nil), value...)
	return nil
}

func (s *memoryStore) Get(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.records[id]
	if !ok {
		return nil, secretstore.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *memoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

func TestAskpassTicketIsOneTime(t *testing.T) {
	store := &memoryStore{records: map[string][]byte{
		"ssh/prod/auth/test": []byte("correct-horse"),
	}}
	ticket, err := Prepare(store, "ssh/prod/auth/test", filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()

	var out bytes.Buffer
	if err := RunHelper(ticket.SocketPath, ticket.Token, "Password:", &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "correct-horse\n" {
		t.Fatalf("helper output = %q", got)
	}

	var second bytes.Buffer
	if err := RunHelper(ticket.SocketPath, ticket.Token, "Password:", &second); err == nil {
		t.Fatal("expected one-time ticket to reject a second request")
	}
}

func TestAskpassRejectsWrongTokenWithoutLeakingSecret(t *testing.T) {
	store := &memoryStore{records: map[string][]byte{
		"ssh/prod/auth/test": []byte("top-secret"),
	}}
	ticket, err := Prepare(store, "ssh/prod/auth/test", filepath.Join(t.TempDir(), "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	defer ticket.Close()

	var out bytes.Buffer
	err = RunHelper(ticket.SocketPath, strings.Repeat("0", len(ticket.Token)), "Password:", &out)
	if err == nil {
		t.Fatal("expected wrong token to fail")
	}
	if strings.Contains(err.Error(), "top-secret") || strings.Contains(out.String(), "top-secret") {
		t.Fatalf("secret leaked on rejected token: err=%v output=%q", err, out.String())
	}
}

func TestPrepareFailsWhenSecretMissing(t *testing.T) {
	store := &memoryStore{records: map[string][]byte{}}
	_, err := Prepare(store, "ssh/prod/auth/missing", filepath.Join(t.TempDir(), "runtime"))
	if err == nil || !errors.Is(err, secretstore.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
