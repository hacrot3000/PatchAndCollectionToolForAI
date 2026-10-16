package patchtool

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

// ProjectIdentity is a workspace-local guard, separate from shared-server login identity.
// Project keys are human-readable; the random ID distinguishes folders with the same name.
type ProjectIdentity struct {
    Key string `json:"key"`
    ID string `json:"id"`
}

const projectConfigName = ".python_patch_tool.json"

var projectUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func newProjectUUID() (string, error) {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil { return "", err }
    b[6] = (b[6] & 0x0f) | 0x40
    b[8] = (b[8] & 0x3f) | 0x80
    s := hex.EncodeToString(b[:])
    return s[:8]+"-"+s[8:12]+"-"+s[12:16]+"-"+s[16:20]+"-"+s[20:], nil
}

func readProjectIdentity(root string) (map[string]json.RawMessage, map[string]json.RawMessage, os.FileMode, error) {
    cfg := filepath.Join(root, projectConfigName)
    mode := os.FileMode(0o600)
    raw, err := os.Lstat(cfg)
    if errors.Is(err, os.ErrNotExist) { return map[string]json.RawMessage{}, map[string]json.RawMessage{}, mode, nil }
    if err != nil { return nil, nil, 0, err }
    if !raw.Mode().IsRegular() || raw.Mode()&os.ModeSymlink != 0 || raw.Size()>1024*1024 {
        return nil, nil, 0, fmt.Errorf("%s must be a regular non-symlink JSON file <= 1MiB", cfg)
    }
    mode = raw.Mode().Perm()
    data, err := os.ReadFile(cfg)
    if err != nil { return nil, nil, 0, err }
    rootMap := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &rootMap); err != nil || rootMap == nil {
        return nil, nil, 0, fmt.Errorf("invalid project config %s: %v", cfg, err)
    }
    projectMap := map[string]json.RawMessage{}
    if node, ok := rootMap["project"]; ok {
        if err := json.Unmarshal(node, &projectMap); err != nil || projectMap == nil {
            return nil, nil, 0, fmt.Errorf("project in %s must be an object", cfg)
        }
    }
    return rootMap, projectMap, mode, nil
}

// EnsureProjectIdentity migrates existing projects and initializes new ones.
// The exclusive lock prevents two TaskDeck/Patch processes from generating different IDs.
// Existing valid keys and IDs are never replaced, including when a directory is renamed.
func EnsureProjectIdentity(workspace string) (ProjectIdentity, error) {
    root, err := filepath.Abs(workspace)
    if err != nil { return ProjectIdentity{}, err }
    dir, err := os.Lstat(root)
    if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
        return ProjectIdentity{}, fmt.Errorf("unsafe workspace for project identity: %s", root)
    }
    lock := filepath.Join(root, projectConfigName+".identity.lock")
    var owner *os.File
    for tries:=0; tries<100; tries++ {
        owner,err=os.OpenFile(lock,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0o600)
        if err==nil { break }
        if !errors.Is(err,os.ErrExist) { return ProjectIdentity{},err }
        time.Sleep(50*time.Millisecond)
    }
    if owner==nil { return ProjectIdentity{},fmt.Errorf("project identity lock is busy: %s",lock) }
    defer func(){ _=owner.Close(); _=os.Remove(lock) }()
    rootMap,projectMap,mode,err:=readProjectIdentity(root)
    if err!=nil { return ProjectIdentity{},err }
    key := filepath.Base(root)
    id := ""
    if raw,ok:=projectMap["key"];ok {
        if err:=json.Unmarshal(raw,&key);err!=nil || strings.TrimSpace(key)=="" {
            return ProjectIdentity{},fmt.Errorf("project.key must be a non-empty string")
        }
    }
    if strings.TrimSpace(key)=="" || key=="." || key==string(filepath.Separator) {
        return ProjectIdentity{},fmt.Errorf("invalid project directory name")
    }
    if raw,ok:=projectMap["id"];ok {
        if err:=json.Unmarshal(raw,&id);err!=nil || !projectUUIDPattern.MatchString(id) {
            return ProjectIdentity{},fmt.Errorf("project.id must be a valid lowercase UUID; refusing silent replacement")
        }
    }
    identity:=ProjectIdentity{Key:key,ID:id}
    if len(projectMap["key"])>0 && id!="" { return identity,nil }
    if id=="" {
        identity.ID,err=newProjectUUID()
        if err!=nil { return ProjectIdentity{},err }
    }
    projectMap["key"],_ = json.Marshal(identity.Key)
    projectMap["id"],_ = json.Marshal(identity.ID)
    rootMap["project"],err=json.Marshal(projectMap)
    if err!=nil { return ProjectIdentity{},err }
    output,err:=json.MarshalIndent(rootMap,"","  ")
    if err!=nil { return ProjectIdentity{},err }
    if len(output)>1024*1024 { return ProjectIdentity{},fmt.Errorf("project config exceeds size limit") }
    temp,err:=os.CreateTemp(root,".project-identity-*.tmp")
    if err!=nil { return ProjectIdentity{},err }
    tempName:=temp.Name()
    defer os.Remove(tempName)
    if err=temp.Chmod(mode);err==nil { _,err=temp.Write(append(output,'\n')) }
    if err==nil { err=temp.Sync() }
    closeErr:=temp.Close()
    if err==nil { err=closeErr }
    if err!=nil { return ProjectIdentity{},err }
    if err=os.Rename(tempName,filepath.Join(root,projectConfigName));err!=nil {
        return ProjectIdentity{},err
    }
    return identity,nil
}
