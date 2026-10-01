package filetransferprofile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeFTPDefaultsAndHidesSecretRef(t *testing.T) {
	got, err := Normalize(Profile{
		ID:       "ftp-prod",
		Name:     " FTP Production ",
		Protocol: ProtocolFTP,
		Host:     "ftp.example.com",
		Username: "deploy",
		SecretRef: "file-transfer/ftp-prod/password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != DefaultFTPPort || got.ConnectTimeoutSeconds != DefaultConnectTimeoutSecond || got.InitialPath != "." {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret_ref") || strings.Contains(string(data), got.SecretRef) {
		t.Fatalf("secret reference leaked to JSON: %s", data)
	}
}

func TestNormalizeSFTPInheritsSSHConnection(t *testing.T) {
	got, err := Normalize(Profile{
		ID:           "sftp-prod",
		Name:         "SFTP Production",
		Protocol:     ProtocolSFTP,
		SSHProfileID: "prod-ssh",
		InitialPath:  "/srv/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHProfileID != "prod-ssh" || got.InitialPath != "/srv/app" {
		t.Fatalf("unexpected normalized SFTP profile: %#v", got)
	}
}

func TestNormalizeSFTPRejectsDuplicatedSSHCredentials(t *testing.T) {
	_, err := Normalize(Profile{
		ID:           "sftp-prod",
		Name:         "SFTP Production",
		Protocol:     ProtocolSFTP,
		SSHProfileID: "prod-ssh",
		Host:         "example.com",
	})
	if err == nil || !strings.Contains(err.Error(), "must inherit") {
		t.Fatalf("expected inherited SSH field rejection, got %v", err)
	}
}

func TestNormalizeRejectsRemotePathControlCharacters(t *testing.T) {
	_, err := Normalize(Profile{
		ID:       "ftp-prod",
		Name:     "FTP",
		Protocol: ProtocolFTP,
		Host:     "ftp.example.com",
		Username: "deploy",
		InitialPath: "/srv\nsecret",
	})
	if err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("expected control-character error, got %v", err)
	}
}
