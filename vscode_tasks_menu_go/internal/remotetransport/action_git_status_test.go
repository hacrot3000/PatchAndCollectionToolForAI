package remotetransport

import (
    "bytes"
    "context"
    "errors"
    "strings"
    "testing"
)

func TestRemoteGitStatusE2EECodecAndReplay(t *testing.T) {
    client, agent, _ := participants(t)
    request, err := SealRemoteAction(client, RemoteAction{
        RequestID: testActionID, Operation: RemoteActionGitStatus,
    })
    if err != nil { t.Fatal(err) }
    if bytes.Contains(request, []byte(RemoteActionGitStatus)) {
        t.Fatal("Relay can see plaintext Git operation")
    }
    opened, err := OpenRemoteAction(agent, request)
    if err != nil || opened.Operation != RemoteActionGitStatus || opened.RequestID != testActionID {
        t.Fatalf("Git operation changed under E2EE: %+v %v", opened, err)
    }
    want := "## main\n M src/main.go\n"
    response, err := SealRemoteActionGitStatusResult(agent, testActionID, want)
    if err != nil { t.Fatal(err) }
    if bytes.Contains(response, []byte("src/main.go")) {
        t.Fatal("Relay can see plaintext Git filename")
    }
    status, err := OpenRemoteActionGitStatusResult(client, response, testActionID)
    if err != nil || status != want { t.Fatalf("Git status reply %q: %v", status, err) }
    if _, err := OpenRemoteActionGitStatusResult(client, response, testActionID); err == nil {
        t.Fatal("replayed AEAD Git response accepted")
    }
    if client.Established() { t.Fatal("replay did not close E2EE channel") }
}

func TestRemoteGitStatusMalformedResponsesFailClosed(t *testing.T) {
    cases := map[string]string{
        "forged capability": `{"type":"taskdeck-e2ee-git-status-result-v1","request_id":"0123456789abcdef0123456789abcdef","status":"## main","terminal_exec":true}`,
        "duplicate key": `{"type":"taskdeck-e2ee-git-status-result-v1","request_id":"0123456789abcdef0123456789abcdef","status":"## main","status":"## hacked"}`,
        "wrong ID": `{"type":"taskdeck-e2ee-git-status-result-v1","request_id":"fedcba9876543210fedcba9876543210","status":"## main"}`,
        "wrong protocol": `{"type":"taskdeck-e2ee-action-result-v1","request_id":"0123456789abcdef0123456789abcdef","status":"## main"}`,
        "terminal escape": `{"type":"taskdeck-e2ee-git-status-result-v1","request_id":"0123456789abcdef0123456789abcdef","status":"\u001b[31m"}`,
        "oversize": `{"type":"taskdeck-e2ee-git-status-result-v1","request_id":"0123456789abcdef0123456789abcdef","status":"`+strings.Repeat("x",257)+`"}`,
    }
    for name, clear := range cases {
        t.Run(name,func(t *testing.T){
            client, agent, _ := participants(t)
            frame, err := agent.Seal([]byte(clear))
            if err != nil { t.Fatal(err) }
            if _, err := OpenRemoteActionGitStatusResult(client, frame, testActionID); !errors.Is(err, ErrInvalidRemoteAction) {
                t.Fatalf("malformed Git reply accepted: %v",err)
            }
            if client.Established() { t.Fatal("invalid Git response left E2EE channel alive") }
        })
    }
}

func TestRemoteGitStatusRejectsShellAndOversize(t *testing.T) {
    client, agent, _ := participants(t)
    if _,err:=SealRemoteActionGitStatusResult(agent, testActionID, "\x1b[31mred");err==nil {
        t.Fatal("terminal control sequence leaked")
    }
    if _,err:=SealRemoteActionGitStatusResult(agent, testActionID, strings.Repeat("x",257));err==nil {
        t.Fatal("oversized Git output accepted")
    }
    if _,err:=SealRemoteAction(client,RemoteAction{
        RequestID:testActionID,Operation:"git.commit",
    });err==nil {t.Fatal("git.commit without a bounded message was enabled")}
    if _,err:=SealRemoteAction(client,RemoteAction{
        RequestID:testActionID,Operation:"terminal.exec",
    });err==nil {t.Fatal("terminal.exec was enabled")}
}

func TestRemoteGitStatusNativeClientChecksEncryptedResult(t *testing.T){
    client,agent,_:=participants(t)
    exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
        request,err:=OpenRemoteAction(agent,frame)
        if err!=nil || request.Operation!=RemoteActionGitStatus {return nil,ErrRemoteActionDenied}
        return SealRemoteActionGitStatusResult(agent,request.RequestID,"## main\n M file.go\n")
    })
    native,err:=NewPingClient(client,exchange)
    if err!=nil{t.Fatal(err)}
    defer native.Close()
    result,err:=native.GitStatus(context.Background())
    if err!=nil||result!="## main\n M file.go\n" {
        t.Fatalf("native E2EE Git status=%q err=%v",result,err)
    }
}
