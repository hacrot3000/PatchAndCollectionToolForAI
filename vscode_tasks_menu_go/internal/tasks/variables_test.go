package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVSCodeEditorVariablesProcessAndShell(t *testing.T) {
	workspace := t.TempDir()
	file := filepath.Join(workspace, "a'b $(printf attack).py")
	ctx := VariableContext{File:file,LineNumber:7,ColumnNumber:12,SelectedText:"hello $USER",WorkspaceFolders:map[string]string{"project":workspace}}
	task := Task{Label:"process",Type:"process",Command:"echo",
		Args:[]any{"${file}","${fileBasenameNoExtension}","${relativeFile:project}","${lineNumber}","${columnNumber}","${selectedText}"}}
	spec,err:=ResolveExecutionWithContext(task,workspace,nil,ctx)
	if err!=nil { t.Fatal(err) }
	if spec.Args[0]!=file||spec.Args[1]!="a'b $(printf attack)"||spec.Args[2]!="a'b $(printf attack).py"||spec.Args[3]!="7"||spec.Args[4]!="12"||spec.Args[5]!="hello $USER" { t.Fatalf("args=%#v",spec.Args) }
	shell:=Task{Label:"quoted",Type:"shell",Command:"printf '%s\\n' '${file}' \"${fileBasename}\" ${selectedText}"}
	spec,err=ResolveExecutionWithContext(shell,workspace,nil,ctx)
	if err!=nil { t.Fatal(err) }
	output,err:=exec.Command(spec.Command,spec.Args...).CombinedOutput()
	if err!=nil { t.Fatalf("shell: %v %s",err,output) }
	if string(output)!=file+"\n"+filepath.Base(file)+"\nhello $USER\n" { t.Fatalf("shell output %q",output) }
}

func TestVSCodeEditorVariablesRejectMissingActiveFile(t *testing.T) {
	task:=Task{Label:"file",Type:"shell",Command:"echo '${file}'"}
	_,err:=ResolveExecutionWithContext(task,t.TempDir(),nil,VariableContext{})
	if err==nil||!strings.Contains(err.Error(),"active") { t.Fatalf("err=%v",err) }
}

func TestVSCodeEditorVariablesExpandCwdEnvAndSettings(t *testing.T) {
	workspace:=t.TempDir()
	if err:=os.MkdirAll(filepath.Join(workspace,".vscode"),0755);err!=nil { t.Fatal(err) }
	data:=[]byte("{// comment\n\"python.test\":\"yes\", \"build\":{\"jobs\":4},}")
	if err:=os.WriteFile(filepath.Join(workspace,".vscode","settings.json"),data,0600);err!=nil { t.Fatal(err) }
	config,err:=LoadVSCodeConfiguration(workspace);if err!=nil { t.Fatal(err) }
	ctx:=VariableContext{File:filepath.Join(workspace,"src","hello.py"),Config:config}
	task:=Task{Label:"vars",Type:"process",Command:"echo",
		Args:[]any{"${config:python.test}","${config:build.jobs}","${cwd}","${fileDirnameBasename}","${fileExtname}"},
		Options:map[string]any{"cwd":"${workspaceFolder}/src","env":map[string]any{"TARGET":"${fileBasename}"}}}
	spec,err:=ResolveExecutionWithContext(task,workspace,nil,ctx)
	if err!=nil { t.Fatal(err) }
	if spec.Args[0]!="yes"||spec.Args[1]!="4"||spec.Args[2]!=filepath.Join(workspace,"src")||spec.Args[3]!="src"||spec.Args[4]!=".py" { t.Fatalf("args=%#v",spec.Args) }
	found:=false;for _,v:=range spec.Env { if v=="TARGET=hello.py" { found=true } }
	if !found { t.Fatal("missing target env") }
}

func TestVSCodeEditorVariablesAvoidNestedShellExpansion(t *testing.T) {
	workspace:=t.TempDir()
	ctx:=VariableContext{File:filepath.Join(workspace,"a'b.py"),SelectedText:"$(touch /tmp/not-a-real-file) '${file}'"}
	task:=Task{Label:"safe",Type:"shell",Command:"printf '%s' \"${selectedText}\""}
	spec,err:=ResolveExecutionWithContext(task,workspace,nil,ctx)
	if err!=nil { t.Fatal(err) }
	output,err:=exec.Command(spec.Command,spec.Args...).CombinedOutput()
	if err!=nil { t.Fatal(err) }
	if string(output)!=ctx.SelectedText { t.Fatalf("unexpected expansion %q",output) }
}
