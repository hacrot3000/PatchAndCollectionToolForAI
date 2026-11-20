package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const remoteWorkspaceVersion = 1

type remoteWorkspaceProfile struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	SSHProfileID       string   `json:"ssh_profile_id"`
	TransferProfileID  string   `json:"transfer_profile_id"`
	RemoteRoot         string   `json:"remote_root"`
	DatabaseProfileIDs []string `json:"database_profile_ids,omitempty"`
}

type remoteWorkspaceStoreFile struct {
	Version    int                      `json:"version"`
	Workspaces []remoteWorkspaceProfile `json:"workspaces"`
}

var remoteWorkspaceStoreMu sync.Mutex

func remoteWorkspaceStorePath() (string,error) {
	base,err:=os.UserConfigDir()
	if err!=nil{return "",fmt.Errorf("resolve user config dir: %w",err)}
	return filepath.Join(base,"taskdeck","remote-workspaces.json"),nil
}

func normalizeRemoteRoot(value string) (string,error) {
	value=strings.TrimSpace(value)
	if value==""||len(value)>4096{return "",errors.New("remote workspace root is required and must not exceed 4096 bytes")}
	if strings.ContainsAny(value,"\x00\r\n"){return "",errors.New("remote workspace root contains control characters")}
	value=strings.ReplaceAll(value,"\\","/")
	if !strings.HasPrefix(value,"/"){return "",errors.New("remote workspace root must be an absolute POSIX path")}
	parts:=[]string{}
	for _,part:=range strings.Split(value,"/"){
		switch part {
		case "", ".":
		case "..":
			if len(parts)==0{return "",errors.New("remote workspace root escapes filesystem root")}
			parts=parts[:len(parts)-1]
		default:parts=append(parts,part)
		}
	}
	return "/"+strings.Join(parts,"/"),nil
}

func normalizeRemoteWorkspaceProfile(s *Server,p remoteWorkspaceProfile) (remoteWorkspaceProfile,error) {
	p.ID=strings.TrimSpace(p.ID);p.Name=strings.TrimSpace(p.Name)
	p.SSHProfileID=strings.TrimSpace(p.SSHProfileID);p.TransferProfileID=strings.TrimSpace(p.TransferProfileID)
	if p.ID==""{
		id,err:=sshprofile.NewID();if err!=nil{return p,err};p.ID=id
	}
	if !validAddonToken(p.ID){return p,errors.New("remote workspace id is invalid")}
	if p.Name==""||len(p.Name)>128||strings.ContainsAny(p.Name,"\x00\r\n"){return p,errors.New("remote workspace name is invalid")}
	if p.SSHProfileID==""||p.TransferProfileID==""{return p,errors.New("remote workspace requires SSH and SFTP profile ids")}
	root,err:=normalizeRemoteRoot(p.RemoteRoot);if err!=nil{return p,err};p.RemoteRoot=root

	sshStore,err:=s.sshProfileStore();if err!=nil{return p,err}
	if _,err=sshStore.Get(p.SSHProfileID);err!=nil{return p,fmt.Errorf("remote workspace SSH profile unavailable: %w",err)}
	transferStore,err:=s.fileTransferProfileStore();if err!=nil{return p,err}
	transfer,err:=transferStore.Get(p.TransferProfileID);if err!=nil{return p,fmt.Errorf("remote workspace SFTP profile unavailable: %w",err)}
	if transfer.Protocol!=filetransferprofile.ProtocolSFTP{return p,errors.New("remote workspace transfer profile must use SFTP")}
	if transfer.SSHProfileID!=p.SSHProfileID{return p,errors.New("remote workspace SFTP profile must reference the same SSH profile")}

	seen:=map[string]bool{};dbIDs:=make([]string,0,len(p.DatabaseProfileIDs))
	if len(p.DatabaseProfileIDs)>32{return p,errors.New("remote workspace database profiles exceed 32 entries")}
	dbStore,err:=s.databaseProfileStore();if err!=nil{return p,err}
	for _,id:=range p.DatabaseProfileIDs{
		id=strings.TrimSpace(id);if id==""||seen[id]{continue}
		db,err:=dbStore.Get(id);if err!=nil{return p,fmt.Errorf("remote workspace database profile %q unavailable: %w",id,err)}
		if db.Transport==dbprofile.TransportSSHTunnel&&db.SSHProfileID!=p.SSHProfileID{
			return p,fmt.Errorf("remote workspace database profile %q uses a different SSH tunnel profile",id)
		}
		seen[id]=true;dbIDs=append(dbIDs,id)
	}
	p.DatabaseProfileIDs=dbIDs
	return p,nil
}

