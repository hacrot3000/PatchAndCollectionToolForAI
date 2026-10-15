package patchtool

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "unicode/utf8"
)

const (
    aiPackRulesName = "PROJECT_RULES.json"
    maxProjectRulesBytes = 32 << 10
)

// AIPackProjectRules contains local project-specific AI constraints, not shared
// settings across workspaces. The UUID protects against copying stale rules
// between workspaces that happen to have the same folder name.
type AIPackProjectRules struct {
    ProjectIdentity ProjectIdentity `json:"project_identity"`
    Rules string `json:"rules"`
}

type storedAIPackProjectRules struct {
    Version int `json:"version"`
    ProjectIdentity ProjectIdentity `json:"project_identity"`
    Rules string `json:"rules"`
}

func ReadAIPackProjectRules(workspace string) (AIPackProjectRules, error) {
    identity, err := EnsureProjectIdentity(workspace)
    if err != nil { return AIPackProjectRules{}, err }
    out := AIPackProjectRules{ProjectIdentity: identity}
    dir, err := ensureAIPackCacheDir(workspace)
    if err != nil { return out, err }
    path := filepath.Join(dir, aiPackRulesName)
    info, err := os.Lstat(path)
    if os.IsNotExist(err) { return out, nil }
    if err != nil { return out, err }
    if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size()>maxProjectRulesBytes*2 {
        return out, fmt.Errorf("unsafe project rules file")
    }
    raw, err := os.ReadFile(path)
    if err != nil { return out, err }
    var saved storedAIPackProjectRules
    if err := json.Unmarshal(raw, &saved); err != nil { return out, fmt.Errorf("invalid project rules JSON: %w",err) }
    if saved.Version!=1 || saved.ProjectIdentity!=identity {
        return out, fmt.Errorf("project rules identity mismatch; refusing cross-project rules")
    }
    if len(saved.Rules)>maxProjectRulesBytes || !utf8.ValidString(saved.Rules) {
        return out, fmt.Errorf("project rules exceed text limit")
    }
    out.Rules=saved.Rules
    return out,nil
}

func SaveAIPackProjectRules(workspace, text string) (AIPackProjectRules, error) {
    if !utf8.ValidString(text) {return AIPackProjectRules{},fmt.Errorf("project rules must be valid UTF-8")}
    text=strings.TrimSpace(strings.ReplaceAll(text,"\r\n","\n"))
    if len(text)>maxProjectRulesBytes || strings.ContainsRune(text,0) {
        return AIPackProjectRules{},fmt.Errorf("project rules must be plain text <= %d bytes",maxProjectRulesBytes)
    }
    // Reading first refuses to overwrite rules from another UUID.
    existing,err:=ReadAIPackProjectRules(workspace)
    if err!=nil{return AIPackProjectRules{},err}
    dir,err:=ensureAIPackCacheDir(workspace)
    if err!=nil{return AIPackProjectRules{},err}
    if err:=writeJSONAtomic(filepath.Join(dir,aiPackRulesName),storedAIPackProjectRules{
        Version:1,ProjectIdentity:existing.ProjectIdentity,Rules:text,
    });err!=nil{return AIPackProjectRules{},err}
    existing.Rules=text
    return existing,nil
}
