package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

const (
	taskRunHistoryFile = "vscode_tasks_menu.task_runs.json"
	taskRunHistoryDir = "vscode_tasks_menu.task_runs"
	maxTaskRunHistory = 50
	maxTaskRunLogBytes = 1 << 20
	maxTaskRunHistoryBytes = 256 << 10
)

type taskRunRecord struct {
	ID string `json:"id"`
	SessionID string `json:"session_id"`
	TaskID int `json:"task_id"`
	Label string `json:"label"`
	Status string `json:"status"`
	ExitCode *int `json:"exit_code,omitempty"`
	StartedAt string `json:"started_at"`
	EndedAt string `json:"ended_at"`
	Duration int64 `json:"duration"`
	GitCommit string `json:"git_commit,omitempty"`
	Cwd string `json:"cwd,omitempty"`
	CommandPreview string `json:"command_preview,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	TargetProfileID string `json:"target_profile_id,omitempty"`
	ProjectProfileID string `json:"project_profile_id,omitempty"`
	OwnerUserID string `json:"owner_user_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	LogBytes int `json:"log_bytes,omitempty"`
	LogTruncated bool `json:"log_truncated,omitempty"`
}

type taskRunHistoryFileData struct {
	Version int `json:"version"`
	Runs []taskRunRecord `json:"runs"`
}

type taskRunRecordRequest struct {
	SessionID string `json:"session_id"`
	Log string `json:"log"`
	ProjectProfileID string `json:"project_profile_id,omitempty"`
}

func taskRunHistoryPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, taskRunHistoryFile)
}

