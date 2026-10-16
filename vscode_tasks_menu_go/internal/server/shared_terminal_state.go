package server

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "reflect"
    "strings"

    "bletonfc/vscode_tasks_menu/internal/identity"
    "bletonfc/vscode_tasks_menu/internal/session"
    "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Shared-server terminal layout belongs to one authenticated user, not the
// project. Never reuse the project-global v1-v5 state: it contains other users'
// session IDs and can restore unowned shells. Keep this state outside the
// browsable workspace and use opaque names derived from authenticated IDs.
func (s *Server) sharedTerminalStatePath(principal identity.Principal, profile string) (string,error) {
    if principal.UserID == "" || principal.ProjectID == "" {
        return "",fmt.Errorf("authenticated user and project are required")
    }
    base:=strings.TrimSpace(s.Config.SharedIdentityDB)
    if base!="" {
        base=filepath.Dir(base)
    } else {
        var err error
        base,err=os.UserConfigDir()
        if err!=nil{return "",fmt.Errorf("resolve private TaskDeck data directory: %w",err)}
    }
    folder:=filepath.Join(base,"taskdeck-private-terminal-layouts")
    key:=sha256.Sum256([]byte(filepath.Clean(s.Workspace)+"\x00"+string(principal.ProjectID)+"\x00"+string(principal.UserID)))
    suffix:=".json"
    if normalizeTerminalLayoutProfile(profile)=="mobile"{suffix=".mobile.json"}
    return filepath.Join(folder,hex.EncodeToString(key[:])+suffix),nil
}

func validatePrivateTerminalStatePath(path string, create bool) error {
    dir:=filepath.Dir(path)
    if create {
        if err:=os.MkdirAll(dir,0700);err!=nil{return err}
    }
    info,err:=os.Lstat(dir)
    if errors.Is(err,os.ErrNotExist){return err}
    if err!=nil{return err}
    if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {
        return fmt.Errorf("private terminal state directory is not a directory")
    }
    info,err=os.Lstat(path)
    if errors.Is(err,os.ErrNotExist){return nil}
    if err!=nil{return err}
    if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink!=0 {
        return fmt.Errorf("private terminal state file must be regular")
    }
    return nil
}

func (s *Server) readSharedTerminalState(principal identity.Principal, profile string) (projectTerminalState,error) {
    empty:=defaultProjectTerminalState()
    path,err:=s.sharedTerminalStatePath(principal,profile)
    if err!=nil{return empty,err}
    if err=validatePrivateTerminalStatePath(path,false);err!=nil{
        if errors.Is(err,os.ErrNotExist){return empty,nil}
        return empty,err
    }
    data,err:=os.ReadFile(path)
    if errors.Is(err,os.ErrNotExist){return empty,nil}
    if err!=nil{return empty,err}
    if len(data)>projectTerminalStateMax{return empty,fmt.Errorf("private terminal layout exceeds size limit")}
    if err=json.Unmarshal(data,&empty);err!=nil{return defaultProjectTerminalState(),fmt.Errorf("decode private terminal layout: %w",err)}
    return normalizeProjectTerminalState(empty),nil
}

func (s *Server) writeSharedTerminalState(principal identity.Principal,profile string,value projectTerminalState) error {
    path,err:=s.sharedTerminalStatePath(principal,profile)
    if err!=nil{return err}
    if err=validatePrivateTerminalStatePath(path,true);err!=nil{return err}
    encoded,err:=json.Marshal(normalizeProjectTerminalState(value))
    if err!=nil{return err}
    if len(encoded)>projectTerminalStateMax{return fmt.Errorf("private terminal layout exceeds size limit")}
    tmp,err:=os.CreateTemp(filepath.Dir(path),".taskdeck-terminal-*.tmp")
    if err!=nil{return err}
    defer os.Remove(tmp.Name())
    if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
    if _,err=tmp.Write(encoded);err!=nil{tmp.Close();return err}
    if err=tmp.Sync();err!=nil{tmp.Close();return err}
    if err=tmp.Close();err!=nil{return err}
    if err=validatePrivateTerminalStatePath(path,true);err!=nil{return err}
    return os.Rename(tmp.Name(),path)
}

