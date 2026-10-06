package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

func newConfigBackupTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	sshStore, err := sshprofile.NewStore(filepath.Join(root, "user", "ssh.json"))
	if err != nil { t.Fatal(err) }
	dbStore, err := dbprofile.NewStore(filepath.Join(root, "user", "db.json"))
	if err != nil { t.Fatal(err) }
	transferStore, err := filetransferprofile.NewStore(filepath.Join(root, "user", "transfer.json"))
	if err != nil { t.Fatal(err) }
	return &Server{
		Workspace: filepath.Join(root, "workspace"),
		SSHProfiles: sshStore,
		DBProfiles: dbStore,
		FileTransferProfiles: transferStore,
	}
}

func seedConfigBackupProfiles(t *testing.T, s *Server) {
	t.Helper()
	if err := os.MkdirAll(s.Workspace, 0o755); err != nil { t.Fatal(err) }
	if err := s.SSHProfiles.Save([]sshprofile.Profile{{
		ID:"prod-ssh",Name:"Production SSH",Host:"prod.example.test",Port:22,Username:"deploy",
		AuthMethod:sshprofile.AuthPassword,SecretRef:"ssh/prod/password",
	}}); err != nil { t.Fatal(err) }
	if err := s.DBProfiles.Save([]dbprofile.Profile{{
		ID:"prod-db",Name:"Production DB",AdapterID:"mysql-cli",Transport:dbprofile.TransportDirect,
		Host:"127.0.0.1",Port:3306,Username:"app",Database:"app",SecretRef:"db/prod/password",
	}}); err != nil { t.Fatal(err) }
	if err := s.FileTransferProfiles.Save([]filetransferprofile.Profile{{
		ID:"prod-ftp",Name:"Production FTP",Protocol:filetransferprofile.ProtocolFTP,
		Host:"ftp.example.test",Port:21,Username:"deploy",InitialPath:".",SecretRef:"file-transfer/prod/password",
	}}); err != nil { t.Fatal(err) }
}

func TestConfigBackupExportNeverContainsSecretReferences(t *testing.T) {
	s := newConfigBackupTestServer(t)
	seedConfigBackupProfiles(t,s)
	rr := httptest.NewRecorder()
	s.configBackup(rr, httptest.NewRequest(http.MethodGet,"/api/config-backup",nil))
	if rr.Code != http.StatusOK { t.Fatalf("status=%d body=%s",rr.Code,rr.Body.String()) }
	body:=rr.Body.String()
	for _, forbidden:=range []string{"ssh/prod/password","db/prod/password","file-transfer/prod/password","secret_ref"} {
		if strings.Contains(body,forbidden) { t.Fatalf("backup leaked %q: %s",forbidden,body) }
	}
	if strings.Count(body,`"credential_required":true`) < 3 {
		t.Fatalf("backup did not preserve credential-required metadata: %s",body)
	}
}

func TestConfigBackupRestoreCreatesMissingCredentialReferencesOnNewMachine(t *testing.T) {
	source:=newConfigBackupTestServer(t);seedConfigBackupProfiles(t,source)
	bundle,_,err:=source.captureConfigBackup();if err!=nil{t.Fatal(err)}

	target:=newConfigBackupTestServer(t)
	if err:=os.MkdirAll(target.Workspace,0o755);err!=nil{t.Fatal(err)}
	validated,err:=validateConfigBackupBundle(bundle);if err!=nil{t.Fatal(err)}
	_,actual,err:=target.captureConfigBackup();if err!=nil{t.Fatal(err)}
	if err:=target.applyConfigBackup(validated,"replace",actual);err!=nil{t.Fatal(err)}

	sshProfiles,err:=target.SSHProfiles.Load();if err!=nil{t.Fatal(err)}
	if len(sshProfiles)!=1 || !strings.HasPrefix(sshProfiles[0].SecretRef,"managed/import/ssh/") {
		t.Fatalf("restored SSH profile=%+v",sshProfiles)
	}
	dbProfiles,err:=target.DBProfiles.Load();if err!=nil{t.Fatal(err)}
	if len(dbProfiles)!=1 || !strings.HasPrefix(dbProfiles[0].SecretRef,"managed/import/database/") {
		t.Fatalf("restored DB profile=%+v",dbProfiles)
	}
	transferProfiles,err:=target.FileTransferProfiles.Load();if err!=nil{t.Fatal(err)}
	if len(transferProfiles)!=1 || !strings.HasPrefix(transferProfiles[0].SecretRef,"managed/import/file-transfer/") {
		t.Fatalf("restored transfer profile=%+v",transferProfiles)
	}
}

func TestConfigBackupRestorePreservesExistingLocalCredentialByProfileID(t *testing.T) {
	s:=newConfigBackupTestServer(t);seedConfigBackupProfiles(t,s)
	bundle,actual,err:=s.captureConfigBackup();if err!=nil{t.Fatal(err)}
	bundle.Connections.SSH[0].Profile.Name="Renamed SSH"
	validated,err:=validateConfigBackupBundle(bundle);if err!=nil{t.Fatal(err)}
	if err:=s.applyConfigBackup(validated,"merge",actual);err!=nil{t.Fatal(err)}
	profiles,err:=s.SSHProfiles.Load();if err!=nil{t.Fatal(err)}
	if len(profiles)!=1 || profiles[0].Name!="Renamed SSH" || profiles[0].SecretRef!="ssh/prod/password" {
		t.Fatalf("profile=%+v",profiles)
	}
}

func TestConfigBackupAPIRejectsInvalidBundleBeforeRestore(t *testing.T) {
	s:=newConfigBackupTestServer(t);seedConfigBackupProfiles(t,s)
	before,_,err:=s.captureConfigBackup();if err!=nil{t.Fatal(err)}
	raw,_:=json.Marshal(configRestoreRequest{Mode:"replace",Bundle:configBackupBundle{Version:999}})
	rr:=httptest.NewRecorder()
	req:=httptest.NewRequest(http.MethodPost,"/api/config-backup",strings.NewReader(string(raw)))
	req.Header.Set("Content-Type","application/json")
	s.configBackup(rr,req)
	if rr.Code!=http.StatusBadRequest { t.Fatalf("status=%d body=%s",rr.Code,rr.Body.String()) }
	after,_,err:=s.captureConfigBackup();if err!=nil{t.Fatal(err)}
	if len(before.Connections.SSH)!=len(after.Connections.SSH) || after.Connections.SSH[0].Profile.Name!="Production SSH" {
		t.Fatalf("invalid restore mutated state: before=%+v after=%+v",before.Connections,after.Connections)
	}
}
