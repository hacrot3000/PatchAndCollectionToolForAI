package server

import (
 "context"
 "errors"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

type localAuthenticatedPingLink struct {
 server *Server
 session *remoteTaskDeckSession
 channel *remotetransport.Channel
}

func (l localAuthenticatedPingLink) ExchangePing(ctx context.Context,encrypted []byte)([]byte,error){
 return l.server.executeRemoteE2EEPing(ctx,l.session,l.channel,encrypted)
}

// The native side and server-side channel share only authenticated AES-GCM
// ciphertext. This is an INTERNAL end-to-end transport test. Relay/Portal
// application transport is intentionally not mounted.
func TestRemoteNativePingRoundTripRequiresLocalTaskDeckSession(t *testing.T){
 t.Run("authorized read-only ping",func(t *testing.T){
  s,session,native,agent:=remoteDispatchFixture(t)
  link:=localAuthenticatedPingLink{s,session,agent}
  client,err:=remotetransport.NewPingClient(native,link)
  if err!=nil{t.Fatal(err)}
  defer client.Close()
  if err:=client.Ping(context.Background());err!=nil{t.Fatalf("authorized ping rejected: %v",err)}
  if err:=client.Ping(context.Background());err!=nil{t.Fatalf("second ordered ping rejected: %v",err)}
 })
 t.Run("Portal ticket without TaskDeck login",func(t *testing.T){
  s,_,native,agent:=remoteDispatchFixture(t)
  client,err:=remotetransport.NewPingClient(native,localAuthenticatedPingLink{s,nil,agent})
  if err!=nil{t.Fatal(err)}
  if err:=client.Ping(context.Background());!errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("Portal-only session admitted: %v",err)
  }
  if native.Established(){t.Fatal("unauthorized channel remained live")}
 })
 t.Run("revoked TaskDeck password session",func(t *testing.T){
  s,session,native,agent:=remoteDispatchFixture(t)
  client,err:=remotetransport.NewPingClient(native,localAuthenticatedPingLink{s,session,agent})
  if err!=nil{t.Fatal(err)}
  session.Close()
  if err:=client.Ping(context.Background());!errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("revoked session ping succeeded: %v",err)
  }
 })
}
