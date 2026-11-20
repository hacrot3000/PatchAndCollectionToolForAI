package patchtool

import (
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestEnsureProjectIdentityMigratesExistingWorkspaceWithoutLosingConfig(t *testing.T) {
    root:=t.TempDir()
    config:=filepath.Join(root,projectConfigName)
    before:=`{"validation_profiles":{"build":{"argv":["go","test","./..."]}},"project":{"key":"original-key"}}`
    if err:=os.WriteFile(config,[]byte(before),0o600);err!=nil {t.Fatal(err)}
    identity,err:=EnsureProjectIdentity(root)
    if err!=nil {t.Fatal(err)}
    if identity.Key!="original-key" || !projectUUIDPattern.MatchString(identity.ID) {t.Fatalf("bad identity: %+v",identity)}
    again,err:=EnsureProjectIdentity(root)
    if err!=nil || again!=identity {t.Fatalf("identity must survive restart: %+v %+v %v",identity,again,err)}
    bytes,err:=os.ReadFile(config)
    if err!=nil {t.Fatal(err)}
    var data struct {
        Project ProjectIdentity `json:"project"`
        ValidationProfiles map[string]json.RawMessage `json:"validation_profiles"`
    }
    if err:=json.Unmarshal(bytes,&data);err!=nil {t.Fatal(err)}
    if data.Project!=identity || len(data.ValidationProfiles)!=1 {t.Fatalf("migration discarded settings: %s",bytes)}
}

func TestEnsureProjectIdentityCreatesDistinctIDsForSameFolderName(t *testing.T) {
    root:=t.TempDir()
    a,b:=filepath.Join(root,"one","admin-go-tool"),filepath.Join(root,"two","admin-go-tool")
    for _,p:=range []string{a,b} {if err:=os.MkdirAll(p,0o755);err!=nil {t.Fatal(err)}}
    first,err:=EnsureProjectIdentity(a);if err!=nil {t.Fatal(err)}
    second,err:=EnsureProjectIdentity(b);if err!=nil {t.Fatal(err)}
    if first.Key!="admin-go-tool" || second.Key!="admin-go-tool" || first.ID==second.ID {t.Fatalf("must disambiguate workspaces: %+v %+v",first,second)}
}

func TestEnsureProjectIdentityRejectsCorruptAndExistingMismatchedID(t *testing.T) {
    root:=t.TempDir()
    config:=filepath.Join(root,projectConfigName)
    for _,value:=range []string{
        `{"project":{"key":"test","id":"INVALID"}}`,
        `{"project":42}`,
        `{"project":{"key":""}}`,
        `not json`,
    } {
        if err:=os.WriteFile(config,[]byte(value),0o600);err!=nil {t.Fatal(err)}
        if _,err:=EnsureProjectIdentity(root);err==nil {t.Fatalf("accepted malformed config %s",value)}
        actual,err:=os.ReadFile(config);if err!=nil {t.Fatal(err)}
        if strings.TrimSpace(string(actual))!=value {t.Fatalf("invalid config mutated: %s",actual)}
    }
}

func TestEnsureProjectIdentityConcurrentCallsKeepOneUUID(t *testing.T) {
    root:=t.TempDir()
    const n=10
    got:=make(chan ProjectIdentity,n)
    errors:=make(chan error,n)
    for i:=0;i<n;i++ {
        go func(){
            identity,err:=EnsureProjectIdentity(root)
            if err!=nil {errors<-err;return}
            got<-identity
        }()
    }
    var first ProjectIdentity
    for i:=0;i<n;i++ {
        select {
        case err:=<-errors: t.Fatal(err)
        case id:=<-got:
            if first.ID=="" {first=id}
            if first!=id {t.Fatalf("concurrent IDs differ: %+v %+v",first,id)}
        }
    }
}
