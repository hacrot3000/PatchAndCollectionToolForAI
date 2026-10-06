package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLSPServerMappingUsesHostInstalledTools(t *testing.T) {
	tests:=[]struct{path,name,language string}{
		{"main.go","gopls","go"},
		{"src/main.cpp","clangd","cpp"},
		{"script.py","pylsp","python"},
		{"app.ts","typescript-language-server","typescript"},
		{"app.js","typescript-language-server","javascript"},
	}
	for _,test:=range tests{
		spec,ok:=lspSpecForPath(test.path)
		if !ok||spec.Name!=test.name||spec.LanguageID!=test.language{
			t.Fatalf("path=%s spec=%+v ok=%v",test.path,spec,ok)
		}
	}
	if _,ok:=lspSpecForPath("README.md");ok{t.Fatal("Markdown unexpectedly mapped to an LSP server")}
}

func TestLSPWireFramingRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	want:=map[string]any{"jsonrpc":"2.0","id":7,"method":"textDocument/hover","params":map[string]any{"x":1}}
	if err:=writeLSPMessage(&buffer,want);err!=nil{t.Fatal(err)}
	got,err:=readLSPMessage(bufio.NewReader(&buffer));if err!=nil{t.Fatal(err)}
	if id,ok:=rawIDNumber(got.ID);!ok||id!=7{t.Fatalf("id=%d ok=%v",id,ok)}
	if got.Method!="textDocument/hover"{t.Fatalf("method=%q",got.Method)}
}

func TestLSPFileURINormalizationRejectsWorkspaceEscape(t *testing.T) {
	root:=t.TempDir()
	inside:=filepath.Join(root,"src","main.go")
	if err:=os.MkdirAll(filepath.Dir(inside),0o755);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(inside,[]byte("package main\n"),0o644);err!=nil{t.Fatal(err)}
	pathValue,err:=fileURIToProjectPath(root,pathToFileURI(inside));if err!=nil{t.Fatal(err)}
	if pathValue!="src/main.go"{t.Fatalf("project path=%q",pathValue)}
	outside:=filepath.Join(t.TempDir(),"outside.go")
	if _,err:=fileURIToProjectPath(root,pathToFileURI(outside));err==nil||!strings.Contains(err.Error(),"escapes workspace"){
		t.Fatalf("outside URI err=%v",err)
	}
}

func TestLSPLocationAndRenameEditNormalization(t *testing.T) {
	root:=t.TempDir()
	target:=filepath.Join(root,"pkg","item.go")
	if err:=os.MkdirAll(filepath.Dir(target),0o755);err!=nil{t.Fatal(err)}
	raw,_:=json.Marshal([]map[string]any{{
		"uri":pathToFileURI(target),
		"range":map[string]any{"start":map[string]int{"line":2,"character":3},"end":map[string]int{"line":2,"character":7}},
	}})
	locations,err:=normalizeLSPLocations(root,raw);if err!=nil{t.Fatal(err)}
	if len(locations)!=1||locations[0].Path!="pkg/item.go"||locations[0].Range.Start.Line!=2{t.Fatalf("locations=%+v",locations)}

	editRaw,_:=json.Marshal(map[string]any{"changes":map[string]any{
		pathToFileURI(target):[]map[string]any{{
			"range":map[string]any{"start":map[string]int{"line":1,"character":0},"end":map[string]int{"line":1,"character":3}},
			"newText":"renamed",
		}},
	}})
	edits,err:=normalizeLSPWorkspaceEdit(root,editRaw);if err!=nil{t.Fatal(err)}
	if len(edits)!=1||edits[0].Path!="pkg/item.go"||len(edits[0].Edits)!=1||edits[0].Edits[0].NewText!="renamed"{
		t.Fatalf("edits=%+v",edits)
	}
}

func TestLSPRenameRejectsFileOperationsAndInvalidRequests(t *testing.T) {
	root:=t.TempDir()
	raw,_:=json.Marshal(map[string]any{"documentChanges":[]map[string]any{{"kind":"rename","oldUri":"file:///a","newUri":"file:///b"}}})
	if _,err:=normalizeLSPWorkspaceEdit(root,raw);err==nil||!strings.Contains(err.Error(),"unsupported file create/rename/delete"){
		t.Fatalf("file operation err=%v",err)
	}
	if _,err:=normalizeLSPActionRequest(lspActionRequest{Action:"rename",Path:"main.go"});err==nil{
		t.Fatal("rename without new_name accepted")
	}
	if _,err:=normalizeLSPActionRequest(lspActionRequest{Action:"unknown",Path:"main.go"});err==nil{
		t.Fatal("unknown LSP action accepted")
	}
}

func TestLSPPublishedDiagnosticsAreDocumentScoped(t *testing.T) {
	params,_:=json.Marshal(map[string]any{
		"uri":"file:///workspace/main.go",
		"diagnostics":[]map[string]any{{
			"range":map[string]any{"start":map[string]int{"line":1,"character":2},"end":map[string]int{"line":1,"character":4}},
			"severity":1,"source":"fixture","message":"broken",
		}},
	})
	message:=lspWireMessage{Method:"textDocument/publishDiagnostics",Params:params}
	items:=decodePublishedDiagnostics(message,"file:///workspace/main.go")
	if len(items)!=1||items[0].Message!="broken"||items[0].Severity!=1{t.Fatalf("diagnostics=%+v",items)}
	if other:=decodePublishedDiagnostics(message,"file:///workspace/other.go");other!=nil{t.Fatalf("foreign diagnostics=%+v",other)}
}