func taskRunLogsDir(workspace string) (string, error) {
	if err := projectfiles.EnsureDir(workspace); err != nil {
		return "", err
	}
	dir := filepath.Join(projectfiles.Dir(workspace), taskRunHistoryDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create task-run log directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func normalizeTaskRunRecord(item taskRunRecord) taskRunRecord {
	item.ID = trimStateString(item.ID, 160)
	item.SessionID = trimStateString(item.SessionID, 160)
	item.Label = trimStateString(item.Label, 512)
	item.Status = trimStateString(item.Status, 32)
	item.StartedAt = trimStateString(item.StartedAt, 80)
	item.EndedAt = trimStateString(item.EndedAt, 80)
	item.GitCommit = trimStateString(item.GitCommit, 80)
	item.Cwd = trimStateString(item.Cwd, 4096)
	item.CommandPreview = trimStateString(item.CommandPreview, 4096)
	item.TargetType = trimStateString(item.TargetType, 64)
	item.TargetProfileID = trimStateString(item.TargetProfileID, 160)
	item.ProjectProfileID = trimStateString(item.ProjectProfileID, 160)
	item.OwnerUserID = trimStateString(item.OwnerUserID, 160)
	item.ProjectID = trimStateString(item.ProjectID, 160)
	if item.Duration < 0 { item.Duration = 0 }
	if item.LogBytes < 0 { item.LogBytes = 0 }
	return item
}

func readTaskRunHistory(workspace string) (taskRunHistoryFileData, error) {
	result := taskRunHistoryFileData{Version:1, Runs:[]taskRunRecord{}}
	path, err := taskRunHistoryPath(workspace)
	if err != nil { return result, err }
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) { return result, nil }
	if err != nil { return result, err }
	if len(data) > maxTaskRunHistoryBytes { return result, errors.New("task-run history file is too large") }
	if len(strings.TrimSpace(string(data))) == 0 { return result, nil }
	if err := json.Unmarshal(data,&result); err != nil { return taskRunHistoryFileData{Version:1,Runs:[]taskRunRecord{}},err }
	result.Version=1
	if len(result.Runs)>maxTaskRunHistory { result.Runs=result.Runs[:maxTaskRunHistory] }
	out:=make([]taskRunRecord,0,len(result.Runs));seen:=map[string]bool{}
	for _,item:=range result.Runs {
		item=normalizeTaskRunRecord(item)
		if item.ID==""||item.SessionID==""||item.TaskID<=0||seen[item.ID] { continue }
		seen[item.ID]=true;out=append(out,item)
	}
	result.Runs=out
	return result,nil
}

func writeTaskRunHistory(workspace string, data taskRunHistoryFileData) error {
	data.Version=1
	if len(data.Runs)>maxTaskRunHistory { data.Runs=data.Runs[:maxTaskRunHistory] }
	encoded,err:=json.MarshalIndent(data,"","  ");if err!=nil{return err}
	encoded=append(encoded,'\n')
	if len(encoded)>maxTaskRunHistoryBytes{return errors.New("task-run history exceeds size limit")}
	target,err:=taskRunHistoryPath(workspace);if err!=nil{return err}
	tmp,err:=os.CreateTemp(filepath.Dir(target),".task-runs-*.tmp");if err!=nil{return err}
	name:=tmp.Name();defer os.Remove(name)
	if err:=tmp.Chmod(0o600);err!=nil{tmp.Close();return err}
	if _,err:=tmp.Write(encoded);err!=nil{tmp.Close();return err}
	if err:=tmp.Sync();err!=nil{tmp.Close();return err}
	if err:=tmp.Close();err!=nil{return err}
	if err:=os.Rename(name,target);err!=nil{return err}
	return os.Chmod(target,0o600)
}

func taskRunLogPath(workspace,id string)(string,error){
	if id==""||filepath.Base(id)!=id||strings.ContainsAny(id,"/\\\x00\r\n"){return "",errors.New("invalid task-run id")}
	dir,err:=taskRunLogsDir(workspace);if err!=nil{return "",err}
	return filepath.Join(dir,id+".log"),nil
}

func taskRunDuration(meta session.Metadata) int64 {
	start,err:=time.Parse(time.RFC3339,meta.StartedAt);if err!=nil{return 0}
	end,err:=time.Parse(time.RFC3339,meta.EndedAt);if err!=nil{return 0}
	if end.Before(start){return 0}
	return int64(end.Sub(start)/time.Second)
}

func taskRunGitCommit(ctx context.Context,s *Server,cwd string) string {
	cwd=strings.TrimSpace(cwd);if cwd==""{return ""}
	out,_,_,err:=runGitInDirectory(ctx,4*time.Second,cwd,"rev-parse","--verify","HEAD^{commit}")
	if err!=nil{return ""}
	return strings.TrimSpace(out)
}

func taskRunVisible(r *http.Request,item taskRunRecord) bool {
	principal,ok:=PrincipalFromContext(r.Context())
	if !ok { return true }
	meta:=session.Metadata{Kind:tasks.SessionKindTask,ProjectID:item.ProjectID,OwnerUserID:item.OwnerUserID}
	return sharedSessionVisible(principal,meta)
}

func taskRunID(sessionID string) string {
	value:=strings.NewReplacer("-","","_","").Replace(sessionID)
	if len(value)>48{value=value[:48]}
	return "run-"+value
}

func (s *Server) recordTaskRun(r *http.Request,req taskRunRecordRequest)(taskRunRecord,error){
	if s.Sessions==nil{return taskRunRecord{},errors.New("session service unavailable")}
	meta,ok:=s.Sessions.Metadata(strings.TrimSpace(req.SessionID));if !ok{return taskRunRecord{},errors.New("task session not found")}
	if meta.Kind!=tasks.SessionKindTask||meta.TaskID<=0{return taskRunRecord{},errors.New("session is not a task run")}
	if meta.Status=="running"||strings.TrimSpace(meta.EndedAt)==""{return taskRunRecord{},errors.New("task run is still active")}
	if principal,has:=PrincipalFromContext(r.Context());has&&!sharedSessionVisible(principal,meta){return taskRunRecord{},errors.New("task run is not visible")}

	s.taskRunHistoryMu.Lock()
	defer s.taskRunHistoryMu.Unlock()

	log:=req.Log;truncated:=false
	if len(log)>maxTaskRunLogBytes{log=log[:maxTaskRunLogBytes];truncated=true}
	id:=taskRunID(meta.ID)
	logPath,err:=taskRunLogPath(s.Workspace,id);if err!=nil{return taskRunRecord{},err}
	tmp,err:=os.CreateTemp(filepath.Dir(logPath),".task-run-log-*.tmp");if err!=nil{return taskRunRecord{},err}
	tmpName:=tmp.Name();defer os.Remove(tmpName)
	if err:=tmp.Chmod(0o600);err!=nil{tmp.Close();return taskRunRecord{},err}
	if _,err:=tmp.WriteString(log);err!=nil{tmp.Close();return taskRunRecord{},err}
	if err:=tmp.Sync();err!=nil{tmp.Close();return taskRunRecord{},err}
	if err:=tmp.Close();err!=nil{return taskRunRecord{},err}
	if err:=os.Rename(tmpName,logPath);err!=nil{return taskRunRecord{},err}

	item:=normalizeTaskRunRecord(taskRunRecord{
		ID:id,SessionID:meta.ID,TaskID:meta.TaskID,Label:meta.Label,Status:meta.Status,ExitCode:meta.ExitCode,
		StartedAt:meta.StartedAt,EndedAt:meta.EndedAt,Duration:taskRunDuration(meta),
		GitCommit:taskRunGitCommit(r.Context(),s,meta.Cwd),Cwd:meta.Cwd,CommandPreview:meta.CommandPreview,
		TargetType:meta.TargetType,TargetProfileID:meta.TargetProfileID,ProjectProfileID:req.ProjectProfileID,
		OwnerUserID:meta.OwnerUserID,ProjectID:meta.ProjectID,LogBytes:len(log),LogTruncated:truncated,
	})
	history,err:=readTaskRunHistory(s.Workspace);if err!=nil{return taskRunRecord{},err}
	next:=[]taskRunRecord{item}
	for _,old:=range history.Runs{if old.ID!=item.ID{next=append(next,old)};if len(next)>=maxTaskRunHistory{break}}
	history.Runs=next
	if err:=writeTaskRunHistory(s.Workspace,history);err!=nil{return taskRunRecord{},err}
	keep:=map[string]bool{};for _,run:=range next{keep[run.ID]=true}
	if dir,err:=taskRunLogsDir(s.Workspace);err==nil{
		if entries,readErr:=os.ReadDir(dir);readErr==nil{for _,entry:=range entries{
			if entry.IsDir()||!strings.HasSuffix(entry.Name(),".log"){continue}
			id:=strings.TrimSuffix(entry.Name(),".log");if !keep[id]{_ = os.Remove(filepath.Join(dir,entry.Name()))}
		}}
	}
	return item,nil
}


func (s *Server) taskRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.taskRunHistoryMu.Lock()
		history, err := readTaskRunHistory(s.Workspace)
		s.taskRunHistoryMu.Unlock()
		if err != nil {
			http.Error(w, "cannot read task-run history", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id != "" {
			for _, item := range history.Runs {
				if item.ID == id && taskRunVisible(r, item) {
					writeJSON(w, http.StatusOK, item)
					return
				}
			}
			http.Error(w, "task run not found", http.StatusNotFound)
			return
		}
		out := make([]taskRunRecord, 0, len(history.Runs))
		for _, item := range history.Runs {
			if taskRunVisible(r, item) {
				out = append(out, item)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": 1, "runs": out})
	case http.MethodPost:
		var req taskRunRecordRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTaskRunLogBytes+(128<<10)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid task-run record payload", http.StatusBadRequest)
			return
		}
		item, err := s.recordTaskRun(r, req)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not visible") {
				status = http.StatusForbidden
			} else if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		s.auditSharedSuccess(r, "task.history.record", "task_run", item.ID, map[string]any{
			"task_id": item.TaskID,
			"session_id": item.SessionID,
			"log_bytes": item.LogBytes,
			"log_truncated": item.LogTruncated,
		})
		writeJSON(w, http.StatusCreated, item)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) taskRunLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "task-run id is required", http.StatusBadRequest)
		return
	}
	s.taskRunHistoryMu.Lock()
	history, err := readTaskRunHistory(s.Workspace)
	if err != nil {
		s.taskRunHistoryMu.Unlock()
		http.Error(w, "cannot read task-run history", http.StatusInternalServerError)
		return
	}
	var selected *taskRunRecord
	for i := range history.Runs {
		if history.Runs[i].ID == id && taskRunVisible(r, history.Runs[i]) {
			item := history.Runs[i]
			selected = &item
			break
		}
	}
	if selected == nil {
		s.taskRunHistoryMu.Unlock()
		http.Error(w, "task run not found", http.StatusNotFound)
		return
	}
	path, err := taskRunLogPath(s.Workspace, id)
	if err != nil {
		s.taskRunHistoryMu.Unlock()
		http.Error(w, "task-run log unavailable", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(path)
	s.taskRunHistoryMu.Unlock()
	if err != nil {
		http.Error(w, "task-run log unavailable", http.StatusNotFound)
		return
	}
	if len(data) > maxTaskRunLogBytes {
		data = data[:maxTaskRunLogBytes]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", selected.ID+".log"))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
