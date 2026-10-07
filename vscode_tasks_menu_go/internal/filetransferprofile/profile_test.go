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
	if got.Port != DefaultFTPPort || got.ConnectTimeoutSeconds != DefaultConnectTimeoutSecond || got.InitialPath != "." || got.FTPTLSMode != FTPTLSPlain {
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

func TestNormalizeFTPTLSModesAndPorts(t *testing.T) {
	for _, tc := range []struct {
		mode FTPTLSMode
		port int
	}{
		{FTPTLSPlain, 21},
		{FTPTLSExplicit, 21},
		{FTPTLSImplicit, 990},
	} {
		got, err := Normalize(Profile{
			ID: "ftp-prod", Name: "FTP", Protocol: ProtocolFTP,
			Host: "ftp.example.com", Username: "deploy", FTPTLSMode: tc.mode,
		})
		if err != nil { t.Fatalf("mode=%q: %v", tc.mode, err) }
		if got.FTPTLSMode != tc.mode || got.Port != tc.port {
			t.Fatalf("mode=%q got=%#v", tc.mode, got)
		}
	}
	if _, err := Normalize(Profile{
		ID: "ftp-prod", Name: "FTP", Protocol: ProtocolFTP,
		Host: "ftp.example.com", Username: "deploy", FTPTLSMode: FTPTLSMode("broken"),
	}); err == nil || !strings.Contains(err.Error(), "unsupported ftp tls mode") {
		t.Fatalf("invalid TLS mode error=%v", err)
	}
}

func TestNormalizeDefaultsAndValidatesMaxConnections(t *testing.T) {
	ftp, err := Normalize(Profile{
		ID: "ftp-pool", Name: "FTP Pool", Protocol: ProtocolFTP,
		Host: "ftp.example.com", Username: "deploy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if DefaultMaxConnections != 3 {
		t.Fatalf("DefaultMaxConnections=%d want 3", DefaultMaxConnections)
	}
	if ftp.MaxConnections != DefaultMaxConnections {
		t.Fatalf("FTP max connections=%d want default %d", ftp.MaxConnections, DefaultMaxConnections)
	}

	sftp, err := Normalize(Profile{
		ID: "sftp-pool", Name: "SFTP Pool", Protocol: ProtocolSFTP,
		SSHProfileID: "prod-ssh", MaxConnections: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sftp.MaxConnections != 7 {
		t.Fatalf("SFTP max connections=%d want 7", sftp.MaxConnections)
	}

	for _, value := range []int{-1, 17} {
		_, err := Normalize(Profile{
			ID: "bad-pool", Name: "Bad Pool", Protocol: ProtocolFTP,
			Host: "ftp.example.com", Username: "deploy", MaxConnections: value,
		})
		if err == nil || !strings.Contains(err.Error(), "max connections") {
			t.Fatalf("max_connections=%d error=%v", value, err)
		}
	}
}

func TestNormalizeSFTPRejectsFTPTLSMode(t *testing.T) {
	_, err := Normalize(Profile{
		ID: "sftp-prod", Name: "SFTP", Protocol: ProtocolSFTP,
		SSHProfileID: "prod-ssh", FTPTLSMode: FTPTLSExplicit,
	})
	if err == nil || !strings.Contains(err.Error(), "must inherit") {
		t.Fatalf("expected SFTP TLS-field rejection, got %v", err)
	}
}