func readRemoteWorkspaceProfiles() ([]remoteWorkspaceProfile,error) {
	pathValue,err:=remoteWorkspaceStorePath();if err!=nil{return nil,err}
	data,err:=os.ReadFile(pathValue)
	if errors.Is(err,os.ErrNotExist){return []remoteWorkspaceProfile{},nil}
	if err!=nil{return nil,err}
	var file remoteWorkspaceStoreFile
	decoder:=json.NewDecoder(strings.NewReader(string(data)));decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&file);err!=nil{return nil,fmt.Errorf("decode remote workspaces: %w",err)}
	if file.Version!=remoteWorkspaceVersion{return nil,fmt.Errorf("unsupported remote workspace version %d",file.Version)}
	sort.Slice(file.Workspaces,func(i,j int)bool{return strings.ToLower(file.Workspaces[i].Name)<strings.ToLower(file.Workspaces[j].Name)})
	return file.Workspaces,nil
}

func writeRemoteWorkspaceProfiles(items []remoteWorkspaceProfile) error {
	pathValue,err:=remoteWorkspaceStorePath();if err!=nil{return err}
	if err:=os.MkdirAll(filepath.Dir(pathValue),0o700);err!=nil{return err}
	data,err:=json.MarshalIndent(remoteWorkspaceStoreFile{Version:remoteWorkspaceVersion,Workspaces:items},"","  ");if err!=nil{return err}
	data=append(data,'\n')
	temp:=pathValue+".tmp"
	if err:=os.WriteFile(temp,data,0o600);err!=nil{return err}
	if err:=os.Chmod(temp,0o600);err!=nil{_ = os.Remove(temp);return err}
	if err:=os.Rename(temp,pathValue);err!=nil{_ = os.Remove(temp);return err}
	return os.Chmod(pathValue,0o600)
}

func findRemoteWorkspace(items []remoteWorkspaceProfile,id string) (int,remoteWorkspaceProfile,bool) {
	id=strings.TrimSpace(id)
	for i,item:=range items{if item.ID==id{return i,item,true}}
	return -1,remoteWorkspaceProfile{},false
}

func (s *Server) remoteWorkspacesAPI(w http.ResponseWriter,r *http.Request) {
	remoteWorkspaceStoreMu.Lock();defer remoteWorkspaceStoreMu.Unlock()
	items,err:=readRemoteWorkspaceProfiles();if err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w,http.StatusOK,map[string]any{"version":remoteWorkspaceVersion,"workspaces":items})
	case http.MethodPost:
		var req remoteWorkspaceProfile
		decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10));decoder.DisallowUnknownFields()
		if err:=decoder.Decode(&req);err!=nil{http.Error(w,"invalid remote workspace JSON",http.StatusBadRequest);return}
		req.ID=""
		req,err=normalizeRemoteWorkspaceProfile(s,req);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
		items=append(items,req)
		if err:=writeRemoteWorkspaceProfiles(items);err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
		s.auditSharedSuccess(r,"remote_workspace.create","remote_workspace",req.ID,nil)
		writeJSON(w,http.StatusCreated,req)
	default:http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
	}
}

func (s *Server) remoteWorkspaceItemAPI(w http.ResponseWriter,r *http.Request) {
	id:=strings.TrimSpace(strings.TrimPrefix(r.URL.Path,"/api/remote-workspaces/"))
	if strings.Contains(id,"/")||!validAddonToken(id){http.NotFound(w,r);return}
	remoteWorkspaceStoreMu.Lock();defer remoteWorkspaceStoreMu.Unlock()
	items,err:=readRemoteWorkspaceProfiles();if err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
	index,current,ok:=findRemoteWorkspace(items,id);if !ok{http.NotFound(w,r);return}
	switch r.Method {
	case http.MethodGet:writeJSON(w,http.StatusOK,current)
	case http.MethodPut:
		var req remoteWorkspaceProfile
		decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10));decoder.DisallowUnknownFields()
		if err:=decoder.Decode(&req);err!=nil{http.Error(w,"invalid remote workspace JSON",http.StatusBadRequest);return}
		req.ID=id
		req,err=normalizeRemoteWorkspaceProfile(s,req);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
		items[index]=req
		if err:=writeRemoteWorkspaceProfiles(items);err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
		s.auditSharedSuccess(r,"remote_workspace.update","remote_workspace",id,nil)
		writeJSON(w,http.StatusOK,req)
	case http.MethodDelete:
		items=append(items[:index],items[index+1:]...)
		if err:=writeRemoteWorkspaceProfiles(items);err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
		s.auditSharedSuccess(r,"remote_workspace.delete","remote_workspace",id,nil)
		w.WriteHeader(http.StatusNoContent)
	default:http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
	}
}
