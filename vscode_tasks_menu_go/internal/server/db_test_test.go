package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func TestPrepareDatabaseTestProfileUsesDraftFieldsAndSavedSecret(t *testing.T) {
	s, store, secrets := newDBProfileAPITestServer(t)

	sshStore, err := sshprofile.NewStore(t.TempDir() + "/ssh_profiles.json")
	if err != nil {
		t.Fatal(err)
	}
	sshProfile, err := sshStore.Create(sshprofile.Profile{
		ID:         "jump-1",
		Name:       "Jump",
		Host:       "10.18.23.50",
		Username:   "deploy",
		AuthMethod: sshprofile.AuthAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.SSHProfiles = sshStore

	const secretRef = "db/saved/auth/test"
	if err := secrets.Put(secretRef, []byte("saved-password")); err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(dbprofile.Profile{
		ID:           "saved-db",
		Name:         "Saved DB",
		AdapterID:    "mysql-cli",
		Transport:    dbprofile.TransportSSHTunnel,
		Host:         "10.18.23.50",
		Port:         3306,
		Username:     "root",
		SSHProfileID: sshProfile.ID,
		SecretRef:    secretRef,
	})
	if err != nil {
		t.Fatal(err)
	}

	draft := dbProfileRequest{
		Name:         "Edited DB",
		AdapterID:    "mysql-cli",
		Transport:    dbprofile.TransportSSHTunnel,
		Host:         "localhost",
		Port:         3306,
		Username:     "root",
		SSHProfileID: sshProfile.ID,
		Options:      map[string]string{"connect_timeout_seconds": "10"},
	}
	candidate, explicitSecret, err := s.prepareDatabaseTestProfile("saved-db", draft)
	if err != nil {
		t.Fatal(err)
	}
	if explicitSecret != nil {
		t.Fatalf("blank edit secret unexpectedly became explicit: %q", *explicitSecret)
	}
	if candidate.Host != "localhost" || candidate.Port != 3306 || candidate.Username != "root" {
		t.Fatalf("draft endpoint was not preserved: %+v", candidate)
	}
	if candidate.Transport != dbprofile.TransportSSHTunnel || candidate.SSHProfileID != sshProfile.ID {
		t.Fatalf("draft SSH tunnel was not preserved: %+v", candidate)
	}
	if candidate.SecretRef != secretRef {
		t.Fatalf("saved secret ref=%q want %q", candidate.SecretRef, secretRef)
	}

	stored, err := store.Get("saved-db")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Host != "10.18.23.50" {
		t.Fatalf("database test mutated saved profile host=%q", stored.Host)
	}
}
