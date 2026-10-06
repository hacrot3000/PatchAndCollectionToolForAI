package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

const (
	lspMessageMaxBytes = 8 << 20
	lspActionTimeout   = 15 * time.Second
)

type lspServerSpec struct {
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	LanguageID string   `json:"language_id"`
	Extensions []string `json:"extensions"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspActionRequest struct {
	Action    string      `json:"action"`
	Path      string      `json:"path"`
	Text      string      `json:"text,omitempty"`
	Position  lspPosition `json:"position,omitempty"`
	NewName   string      `json:"new_name,omitempty"`
}

type lspLocation struct {
	Path  string   `json:"path"`
	Range lspRange `json:"range"`
}

type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"new_text"`
}

type lspFileEdits struct {
	Path  string        `json:"path"`
	Edits []lspTextEdit `json:"edits"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Code     any      `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type lspActionResponse struct {
	Server      string          `json:"server"`
	LanguageID  string          `json:"language_id"`
	Hover       string          `json:"hover,omitempty"`
	Locations   []lspLocation   `json:"locations,omitempty"`
	Edits       []lspFileEdits  `json:"edits,omitempty"`
	Diagnostics []lspDiagnostic `json:"diagnostics,omitempty"`
}

type lspWireMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type lspClient struct {
	stdin io.Writer
	msgs  <-chan lspWireMessage
	mu    sync.Mutex
	next  int
}

func lspSpecs() []lspServerSpec {
	return []lspServerSpec{
		{Name:"gopls",Command:"gopls",LanguageID:"go",Extensions:[]string{".go"}},
		{Name:"clangd",Command:"clangd",LanguageID:"cpp",Extensions:[]string{".c",".h",".cc",".cpp",".cxx",".hpp",".hh",".hxx"}},
		{Name:"pylsp",Command:"pylsp",LanguageID:"python",Extensions:[]string{".py",".pyi"}},
		{Name:"typescript-language-server",Command:"typescript-language-server",Args:[]string{"--stdio"},LanguageID:"typescript",Extensions:[]string{".ts",".tsx",".js",".jsx",".mjs",".cjs"}},
	}
}

func lspSpecForPath(pathValue string) (lspServerSpec, bool) {
	ext:=strings.ToLower(filepath.Ext(strings.TrimSpace(pathValue)))
	for _,spec:=range lspSpecs(){
		for _,candidate:=range spec.Extensions {
			if ext==candidate {
				if ext==".js"||ext==".jsx"||ext==".mjs"||ext==".cjs" { spec.LanguageID="javascript" }
				return spec,true
			}
		}
	}
	return lspServerSpec{},false
}

func lspAvailable(spec lspServerSpec) bool {
	_,err:=exec.LookPath(spec.Command)
	return err==nil
}

func pathToFileURI(pathValue string) string {
	return (&url.URL{Scheme:"file",Path:filepath.ToSlash(pathValue)}).String()
}

func fileURIToProjectPath(workspace, uri string) (string,error) {
	parsed,err:=url.Parse(strings.TrimSpace(uri))
	if err!=nil||parsed.Scheme!="file" { return "",errors.New("LSP location is not a file URI") }
	pathValue:=filepath.Clean(filepath.FromSlash(parsed.Path))
	root,err:=filepath.Abs(workspace);if err!=nil{return "",err}
	target,err:=filepath.Abs(pathValue);if err!=nil{return "",err}
	rel,err:=filepath.Rel(root,target);if err!=nil{return "",err}
	if rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator))||filepath.IsAbs(rel){
		return "",errors.New("LSP location escapes workspace")
	}
	return filepath.ToSlash(rel),nil
}

func writeLSPMessage(w io.Writer,value any) error {
	data,err:=json.Marshal(value);if err!=nil{return err}
	if len(data)>lspMessageMaxBytes{return errors.New("LSP message exceeds size limit")}
	_,err=fmt.Fprintf(w,"Content-Length: %d\r\n\r\n",len(data));if err!=nil{return err}
	_,err=w.Write(data);return err
}

func readLSPMessage(reader *bufio.Reader) (lspWireMessage,error) {
	var length int
	for {
		line,err:=reader.ReadString('\n');if err!=nil{return lspWireMessage{},err}
		line=strings.TrimRight(line,"\r\n")
		if line=="" { break }
		name,value,ok:=strings.Cut(line,":");if !ok{continue}
		if strings.EqualFold(strings.TrimSpace(name),"Content-Length"){
			n,err:=strconv.Atoi(strings.TrimSpace(value));if err!=nil{return lspWireMessage{},err}
			length=n
		}
	}
	if length<=0||length>lspMessageMaxBytes{return lspWireMessage{},errors.New("invalid LSP Content-Length")}
	data:=make([]byte,length);if _,err:=io.ReadFull(reader,data);err!=nil{return lspWireMessage{},err}
	var message lspWireMessage
	if err:=json.Unmarshal(data,&message);err!=nil{return lspWireMessage{},err}
	return message,nil
}

