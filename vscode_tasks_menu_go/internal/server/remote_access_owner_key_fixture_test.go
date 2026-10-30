package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
)

// Test-only setup for internal low-level password sessions: production sets
// clientProofVerified only after checking the REAL Ed25519 possession proof
// on the Agent's E2EE transcript. This fixture models that completed stage;
// it never enables or exposes a network route.
func bindTestRemoteOwnerClient(t *testing.T,s *Server,session *remoteTaskDeckSession){
 t.Helper()
 if s==nil||session==nil{t.Fatal("missing local TaskDeck session")}
 user,err:=s.Identity.UserByID(context.Background(),session.userID)
 if err!=nil{t.Fatal(err)}
 pub,_,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 encoded:=base64.StdEncoding.EncodeToString(pub)
 registry,err:=remoteclientkey.DefaultPath(s.Workspace)
 if err!=nil{t.Fatal(err)}
 if _,err:=remoteclientkey.AddForUser(registry,user.Username,string(user.ID),
  encoded,"test-native-key");err!=nil{t.Fatal(err)}
 session.clientPublicKey=encoded
 session.clientUsername=user.Username
 session.clientProofVerified=true
}
