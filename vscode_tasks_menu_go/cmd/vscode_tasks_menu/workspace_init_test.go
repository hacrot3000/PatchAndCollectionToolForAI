package main

import (
 "bytes"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestMissingTasksInteractiveYesCreatesWorkspace(t *testing.T){
 root:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(root,"go.mod"),[]byte("module example.org/demo"),0644);err!=nil{t.Fatal(err)}
 var output bytes.Buffer
 err:=bootstrapWorkspaceTasks(root,strings.NewReader("y\n"),&output,true,false)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(output.String(),"Go")||!strings.Contains(output.String(),"Đã tạo"){
  t.Fatalf("expected detection and confirmation; got %s",output.String())
 }
 list,err:=tasks.Load(root);if err!=nil{t.Fatal(err)}
 if len(list)<2{t.Fatalf("expected Go tasks, got %v",list)}
}
func TestMissingTasksNoAndNoTTYDoNotCreate(t *testing.T){
 for _,tc:=range []struct{name string; interactive bool; input string}{
  {"decline",true,"n\n"},{"default",true,"\n"},{"notty",false,""},
 }{
  t.Run(tc.name,func(t *testing.T){
   root:=t.TempDir()
   err:=bootstrapWorkspaceTasks(root,strings.NewReader(tc.input),new(bytes.Buffer),tc.interactive,false)
   if err==nil{t.Fatal("missing tasks unexpectedly accepted")}
   _,statErr:=os.Stat(filepath.Join(root,".vscode","tasks.json"))
   if !os.IsNotExist(statErr){t.Fatalf("created file without consent: %v",statErr)}
  })
 }
}
func TestExplicitInitWithoutTerminalScaffoldsProject(t *testing.T){
 root:=t.TempDir()
 var output bytes.Buffer
 if err:=bootstrapWorkspaceTasks(root,strings.NewReader(""),&output,false,true);err!=nil{t.Fatal(err)}
 if _,err:=tasks.Load(root);err!=nil{t.Fatal(err)}
}
func TestExistingTasksAreNeverReplaced(t *testing.T){
 root:=t.TempDir()
 file:=filepath.Join(root,".vscode","tasks.json")
 if err:=os.MkdirAll(filepath.Dir(file),0755);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(file,[]byte(`{"version":"2.0.0","tasks":[]}`),0644);err!=nil{t.Fatal(err)}
 if err:=bootstrapWorkspaceTasks(root,strings.NewReader("y
"),new(bytes.Buffer),true,true);err!=nil{t.Fatal(err)}
 body,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(body),`"tasks":[]`){t.Fatalf("existing tasks overwritten: %s",body)}
}
func TestNoAccidentalProjectCreationOutsideWorkspace(t *testing.T){
 root:=t.TempDir()
 missing:=filepath.Join(root,"missing")
 if err:=bootstrapWorkspaceTasks(missing,strings.NewReader("y
"),new(bytes.Buffer),true,false);err==nil{
  t.Fatal("nonexistent workspace must not be created by guesswork")
 }
 if _,err:=os.Stat(missing);!os.IsNotExist(err){t.Fatalf("unexpected workspace creation: %v",err)}
}
