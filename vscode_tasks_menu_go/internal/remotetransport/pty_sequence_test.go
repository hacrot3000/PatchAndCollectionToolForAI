package remotetransport

import (
 "sync"
 "sync/atomic"
 "testing"
)

func ptySeqInput(handle string,n uint64)RemotePTYFrame{
 return RemotePTYFrame{Handle:handle,Kind:"input",
  Sequence:n,Data:[]byte("a\r")}
}
func ptySeqOutput(handle string,n uint64)RemotePTYFrame{
 return RemotePTYFrame{Handle:handle,Kind:"output",
  Sequence:n,Data:[]byte("output")}
}
func TestRemotePTYSequenceGatePreservesBothDirectionsAndClose(t *testing.T){
 handle:="1123456789abcdef0123456789abcdef"
 gate,err:=NewBoundedRemotePTYSessionGate(handle)
 if err!=nil{t.Fatal(err)}
 if err=gate.AcceptFromClient(ptySeqInput(handle,1));err!=nil{t.Fatal(err)}
 if err=gate.AcceptFromHost(ptySeqOutput(handle,1));err!=nil{t.Fatal(err)}
 if err=gate.AcceptFromClient(RemotePTYFrame{
  Handle:handle,Kind:"resize",Sequence:2,Rows:30,Cols:120,
 });err!=nil{t.Fatal(err)}
 if err=gate.AcceptFromHost(ptySeqOutput(handle,2));err!=nil{t.Fatal(err)}
 if gate.Closed(){t.Fatal("healthy bidirectional PTY frame session retired")}
 gate.Close()
 if !gate.Closed(){t.Fatal("closed PTY gate still reports open")}
 if gate.AcceptFromClient(ptySeqInput(handle,3))==nil||
  gate.AcceptFromHost(ptySeqOutput(handle,3))==nil{
  t.Fatal("closed gate accepted a PTY frame")
 }
}

func TestRemotePTYSequenceGateRejectsReplaySkippedAndWrongDirection(t *testing.T){
 handle:="1123456789abcdef0123456789abcdef"
 other:="2223456789abcdef0123456789abcdef"
 for _,tc:=range []struct{
  name string
  first RemotePTYFrame
  second RemotePTYFrame
  client bool
 }{
  {"replay",ptySeqInput(handle,1),ptySeqInput(handle,1),true},
  {"skip",ptySeqInput(handle,1),ptySeqInput(handle,3),true},
  {"wrong-handle",ptySeqInput(handle,1),ptySeqInput(other,2),true},
  {"host-input",ptySeqOutput(handle,1),ptySeqInput(handle,2),false},
  {"client-output",ptySeqInput(handle,1),ptySeqOutput(handle,2),true},
  {"zero-sequence",ptySeqInput(handle,1),ptySeqInput(handle,0),true},
 }{
  t.Run(tc.name,func(t *testing.T){
   gate,_:=NewBoundedRemotePTYSessionGate(handle)
   if tc.client{
    if e:=gate.AcceptFromClient(tc.first);e!=nil{t.Fatal(e)}
    if e:=gate.AcceptFromClient(tc.second);e==nil{t.Fatal("invalid client frame accepted")}
   }else{
    if e:=gate.AcceptFromHost(tc.first);e!=nil{t.Fatal(e)}
    if e:=gate.AcceptFromHost(tc.second);e==nil{t.Fatal("invalid host frame accepted")}
   }
   if !gate.Closed(){t.Fatal("out-of-order PTY session was not retired")}
  })
 }
}

func TestRemotePTYSequenceGateCapsFrameCountWithoutWrap(t *testing.T){
 handle:="1123456789abcdef0123456789abcdef"
 gate,_:=NewBoundedRemotePTYSessionGate(handle)
 for i:=uint64(1);i<=RemotePTYMaxFramesPerDirection;i++{
  if err:=gate.AcceptFromClient(ptySeqInput(handle,i));err!=nil{
   t.Fatalf("PTY frame %d denied prematurely: %v",i,err)
  }
 }
 if err:=gate.AcceptFromClient(ptySeqInput(handle,
  RemotePTYMaxFramesPerDirection+1));err==nil||!gate.Closed(){
  t.Fatal("PTY input accepted beyond per-session fixed frame quota")
 }
}

func TestRemotePTYSequenceGateConcurrentReplayedInputAdmitsOnlyOnce(t *testing.T){
 h:="1123456789abcdef0123456789abcdef"
 gate,_:=NewBoundedRemotePTYSessionGate(h)
 var accepted atomic.Int32
 var workers sync.WaitGroup
 for i:=0;i<32;i++{
  workers.Add(1)
  go func(){
   defer workers.Done()
   if gate.AcceptFromClient(ptySeqInput(h,1))==nil{accepted.Add(1)}
  }()
 }
 workers.Wait()
 if accepted.Load()!=1||!gate.Closed(){
  t.Fatalf("duplicate concurrent input admitted %d times",accepted.Load())
 }
}
func TestRemotePTYSequenceGateRejectsUnknownHandleBeforeStart(t *testing.T){
 if gate,err:=NewBoundedRemotePTYSessionGate("invalid");err==nil||gate!=nil{
  t.Fatal("invalid PTY ownership handle created a session gate")
 }
 var empty *BoundedRemotePTYSessionGate
 if !empty.Closed()||empty.AcceptFromClient(ptySeqInput(
  "1123456789abcdef0123456789abcdef",1))==nil{
  t.Fatal("nil PTY gate allowed input")
 }
}


func TestRemotePTYSequenceGateHostOutputHasIndependentQuota(t *testing.T){
 h:="1123456789abcdef0123456789abcdef"
 gate,_:=NewBoundedRemotePTYSessionGate(h)
 for i:=uint64(1);i<=RemotePTYMaxFramesPerDirection;i++{
  if err:=gate.AcceptFromHost(ptySeqOutput(h,i));err!=nil{
   t.Fatalf("host output frame %d rejected: %v",i,err)
  }
 }
 if gate.Closed(){t.Fatal("exhausting output budget early retired logical session")}
 // Input's own sequence is not advanced by output. Once the host
 // attempts an over-budget frame, the entire logical terminal retires.
 if e:=gate.AcceptFromClient(ptySeqInput(h,1));e!=nil{
  t.Fatal("independent client input budget was consumed by output",e)
 }
 if e:=gate.AcceptFromHost(ptySeqOutput(h,RemotePTYMaxFramesPerDirection+1));e==nil{
  t.Fatal("over-budget host output was accepted")
 }
 if !gate.Closed()||gate.AcceptFromClient(ptySeqInput(h,2))==nil{
  t.Fatal("over-budget host output failed to revoke both PTY directions")
 }
}
