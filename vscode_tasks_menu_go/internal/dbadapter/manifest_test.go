package dbadapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestValidationAndCapabilityLookup(t *testing.T) {
	manifest := Manifest{
		ID:              "mysql-cli",
		Name:            "MySQL CLI",
		Kind:            "mysql",
		ProtocolVersion: ProtocolVersion,
		Command:         "/usr/bin/taskdeck",
		Args:            []string{"--db-adapter", "mysql"},
		Capabilities: CapabilitySet{
			Connect:        true,
			Ping:           true,
			ListCatalogs:   true,
			ListObjects:    true,
			DescribeObject: true,
			Execute:        true,
			Cancel:         true,
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []Operation{
		OpHello, OpCapabilities, OpConnect, OpDisconnect, OpPing,
		OpListCatalogs, OpListObjects, OpDescribeObject, OpExecute, OpCancel,
	} {
		if !manifest.Capabilities.Supports(operation) {
			t.Fatalf("operation %q should be supported", operation)
		}
	}
	for _, operation := range []Operation{OpBegin, OpCommit, OpRollback} {
		if manifest.Capabilities.Supports(operation) {
			t.Fatalf("transaction operation %q unexpectedly supported", operation)
		}
	}
}

func TestManifestRejectsMissingCoreCapabilities(t *testing.T) {
	base := Manifest{
		ID:              "adapter",
		Name:            "Adapter",
		Kind:            "test",
		ProtocolVersion: ProtocolVersion,
		Command:         "/bin/true",
		Capabilities:    CapabilitySet{Connect: true, Ping: true},
	}
	withoutConnect := base
	withoutConnect.Capabilities.Connect = false
	if err := withoutConnect.Validate(); err == nil {
		t.Fatal("expected missing connect capability to fail")
	}
	withoutPing := base
	withoutPing.Capabilities.Ping = false
	if err := withoutPing.Validate(); err == nil {
		t.Fatal("expected missing ping capability to fail")
	}
}

func TestManifestOmitsPrivateProcessFieldsFromJSON(t *testing.T) {
	manifest := Manifest{
		ID:              "adapter",
		Name:            "Adapter",
		Kind:            "test",
		ProtocolVersion: ProtocolVersion,
		Command:         "/private/adapter",
		Args:            []string{"--secret-path", "/private/file"},
		Capabilities:    CapabilitySet{Connect: true, Ping: true},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/private/adapter", "--secret-path", "/private/file"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("process details leaked into public manifest JSON: %s", data)
		}
	}
}
