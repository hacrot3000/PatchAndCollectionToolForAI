package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The real host-authorized, Portal-signed admin fixture is shared by
// Git/Terminal tests on all platforms. Never elevate a viewer test by
// editing a session role after authentication.
func remoteTerminalAdminFixture(t *testing.T)(
 *Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel){
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 fingerprint:=sha256.Sum256(pub)
 device:=hex.EncodeToString(fingerprint[:])
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 hello,agent,err:=remotetransport.ClientHello(priv,device,
  strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
 if err!=nil{t.Fatal(err)}
 client,err:=remotetransport.AcceptHello(browser,device,
  strings.Repeat("a",32),strings.Repeat("b",32),hello)
 if err!=nil{t.Fatal(err)}
 handshake,err:=client.Seal([]byte("authenticated-client-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(handshake);err!=nil{t.Fatal(err)}
 server:=sharedLoginTestServer(t)
 server.Config.RemoteAccessEnabled=true
 server.Config.Protocol=config.ProtocolHTTPS
 grant:=testRemoteSignedGrant(t,agent,remotetransport.RoleAdmin)
 session,err:=server.authenticateRemoteTaskDeckCredentials(
  context.Background(),agent,grant,"alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 bindTestRemoteOwnerClient(t,server,session)
 t.Cleanup(session.Close)
 t.Cleanup(client.Close)
 allowTestLivePortal(session)
 return server,session,client,agent
}