func streamLSPMessages(reader io.Reader,out chan<- lspWireMessage,errs chan<- error) {
	defer close(out)
	buffer:=bufio.NewReader(reader)
	for {
		message,err:=readLSPMessage(buffer)
		if err!=nil { if !errors.Is(err,io.EOF){select{case errs<-err:default:}};return }
		out<-message
	}
}

func (c *lspClient) send(value any) error {
	c.mu.Lock();defer c.mu.Unlock()
	return writeLSPMessage(c.stdin,value)
}

func (c *lspClient) request(method string,params any) (int,error) {
	c.next++;id:=c.next
	err:=c.send(map[string]any{"jsonrpc":"2.0","id":id,"method":method,"params":params})
	return id,err
}

func (c *lspClient) notify(method string,params any) error {
	return c.send(map[string]any{"jsonrpc":"2.0","method":method,"params":params})
}

func rawIDNumber(raw json.RawMessage) (int,bool) {
	if len(raw)==0{return 0,false}
	var id int
	if err:=json.Unmarshal(raw,&id);err!=nil{return 0,false}
	return id,true
}

func (c *lspClient) answerServerRequest(message lspWireMessage) {
	id,ok:=rawIDNumber(message.ID);if !ok{return}
	var result any=nil
	switch message.Method {
	case "workspace/configuration":
		result=[]any{}
	case "window/workDoneProgress/create","client/registerCapability","client/unregisterCapability":
		result=nil
	}
	_ = c.send(map[string]any{"jsonrpc":"2.0","id":id,"result":result})
}

func decodePublishedDiagnostics(message lspWireMessage, uri string) []lspDiagnostic {
	if message.Method!="textDocument/publishDiagnostics" { return nil }
	var payload struct {
		URI string `json:"uri"`
		Diagnostics []struct {
			Range lspRange `json:"range"`
			Severity int `json:"severity,omitempty"`
			Code any `json:"code,omitempty"`
			Source string `json:"source,omitempty"`
			Message string `json:"message"`
		} `json:"diagnostics"`
	}
	if json.Unmarshal(message.Params,&payload)!=nil||payload.URI!=uri{return nil}
	out:=make([]lspDiagnostic,0,len(payload.Diagnostics))
	for _,item:=range payload.Diagnostics {
		out=append(out,lspDiagnostic{Range:item.Range,Severity:item.Severity,Code:item.Code,Source:item.Source,Message:item.Message})
	}
	return out
}

func (c *lspClient) waitResponse(ctx context.Context,id int,uri string,diagnostics *[]lspDiagnostic,errs <-chan error) (json.RawMessage,error) {
	for {
		select {
		case <-ctx.Done(): return nil,ctx.Err()
		case err:=<-errs:
			if err!=nil{return nil,err}
		case message,ok:=<-c.msgs:
			if !ok{return nil,io.EOF}
			if message.Method!=""&&len(message.ID)>0 { c.answerServerRequest(message);continue }
			if published:=decodePublishedDiagnostics(message,uri);published!=nil&&diagnostics!=nil { *diagnostics=published;continue }
			if messageID,ok:=rawIDNumber(message.ID);ok&&messageID==id {
				if message.Error!=nil{return nil,fmt.Errorf("LSP %s (%d)",message.Error.Message,message.Error.Code)}
				return message.Result,nil
			}
		}
	}
}

func lspInitializeParams(workspace string) map[string]any {
	rootURI:=pathToFileURI(workspace)
	return map[string]any{
		"processId":nil,
		"rootUri":rootURI,
		"workspaceFolders":[]map[string]any{{"uri":rootURI,"name":filepath.Base(workspace)}},
		"capabilities":map[string]any{
			"workspace":map[string]any{"workspaceEdit":map[string]any{"documentChanges":true}},
			"textDocument":map[string]any{
				"hover":map[string]any{},
				"definition":map[string]any{"linkSupport":true},
				"references":map[string]any{},
				"rename":map[string]any{"prepareSupport":false},
				"publishDiagnostics":map[string]any{},
			},
		},
	}
}

func normalizeLSPLocations(workspace string,raw json.RawMessage) ([]lspLocation,error) {
	if len(raw)==0||string(raw)=="null"{return []lspLocation{},nil}
	var items []json.RawMessage
	if raw[0]=='[' { if err:=json.Unmarshal(raw,&items);err!=nil{return nil,err} } else { items=[]json.RawMessage{raw} }
	out:=make([]lspLocation,0,len(items))
	for _,item:=range items {
		var value struct {
			URI string `json:"uri"`
			Range lspRange `json:"range"`
			TargetURI string `json:"targetUri"`
			TargetSelectionRange lspRange `json:"targetSelectionRange"`
		}
		if err:=json.Unmarshal(item,&value);err!=nil{return nil,err}
		uri:=value.URI;rng:=value.Range
		if uri=="" { uri=value.TargetURI;rng=value.TargetSelectionRange }
		pathValue,err:=fileURIToProjectPath(workspace,uri);if err!=nil{continue}
		out=append(out,lspLocation{Path:pathValue,Range:rng})
	}
	return out,nil
}