func (s *Server) captureSharedTerminalState(principal identity.Principal,req terminalSnapshotRequest)(projectTerminalState,error) {
    allowed:=make(map[string]bool,len(req.SessionIDs))
    for _,id:=range req.SessionIDs {
        id=strings.TrimSpace(id)
        if id==""{continue}
        meta,ok:=s.Sessions.Metadata(id)
        if !ok{continue} // A terminal closed while a pending save was queued.
        if meta.Kind!=tasks.SessionKindTerminal || meta.OwnerUserID!=string(principal.UserID) ||
            meta.ProjectID!=string(principal.ProjectID) {
            return projectTerminalState{},fmt.Errorf("terminal layout references a session not owned by this account")
        }
        allowed[id]=true
    }
    filtered:=terminalSnapshotRequest{ActiveSessionID:req.ActiveSessionID}
    for _,id:=range req.SessionIDs {if allowed[strings.TrimSpace(id)]{filtered.SessionIDs=append(filtered.SessionIDs,id)}}
    groups:=req.Splits
    if len(groups)==0&&req.Split!=nil {groups=[]terminalSnapshotSplitRequest{*req.Split}}
    for _,group:=range groups{
        if allowed[group.LeftSessionID]&&allowed[group.RightSessionID] {
            filtered.Splits=append(filtered.Splits,group)
        }
    }
    return s.captureTerminalState(filtered)
}

func (s *Server) sharedTerminalState(w http.ResponseWriter,r *http.Request,profile string) {
    if !s.sharedSessionOwnershipReady(w){return}
    principal,ok:=PrincipalFromContext(r.Context())
    if !ok{sharedAuthError(w,identity.ErrUnauthenticated);return}
    view:=principal.Allowed(identity.PermissionTerminalViewOwn)||
        principal.Allowed(identity.PermissionTerminalViewAll)||
        principal.Allowed(identity.PermissionSessionsManage)
    if !view{writePermissionDenied(w);return}
    switch r.Method{
    case http.MethodGet:
        value,err:=s.readSharedTerminalState(principal,profile)
        if err!=nil{http.Error(w,"read terminal layout failed",http.StatusInternalServerError);return}
        writeJSON(w,http.StatusOK,value)
    case http.MethodPut:
        if !principal.Allowed(identity.PermissionTerminalControlOwn)&&
            !principal.Allowed(identity.PermissionTerminalControlAll)&&
            !principal.Allowed(identity.PermissionSessionsManage){
            writePermissionDenied(w);return
        }
        if s.terminalStatePersistenceFrozen(){
            http.Error(w,"terminal state persistence is frozen during daemon shutdown",http.StatusServiceUnavailable);return
        }
        var req terminalSnapshotRequest
        dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,projectTerminalStateMax))
        dec.DisallowUnknownFields()
        if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid JSON",http.StatusBadRequest);return}
        value,err:=s.captureSharedTerminalState(principal,req)
        if err!=nil{http.Error(w,err.Error(),http.StatusForbidden);return}
        previous,err:=s.readSharedTerminalState(principal,profile)
        if err!=nil{http.Error(w,"read terminal layout failed",http.StatusInternalServerError);return}
        if profile=="mobile"{value.Splits=nil;value.LiveSplits=nil;value.Split=nil}
        if !reflect.DeepEqual(previous,value) {
            if err=s.writeSharedTerminalState(principal,profile,value);err!=nil{
                http.Error(w,"save terminal layout failed",http.StatusInternalServerError);return
            }
        }
        writeJSON(w,http.StatusOK,value)
    case http.MethodPost:
        value,err:=s.readSharedTerminalState(principal,profile)
        if err!=nil{http.Error(w,"read terminal layout failed",http.StatusInternalServerError);return}
        if len(value.Terminals)==0{
            writeJSON(w,http.StatusCreated,terminalRestoreResponse{Sessions:[]session.Metadata{}})
            return
        }
        if !principal.Allowed(identity.PermissionTerminalCreate){
            writePermissionDenied(w);return
        }
        resp,err:=s.restoreSharedTerminalState(principal,profile,value)
        if err!=nil{http.Error(w,"restore owned terminal layout failed: "+err.Error(),http.StatusConflict);return}
        writeJSON(w,http.StatusCreated,resp)
    default:
        http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
    }
}

