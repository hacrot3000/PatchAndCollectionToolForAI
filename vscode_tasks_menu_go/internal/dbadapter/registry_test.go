package dbadapter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRegistryRegistersListsAndCopiesManifests(t *testing.T) {
	registry := NewRegistry()
	mysql := Manifest{
		ID:              "mysql-cli",
		Name:            "MySQL CLI",
		Kind:            "mysql",
		ProtocolVersion: ProtocolVersion,
		Command:         "/private/taskdeck",
		Args:            []string{"--db-adapter", "mysql"},
		Capabilities:    CapabilitySet{Connect: true, Ping: true, Execute: true},
	}
	if err := registry.Register(mysql); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(mysql); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}

	got, err := registry.Get("mysql-cli")
	if err != nil {
		t.Fatal(err)
	}
	got.Args[0] = "changed"
	again, err := registry.Get("mysql-cli")
	if err != nil {
		t.Fatal(err)
	}
	if again.Args[0] != "--db-adapter" {
		t.Fatalf("registry manifest args were mutated through copy: %#v", again.Args)
	}

	list := registry.List()
	if len(list) != 1 || list[0].ID != "mysql-cli" {
		t.Fatalf("list=%+v", list)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/private/taskdeck", "--db-adapter"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("private process field leaked through registry JSON: %s", data)
		}
	}
}

func TestRegistryRejectsUnknownAdapter(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Get("missing"); !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("error=%v want ErrAdapterNotFound", err)
	}
}
