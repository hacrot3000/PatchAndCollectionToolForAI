package restrictedpolicy

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestDefaultPolicyIsValid(t *testing.T) {
 p:=Default()
 if err:=p.Validate();err!=nil {t.Fatal(err)}
 if p.MaxCopyLines>=p.MaxVisibleLines {t.Fatal("copy budget should be smaller than viewer window")}
}
func TestConfigurableExposureLimits(t *testing.T) {
 p:=Default();p.MaxVisibleLines=60;p.MaxCopyLines=12;p.MaxCopyBytes=512
 p.MaxUniqueLinesPerFile=300;p.MaxUniqueLinesPerDay=600;p.MaxRevealPercent=70
 if err:=p.Validate();err!=nil {t.Fatal(err)}
 p.MaxCopyLines=61
 if err:=p.Validate();err==nil {t.Fatal("copy must not exceed visible")}
}
func TestRejectUnsafePolicyAndUnknownFields(t *testing.T) {
 dir:=t.TempDir()
 file:=filepath.Join(dir,"restricted.json")
 for _,test:=range []struct{input string; valid bool}{
  {`{"max_visible_lines":40,"max_copy_lines":8,"max_unique_lines_per_file":200}`,true},
  {`{"max_copy_lines":9999}`,false},
  {`{"allowed_commands":["git","cat"]}`,true}, // Groups still require gateway allowlist on backend
  {`{"allowed_commands":["git;sh"]}`,false},
  {`{"allowed_commands":["git","git"]}`,false},
  {`{"hidden_unapproved_setting":true}`,false},
  {`{}`+"\n"+`{}`,false},
 } {
  if err:=os.WriteFile(file,[]byte(test.input),0600);err!=nil {t.Fatal(err)}
  _,err:=Load(file)
  if (err==nil)!=test.valid {t.Errorf("Load(%s) err=%v",test.input,err)}
 }
 if _,err:=Load("relative.json");err==nil {t.Fatal("relative config path accepted")}
 if err:=os.Remove(file);err!=nil {t.Fatal(err)}
 if err:=os.Symlink(filepath.Join(dir,"real"),file);err!=nil {t.Fatal(err)}
 if _,err:=Load(file);err==nil||!strings.Contains(err.Error(),"regular") {t.Fatalf("symlink not rejected: %v",err)}
}