// Only local, workspace-bound shells may be recreated in shared mode. SSH
// terminals are managed by explicit profile opening; never reinterpret an SSH
// remote cwd as a host path. The live broker already preserves sessions across
// normal daemon/browser reconnects.
func (s *Server) restoreSharedTerminalState(principal identity.Principal,profile string,value projectTerminalState)(terminalRestoreResponse,error) {
    resp:=terminalRestoreResponse{Sessions:[]session.Metadata{},ActiveIndex:value.ActiveIndex,Splits:value.Splits}
    for _,meta:=range s.Sessions.List(){
        if meta.Kind==tasks.SessionKindTerminal&&meta.ProjectID==string(principal.ProjectID)&&
            meta.OwnerUserID==string(principal.UserID)&&meta.Status=="running"{
            return resp,fmt.Errorf("account already has running terminals")
        }
    }
    remap:=make(map[string]string)
    started:=[]string{}
    for _,saved:=range value.Terminals{
        cwd:=strings.TrimSpace(saved.Cwd)
        relative,err:=filepath.Rel(s.Workspace,cwd)
        if err!=nil||relative==".."||strings.HasPrefix(relative,".."+string(filepath.Separator)){
            resp.Warnings=append(resp.Warnings,"saved terminal outside project workspace was skipped")
            continue
        }
        spec,err:=s.workspaceTerminalExecutionAtProjectPath(relative)
        if err!=nil{
            resp.Warnings=append(resp.Warnings,"saved terminal directory is unavailable; skipped")
            continue
        }
        stampSharedSession(principal,&spec,tasks.SessionKindTerminal)
        meta,err:=s.Sessions.Start(spec)
        if err!=nil{
            for _,id:=range started{_ = s.Sessions.Stop(id)}
            return terminalRestoreResponse{},fmt.Errorf("start terminal: %w",err)
        }
        started=append(started,meta.ID)
        if saved.SessionID!=""{remap[saved.SessionID]=meta.ID}
        if saved.Title!=""{
            if titled,titleErr:=s.Sessions.SetTitle(meta.ID,saved.Title);titleErr==nil{meta=titled}
        }
        resp.Sessions=append(resp.Sessions,meta)
    }
    for _,group:=range value.LiveSplits{
        left,right:=remap[group.LeftSessionID],remap[group.RightSessionID]
        if left!=""&&right!=""&&left!=right{
            resp.LiveSplits=append(resp.LiveSplits,terminalSnapshotSplitRequest{
                LeftSessionID:left,RightSessionID:right,Ratio:group.Ratio,Orientation:group.Orientation})
        }
    }
    if len(started)>0{
        updated:=value
        updated.Terminals=nil
        for _,meta:=range resp.Sessions{
            updated.Terminals=append(updated.Terminals,terminalStateItem{SessionID:meta.ID,Cwd:meta.Cwd,Title:meta.Title})
        }
        updated.LiveSplits=resp.LiveSplits
        if err:=s.writeSharedTerminalState(principal,profile,updated);err!=nil{
            for _,id:=range started{_ = s.Sessions.Stop(id)}
            return terminalRestoreResponse{},fmt.Errorf("persist restored layout: %w",err)
        }
    }
    return resp,nil
}
