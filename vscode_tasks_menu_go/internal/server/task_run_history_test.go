package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func waitTaskRunSession(t *testing.T, manager session.Service, id string) session.Metadata {
	t.Helper()
	deadline:=time.Now().Add(5*time.Second)
	for time.Now().Before(deadline){
		meta,ok:=manager.Metadata(id)
		if ok&&meta.Status!="running"{return meta}
		time.Sleep(10*time.Millisecond)
	}
	t.Fatal("task session did not finish")
	return session.Metadata{}
}

func TestDurableTaskRunHistoryRecordsSessionEvidenceAndLog(t *testing.T) {
	workspace,s,_:=setupGitQuickRepo(t)
	manager:=session.NewManager(2<<20)
	s.Sessions=manager
	meta,err:=manager.Start(tasks.Execution{
		TaskID:7,Label:"Build",Command:"/bin/sh",Args:[]string{"-c","printf task-ok"},Cwd:workspace,Env:os.Environ(),
		SessionKind:tasks.SessionKindTask,OwnerUserID:"alice",ProjectID:"project-1",
	})
	if err!=nil{t.Fatal(err)}
	finished:=waitTaskRunSession(t,manager,meta.ID)
	if finished.ExitCode==nil||*finished.ExitCode!=0{t.Fatalf("finished meta=%+v",finished)}

	logText:=strings.Repeat("x",maxTaskRunLogBytes+37)
	body,_:=json.Marshal(taskRunRecordRequest{SessionID:meta.ID,Log:logText,ProjectProfileID:"release"})
	req:=httptest.NewRequest(http.MethodPost,"/api/task-runs",bytes.NewReader(body))
	req.Header.Set("Content-Type","application/json")
	rr:=httptest.NewRecorder();s.Handler().ServeHTTP(rr,req)
	if rr.Code!=http.StatusCreated{t.Fatalf("record status=%d body=%s",rr.Code,rr.Body.String())}
	var recorded taskRunRecord
	if err:=json.Unmarshal(rr.Body.Bytes(),&recorded);err!=nil{t.Fatal(err)}
	if recorded.TaskID!=7||recorded.Status!="PASS"||recorded.ProjectProfileID!="release"{t.Fatalf("record=%+v",recorded)}
	if !recorded.LogTruncated||recorded.LogBytes!=maxTaskRunLogBytes{t.Fatalf("log bounds record=%+v",recorded)}
	wantHead:=gitQuickRun(t,workspace,"rev-parse","HEAD")
	if recorded.GitCommit!=wantHead{t.Fatalf("git commit=%q want=%q",recorded.GitCommit,wantHead)}

	req=httptest.NewRequest(http.MethodGet,"/api/task-runs",nil)
	rr=httptest.NewRecorder();s.Handler().ServeHTTP(rr,req)
	if rr.Code!=http.StatusOK{t.Fatalf("list status=%d body=%s",rr.Code,rr.Body.String())}
	var list struct{Runs []taskRunRecord `json:"runs"`}
	if err:=json.Unmarshal(rr.Body.Bytes(),&list);err!=nil{t.Fatal(err)}
	if len(list.Runs)!=1||list.Runs[0].ID!=recorded.ID{t.Fatalf("runs=%+v",list.Runs)}

	req=httptest.NewRequest(http.MethodGet,"/api/task-runs/log?id="+recorded.ID,nil)
	rr=httptest.NewRecorder();s.Handler().ServeHTTP(rr,req)
	if rr.Code!=http.StatusOK{t.Fatalf("log status=%d body=%s",rr.Code,rr.Body.String())}
	if rr.Body.Len()!=maxTaskRunLogBytes{t.Fatalf("log bytes=%d",rr.Body.Len())}
}

func TestDurableTaskRunHistoryRejectsActiveSession(t *testing.T) {
	workspace:=t.TempDir();manager:=session.NewManager(1<<20);s:=&Server{Workspace:workspace,Sessions:manager}
	meta,err:=manager.Start(tasks.Execution{
		TaskID:1,Label:"Long",Command:"/bin/sh",Args:[]string{"-c","sleep 1"},Cwd:workspace,Env:os.Environ(),
		SessionKind:tasks.SessionKindTask,OwnerUserID:"alice",ProjectID:"project-1",
	})
	if err!=nil{t.Fatal(err)}
	defer manager.Kill(meta.ID)
	body,_:=json.Marshal(taskRunRecordRequest{SessionID:meta.ID,Log:"partial"})
	req:=httptest.NewRequest(http.MethodPost,"/api/task-runs",bytes.NewReader(body));req.Header.Set("Content-Type","application/json")
	rr:=httptest.NewRecorder();s.Handler().ServeHTTP(rr,req)
	if rr.Code!=http.StatusBadRequest{t.Fatalf("active run status=%d body=%s",rr.Code,rr.Body.String())}
}
