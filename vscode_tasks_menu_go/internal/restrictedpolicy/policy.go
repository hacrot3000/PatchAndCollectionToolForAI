// Package restrictedpolicy defines immutable, launch-time-only security settings.
// This policy must be enforced by the backend; UI hints are not access control.
package restrictedpolicy

import (
 "bytes"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
)

const MaxPolicyBytes = 64 << 10

type Policy struct {
 MaxVisibleLines int `json:"max_visible_lines"`
 MaxCopyLines int `json:"max_copy_lines"`
 MaxCopyBytes int `json:"max_copy_bytes"`
 MaxUniqueLinesPerFile int `json:"max_unique_lines_per_file"`
 MaxUniqueLinesPerDay int `json:"max_unique_lines_per_day"`
 MaxRevealPercent int `json:"max_reveal_percent"`
 AllowedCommands []string `json:"allowed_commands"`
}

func Default() Policy {
 return Policy{
  MaxVisibleLines:80, MaxCopyLines:10, MaxCopyBytes:2048,
  MaxUniqueLinesPerFile:240, MaxUniqueLinesPerDay:2000,
  MaxRevealPercent:80, AllowedCommands:[]string{"git","ls","pwd"},
 }
}

func (p Policy) Validate() error {
 if p.MaxVisibleLines < 1 || p.MaxVisibleLines > 1000 {return errors.New("max_visible_lines must be between 1 and 1000")}
 if p.MaxCopyLines < 0 || p.MaxCopyLines > p.MaxVisibleLines {return errors.New("max_copy_lines must be between 0 and max_visible_lines")}
 if p.MaxCopyBytes < 0 || p.MaxCopyBytes > 65536 {return errors.New("max_copy_bytes must be between 0 and 65536")}
 if p.MaxUniqueLinesPerFile < 1 || p.MaxUniqueLinesPerFile > 100000 {return errors.New("max_unique_lines_per_file must be between 1 and 100000")}
 if p.MaxUniqueLinesPerDay < 1 || p.MaxUniqueLinesPerDay > 1000000 {return errors.New("max_unique_lines_per_day must be between 1 and 1000000")}
 if p.MaxVisibleLines > p.MaxUniqueLinesPerFile || p.MaxUniqueLinesPerFile > p.MaxUniqueLinesPerDay {
  return errors.New("line limits must satisfy visible <= per-file <= per-day")
 }
 if p.MaxRevealPercent < 1 || p.MaxRevealPercent > 100 {return errors.New("max_reveal_percent must be between 1 and 100")}
 if len(p.AllowedCommands) > 32 {return errors.New("too many allowed command groups")}
 seen:=make(map[string]bool,len(p.AllowedCommands))
 for _,name:=range p.AllowedCommands {
  if name=="" || strings.Trim(name,"abcdefghijklmnopqrstuvwxyz0123456789_-")!="" ||
    strings.TrimSpace(name)!=name || seen[name] {
   return fmt.Errorf("invalid or repeated command group %q",name)
  }
  seen[name]=true
 }
 return nil
}

func Load(path string) (Policy,error) {
 if !filepath.IsAbs(path) {return Policy{},errors.New("restricted policy file must be an absolute path")}
 info,err:=os.Lstat(path)
 if err!=nil {return Policy{},err}
 if !info.Mode().IsRegular() || info.Mode().Perm()&0022!=0 {
  return Policy{},errors.New("restricted policy must be a regular non-group/world-writable file, not a symlink")
 }
 f,err:=os.Open(path)
 if err!=nil {return Policy{},err}
 defer f.Close()
 data,err:=io.ReadAll(io.LimitReader(f,MaxPolicyBytes+1))
 if err!=nil {return Policy{},err}
 if len(data)>MaxPolicyBytes {return Policy{},errors.New("restricted policy file is too large")}
 // A launch policy must be an explicit JSON object. JSON null would
 // otherwise silently retain defaults when decoded into a struct.
 trimmed:=bytes.TrimSpace(data)
 if len(trimmed)==0||trimmed[0]!='{' {
  return Policy{},errors.New("restricted policy must be a JSON object")
 }
 p:=Default()
 dec:=json.NewDecoder(bytes.NewReader(data))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&p);err!=nil {return Policy{},fmt.Errorf("restricted policy JSON: %w",err)}
 var extra any
 if err:=dec.Decode(&extra);err!=io.EOF {return Policy{},errors.New("restricted policy must contain exactly one JSON object")}
 if err:=p.Validate();err!=nil {return Policy{},err}
 return p,nil
}
