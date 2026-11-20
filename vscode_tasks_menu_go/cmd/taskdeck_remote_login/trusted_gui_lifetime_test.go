package main

import (
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

// A task/action admitted just before expiry must not receive a fresh
// sixteen-second execution window beyond the original authenticated lease.
func TestTrustedGUIInFlightActionCannotOutliveSession(t *testing.T) {
 actions:=&fakeGUIActions{pingStart:make(chan struct{})}
 ui,_:=newUITest(t,actions)
 defer ui.stop()
 ui.startedAt=time.Now().Add(-trustedGUIMaxLifetime+150*time.Millisecond)
 done:=make(chan int,1)
 go func(){
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest(http.MethodPost,"/api/ping",uiTestOrigin,uiTestToken))
  done<-rr.Code
 }()
 select {
 case <-actions.pingStart:
 case <-time.After(time.Second):
  t.Fatal("authorized action did not start")
 }
 select {
 case code:=<-done:
  if code==http.StatusOK{
   t.Fatal("expired E2EE action returned success")
  }
 case <-time.After(2*time.Second):
  t.Fatal("expired action did not observe session deadline")
 }
}

// Closing/poisoning an authenticated GUI session must cancel any active
// TaskDeck operation rather than waiting for its per-operation timeout.
func TestTrustedGUIPoisonInterruptsInFlightAction(t *testing.T){
 actions:=&fakeGUIActions{pingStart:make(chan struct{})}
 ui,_:=newUITest(t,actions)
 defer ui.stop()
 done:=make(chan int,1)
 go func(){
  rr:=httptest.NewRecorder()
  ui.ServeHTTP(rr,localRequest(http.MethodPost,"/api/ping",uiTestOrigin,uiTestToken))
  done<-rr.Code
 }()
 select {
 case <-actions.pingStart:
 case <-time.After(time.Second):
  t.Fatal("authenticated action did not start")
 }
 ui.poison()
 select{
 case code:=<-done:
  if code==http.StatusOK{t.Fatal("poisoned action returned success")}
 case <-time.After(time.Second):
  t.Fatal("poisoned action was not interrupted")
 }
 if actions.closeCount.Load()!=1{t.Fatal("AEAD channel was not closed exactly once")}
}
