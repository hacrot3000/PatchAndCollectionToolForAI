package remotetransport

import (
 "context"
 "errors"
 "sync/atomic"
 "testing"
 "time"
)

type pingExchangeFunc func(context.Context,[]byte)([]byte,error)
func (f pingExchangeFunc) ExchangePing(ctx context.Context,raw []byte)([]byte,error){
 return f(ctx,raw)
}

func TestPingClientBoundedRoundTripAndReplayPoison(t *testing.T){
 native,agent,_:=participants(t)
 var seen atomic.Int32
 exchange:=pingExchangeFunc(func(_ context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil{return nil,err}
  if request.Operation!=RemoteActionSessionPing{t.Fatal("unexpected operation")}
  seen.Add(1)
  return SealRemoteActionPingResult(agent,request.RequestID)
 })
 client,err:=NewPingClient(native,exchange)
 if err!=nil{t.Fatal(err)}
 defer client.Close()
 for i:=0;i<3;i++{
  if err:=client.Ping(context.Background());err!=nil{t.Fatal(err)}
 }
 if seen.Load()!=3 {t.Fatalf("expected three independent authenticated pings, got %d",seen.Load())}
 client.Close()
 if err:=client.Ping(context.Background());!errors.Is(err,ErrRemoteActionDenied){
  t.Fatalf("closed client accepted new ping: %v",err)
 }
 if native.Established(){t.Fatal("closed client retained AEAD keys")}
}

func TestPingClientRejectsConcurrentRequestAndClosesOnCancel(t *testing.T){
 native,_,_:=participants(t)
 entered:=make(chan struct{})
 firstDone:=make(chan error,1)
 handler:=pingExchangeFunc(func(ctx context.Context,_ []byte)([]byte,error){
  close(entered)
  <-ctx.Done()
  return nil,ctx.Err()
 })
 client,err:=NewPingClient(native,handler)
 if err!=nil{t.Fatal(err)}
 go func(){firstDone<-client.Ping(context.Background())}()
 select{
 case <-entered:
 case <-time.After(time.Second):t.Fatal("first exchange never started")
 }
 if err:=client.Ping(context.Background());!errors.Is(err,ErrRemoteActionDenied){
  t.Fatalf("parallel ping admitted: %v",err)
 }
 closed:=make(chan struct{})
 go func(){client.Close();close(closed)}()
 select{
 case <-closed:
 case <-time.After(time.Second):t.Fatal("Close was blocked by a stalled exchange")
 }
 select{
 case err:=<-firstDone:
  if !errors.Is(err,ErrRemoteActionDenied){t.Fatalf("canceled exchange succeeded: %v",err)}
 case <-time.After(time.Second):t.Fatal("canceled exchange did not return")
 }
}

func TestPingClientRefusesForgedReplyAndWrongRequest(t *testing.T){
 for _,mode:=range []string{"wrongID","malformed","transportError","oversized","zero"}{
  t.Run(mode,func(t *testing.T){
   native,agent,_:=participants(t)
   exchange:=pingExchangeFunc(func(_ context.Context,frame []byte)([]byte,error){
    request,err:=OpenRemoteAction(agent,frame)
    if err!=nil{return nil,err}
    switch mode {
    case "wrongID":
     return SealRemoteActionPingResult(agent,"ffffffffffffffffffffffffffffffff")
    case "malformed":
     return agent.Seal([]byte(`{"type":"e2ee-fake","ok":true}`))
    case "transportError":
     return nil,errors.New("lost relay")
    case "oversized":
     return make([]byte,MaxEncryptedPingFrame+1),nil
    default:
     return nil,nil
    }
   })
   client,err:=NewPingClient(native,exchange)
   if err!=nil{t.Fatal(err)}
   if err:=client.Ping(context.Background());!errors.Is(err,ErrRemoteActionDenied){
    t.Fatalf("forged response accepted: %v",err)
   }
   if native.Established(){t.Fatal("failed exchange left channel available")}
   if err:=client.Ping(context.Background());!errors.Is(err,ErrRemoteActionDenied){
    t.Fatalf("failed session revived: %v",err)
   }
  })
 }
}

func TestPingClientRejectsLateResponseAfterLocalCancel(t *testing.T){
 native,agent,_:=participants(t)
 entered:=make(chan struct{})
 release:=make(chan struct{})
 done:=make(chan error,1)
 exchange:=pingExchangeFunc(func(_ context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil{return nil,err}
  close(entered)
  <-release // transport implementation incorrectly ignores cancellation
  return SealRemoteActionPingResult(agent,request.RequestID)
 })
 client,err:=NewPingClient(native,exchange)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 go func(){done<-client.Ping(ctx)}()
 select{
 case <-entered:
 case <-time.After(time.Second):t.Fatal("no request")
 }
 cancel()
 close(release)
 select{
 case err:=<-done:
  if !errors.Is(err,ErrRemoteActionDenied){t.Fatalf("late reply succeeded: %v",err)}
 case <-time.After(time.Second):t.Fatal("late reply blocked")
 }
 if native.Established(){t.Fatal("canceled client retained channel keys")}
}

func TestPingClientRejectsMissingTrustAndCanceledContext(t *testing.T){
 native,_,_:=participants(t)
 if _,err:=NewPingClient(nil,pingExchangeFunc(func(context.Context,[]byte)([]byte,error){return nil,nil}));err==nil {
  t.Fatal("nil authenticated channel accepted")
 }
 if _,err:=NewPingClient(native,nil);err==nil {t.Fatal("nil transport accepted")}
 client,err:=NewPingClient(native,pingExchangeFunc(func(context.Context,[]byte)([]byte,error){
  t.Error("canceled context invoked transport")
  return nil,nil
 }))
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 if err:=client.Ping(ctx);!errors.Is(err,ErrRemoteActionDenied){t.Fatalf("canceled ping admitted: %v",err)}
 client.Close()
}