func normalizeLSPWorkspaceEdit(workspace string,raw json.RawMessage) ([]lspFileEdits,error) {
	if len(raw)==0||string(raw)=="null"{return []lspFileEdits{},nil}
	var value struct {
		Changes map[string][]struct{Range lspRange `json:"range"`;NewText string `json:"newText"`} `json:"changes"`
		DocumentChanges []struct{
			TextDocument struct{URI string `json:"uri"`} `json:"textDocument"`
			Edits []struct{Range lspRange `json:"range"`;NewText string `json:"newText"`} `json:"edits"`
			Kind string `json:"kind"`
		} `json:"documentChanges"`
	}
	if err:=json.Unmarshal(raw,&value);err!=nil{return nil,err}
	byPath:=map[string][]lspTextEdit{}
	add:=func(uri string,items []struct{Range lspRange `json:"range"`;NewText string `json:"newText"`}) error {
		pathValue,err:=fileURIToProjectPath(workspace,uri);if err!=nil{return err}
		for _,item:=range items { byPath[pathValue]=append(byPath[pathValue],lspTextEdit{Range:item.Range,NewText:item.NewText}) }
		return nil
	}
	for uri,items:=range value.Changes { if err:=add(uri,items);err!=nil{return nil,err} }
	for _,change:=range value.DocumentChanges {
		if change.Kind!="" { return nil,errors.New("LSP rename returned unsupported file create/rename/delete operation") }
		if err:=add(change.TextDocument.URI,change.Edits);err!=nil{return nil,err}
	}
	out:=make([]lspFileEdits,0,len(byPath))
	for pathValue,edits:=range byPath { out=append(out,lspFileEdits{Path:pathValue,Edits:edits}) }
	return out,nil
}

func hoverText(raw json.RawMessage) string {
	if len(raw)==0||string(raw)=="null"{return ""}
	var value struct{ Contents any `json:"contents"` }
	if json.Unmarshal(raw,&value)!=nil{return ""}
	switch content:=value.Contents.(type) {
	case string:return content
	case map[string]any:
		if v,ok:=content["value"].(string);ok{return v}
	case []any:
		parts:=[]string{}
		for _,item:=range content {
			switch v:=item.(type) {
			case string: parts=append(parts,v)
			case map[string]any:
				if text,ok:=v["value"].(string);ok{parts=append(parts,text)}
			}
		}
		return strings.Join(parts,"\n\n")
	}
	data,_:=json.Marshal(value.Contents);return string(data)
}

