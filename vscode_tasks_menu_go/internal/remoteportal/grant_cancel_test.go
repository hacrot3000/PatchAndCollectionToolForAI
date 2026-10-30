package remoteportal

import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/base64"
    "strings"
    "sync"
    "testing"
    "time"

    "bletonfc/vscode_tasks_menu/internal/remoterelay"
)

func TestSignedPortalGrantDoneFiresOnceUnderConcurrentRevocation(t *testing.T) {
    public,private,err:=ed25519.GenerateKey(rand.Reader)
    if err!=nil{t.Fatal(err)}
    channel:=testBoundChannel(t)
    now:=time.Now()
    verifier,err:=NewAgentGrantVerifier(
        base64.StdEncoding.EncodeToString(public),
        "https://portal.example.com",channel.BoundDeviceID())
    if err!=nil{t.Fatal(err)}
    claims:=remoterelay.Claims{
        V:2,AuthEpoch:1,BrowserBinding:channel.BrowserTicketBinding(),
        Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
        Subject:"21",DeviceID:channel.BoundDeviceID(),Role:"viewer",
        IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("f",32),
    }
    grant,err:=verifier.Consume(signPortalTicket(t,private,claims),channel,now)
    if err!=nil{t.Fatal(err)}
    done:=grant.Done()
    select {
    case <-done:t.Fatal("fresh signed grant was revoked")
    default:
    }
    var wg sync.WaitGroup
    for i:=0;i<32;i++{
        wg.Add(1)
        go func(){defer wg.Done();grant.Close()}()
    }
    wg.Wait()
    select {
    case <-done:
    case <-time.After(time.Second):t.Fatal("revoked signed grant did not signal close")
    }
    if grant.markClosed(){t.Fatal("revoked signed grant consumed a second finalization")}
    if _,err:=grant.RoleFor(channel,time.Now());err==nil{
        t.Fatal("closed grant authorized another login")
    }
    var nilGrant *AgentGrant
    select {
    case <-nilGrant.Done():
    default:t.Fatal("nil grant exposed a never-closing cancellation signal")
    }
    select {
    case <-(&AgentGrant{}).Done():
    default:t.Fatal("zero-valued grant exposed a never-closing cancellation signal")
    }
}