func runLSPAction(ctx context.Context,s *Server,spec lspServerSpec,req lspActionRequest,resolved string) (lspActionResponse,error) {
	executable,err:=exec.LookPath(spec.Command);if err!=nil{return lspActionResponse{},fmt.Errorf("%s is not installed or not on PATH",spec.Command)}
	ctx,cancel:=context.WithTimeout(ctx,lspActionTimeout);defer cancel()
	cmd:=exec.CommandContext(ctx,executable,spec.Args...);cmd.Dir=s.Workspace
	stdin,err:=cmd.StdinPipe();if err!=nil{return lspActionResponse{},err}
	stdout,err:=cmd.StdoutPipe();if err!=nil{return lspActionResponse{},err}
	var stderr bytes.Buffer;cmd.Stderr=&limitedWriter{Writer:&stderr,Remaining:256<<10}
	if err:=cmd.Start();err!=nil{return lspActionResponse{},err}
	defer func(){_ = cmd.Process.Kill();_ = cmd.Wait()}()

	msgs:=make(chan lspWireMessage,32);errs:=make(chan error,1);go streamLSPMessages(stdout,msgs,errs)
	client:=&lspClient{stdin:stdin,msgs:msgs}
	uri:=pathToFileURI(resolved);diagnostics:=[]lspDiagnostic{}
	initID,err:=client.request("initialize",lspInitializeParams(s.Workspace));if err!=nil{return lspActionResponse{},err}
	if _,err=client.waitResponse(ctx,initID,uri,&diagnostics,errs);err!=nil{return lspActionResponse{},err}
	if err=client.notify("initialized",map[string]any{});err!=nil{return lspActionResponse{},err}
	if err=client.notify("textDocument/didOpen",map[string]any{
		"textDocument":map[string]any{"uri":uri,"languageId":spec.LanguageID,"version":1,"text":req.Text},
	});err!=nil{return lspActionResponse{},err}

	response:=lspActionResponse{Server:spec.Name,LanguageID:spec.LanguageID}
	positionParams:=map[string]any{"textDocument":map[string]any{"uri":uri},"position":req.Position}
	var requestID int
	switch req.Action {
	case "hover":
		requestID,err=client.request("textDocument/hover",positionParams)
	case "definition":
		requestID,err=client.request("textDocument/definition",positionParams)
	case "references":
		positionParams["context"]=map[string]any{"includeDeclaration":true}
		requestID,err=client.request("textDocument/references",positionParams)
	case "rename":
		params:=map[string]any{"textDocument":map[string]any{"uri":uri},"position":req.Position,"newName":req.NewName}
		requestID,err=client.request("textDocument/rename",params)
	case "diagnostics":
		requestID,err=client.request("textDocument/documentSymbol",map[string]any{"textDocument":map[string]any{"uri":uri}})
	default:
		return response,errors.New("unsupported LSP action")
	}
	if err!=nil{return response,err}
	raw,err:=client.waitResponse(ctx,requestID,uri,&diagnostics,errs);if err!=nil{return response,err}
	switch req.Action {
	case "hover": response.Hover=hoverText(raw)
	case "definition","references":
		response.Locations,err=normalizeLSPLocations(s.Workspace,raw)
	case "rename":
		response.Edits,err=normalizeLSPWorkspaceEdit(s.Workspace,raw)
	case "diagnostics":
		timer:=time.NewTimer(300*time.Millisecond);defer timer.Stop()
		for {
			select {
			case message,ok:=<-msgs:
				if !ok{goto doneDiagnostics}
				if message.Method!=""&&len(message.ID)>0 {client.answerServerRequest(message);continue}
				if published:=decodePublishedDiagnostics(message,uri);published!=nil{diagnostics=published}
			case <-timer.C: goto doneDiagnostics
			case <-ctx.Done(): goto doneDiagnostics
			}
		}
	doneDiagnostics:
	}
	response.Diagnostics=diagnostics
	if err!=nil{return response,err}
	shutdownID,shutdownErr:=client.request("shutdown",nil)
	if shutdownErr==nil {_,_ = client.waitResponse(context.Background(),shutdownID,uri,nil,errs)}
	_ = client.notify("exit",nil)
	return response,nil
}

func normalizeLSPActionRequest(req lspActionRequest) (lspActionRequest,error) {
	req.Action=strings.ToLower(strings.TrimSpace(req.Action))
	req.Path=strings.TrimSpace(req.Path)
	req.NewName=strings.TrimSpace(req.NewName)
	switch req.Action {
	case "diagnostics","hover","definition","references":
	case "rename":
		if req.NewName==""||len(req.NewName)>256{return req,errors.New("rename requires a non-empty new_name")}
	default:return req,errors.New("unsupported LSP action")
	}
	if req.Path==""{return req,errors.New("LSP path is required")}
	if len(req.Text)>4<<20{return req,errors.New("LSP document exceeds 4 MiB")}
	if req.Position.Line<0||req.Position.Character<0{return req,errors.New("LSP position must be non-negative")}
	return req,nil
}

func (s *Server) lspAPI(w http.ResponseWriter,r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pathValue:=strings.TrimSpace(r.URL.Query().Get("path"))
		spec,ok:=lspSpecForPath(pathValue)
		if !ok { writeJSON(w,http.StatusOK,map[string]any{"supported":false});return }
		writeJSON(w,http.StatusOK,map[string]any{"supported":true,"available":lspAvailable(spec),"server":spec.Name,"language_id":spec.LanguageID,"extensions":spec.Extensions})
	case http.MethodPost:
		var req lspActionRequest
		decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,5<<20));decoder.DisallowUnknownFields()
		if err:=decoder.Decode(&req);err!=nil{http.Error(w,"invalid LSP request",http.StatusBadRequest);return}
		req,err:=normalizeLSPActionRequest(req);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
		if req.Action=="rename"&&!s.requireSharedActionPermission(w,r,identity.PermissionFilesWrite,"lsp.rename","file:"+req.Path){return}
		rel,err:=cleanProjectRelativePath(req.Path,false);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
		resolved,err:=s.resolveProjectPath(rel,false,false);if err!=nil{http.Error(w,"project file unavailable",http.StatusNotFound);return}
		spec,ok:=lspSpecForPath(rel);if !ok{http.Error(w,"no TaskDeck LSP mapping for this file type",http.StatusUnsupportedMediaType);return}
		result,err:=runLSPAction(r.Context(),s,spec,req,resolved)
		if err!=nil{http.Error(w,err.Error(),http.StatusBadGateway);return}
		writeJSON(w,http.StatusOK,result)
	default:
		http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
	}
}
