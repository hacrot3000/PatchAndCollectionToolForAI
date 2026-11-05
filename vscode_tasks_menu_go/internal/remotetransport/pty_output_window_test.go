package remotetransport

import (
 "bytes"
 "errors"
 "sync"
 "testing"
)

const outputTestHandle="0123456789abcdef0123456789abcdef"

func outputTestFrame(seq uint64,data []byte)RemotePTYFrame{
 return RemotePTYFrame{Handle:outputTestHandle,Kind:"output",Sequence:seq,Data:data}
}
func TestRemotePTYOutputWindowNeverLosesBytesSilently(t *testing.T){
 w,err:=NewRemotePTYOutputWindow(outputTestHandle)
 if err!=nil{t.Fatal(err)}
 if err=w.AppendFromHost(outputTestFrame(1,[]byte("first")));err!=nil{t.Fatal(err)}
 if err=w.AppendFromHost(outputTestFrame(2,[]byte("second")));err!=nil{t.Fatal(err)}
 first,next,err:=w.ReadAfter(0)
 if err!=nil||string(first)!="firstsecond"||next!=11{
  t.Fatalf("wrong initial window %q %d %v",first,next,err)
 }
 empty,at,err:=w.ReadAfter(next)
 if err!=nil||len(empty)!=0||at!=next{t.Fatal("EOF was not exact",err)}
 if _,_,err=w.ReadAfter(-1);err==nil{t.Fatal("negative cursor accepted")}
 if _,_,err=w.ReadAfter(next+1);err==nil{t.Fatal("future cursor accepted")}
 if from,to,err:=w.RetainedRange();err!=nil||from!=0||to!=11{
  t.Fatalf("wrong authenticated retained range %d..%d %v",from,to,err)
 }
}

func TestRemotePTYOutputWindowEvictsOldDataButReportsExactGap(t *testing.T){
 w,err:=NewRemotePTYOutputWindow(outputTestHandle)
 if err!=nil{t.Fatal(err)}
 chunk:=bytes.Repeat([]byte{'A'},RemotePTYMaxOutput)
 count:=RemotePTYOutputRetainedBytes/RemotePTYMaxOutput+3
 for i:=0;i<count;i++{
  if err=w.AppendFromHost(outputTestFrame(uint64(i+1),chunk));err!=nil{t.Fatal(err)}
 }
 low,high,err:=w.RetainedRange()
 expectedHigh:=int64(count*RemotePTYMaxOutput)
 expectedLow:=expectedHigh-RemotePTYOutputRetainedBytes
 if err!=nil||low!=expectedLow||high!=expectedHigh{
  t.Fatalf("incorrect overflow tracking %d..%d wanted %d..%d (%v)",
   low,high,expectedLow,expectedHigh,err)
 }
 if payload,anchor,err:=w.ReadAfter(0);
  !errors.Is(err,ErrRemotePTYOutputGap)||payload!=nil||anchor!=low{
  t.Fatalf("stale reader did not detect lost bytes: %d %v",anchor,err)
 }
 cursor:=low
 var got []byte
 for cursor<high{
  data,next,err:=w.ReadAfter(cursor)
  if err!=nil||next<=cursor||len(data)>RemotePTYOutputWindowBytes{
   t.Fatalf("invalid PTY window %d..%d %v",cursor,next,err)
  }
  got=append(got,data...)
  cursor=next
 }
 if len(got)!=RemotePTYOutputRetainedBytes||
  !bytes.Equal(got,bytes.Repeat([]byte{'A'},RemotePTYOutputRetainedBytes)){
  t.Fatal("retained authenticated output window corrupted")
 }
}

func TestRemotePTYOutputWindowRejectsReplayWrongRoleAndHandle(t *testing.T){
 for _,wrong:=range []RemotePTYFrame{
  outputTestFrame(2,[]byte("skipped")),
  {Handle:outputTestHandle,Kind:"input",Sequence:1,Data:[]byte("ls\r")},
  {Handle:"ffffffffffffffffffffffffffffffff",Kind:"output",Sequence:1,Data:[]byte("wrong")},
 }{
  w,err:=NewRemotePTYOutputWindow(outputTestHandle)
  if err!=nil{t.Fatal(err)}
  if err=w.AppendFromHost(wrong);err==nil{t.Fatal("unowned/incorrect frame accepted")}
  if _,_,err=w.ReadAfter(0);err==nil{t.Fatal("invalid frame did not retire window")}
  if err=w.AppendFromHost(outputTestFrame(1,[]byte("later")));err==nil{
   t.Fatal("invalid PTY state resumed")
  }
 }
 w,err:=NewRemotePTYOutputWindow(outputTestHandle)
 if err!=nil{t.Fatal(err)}
 valid:=outputTestFrame(1,[]byte("x"))
 if err=w.AppendFromHost(valid);err!=nil{t.Fatal(err)}
 if err=w.AppendFromHost(valid);err==nil{t.Fatal("replayed PTY output admitted")}
 if _,_,err=w.ReadAfter(0);err==nil{t.Fatal("replay did not retire buffer")}
}

func TestRemotePTYOutputWindowCloseScrubsDataAndRefusesLateOutput(t *testing.T){
 w,err:=NewRemotePTYOutputWindow(outputTestHandle)
 if err!=nil{t.Fatal(err)}
 if err=w.AppendFromHost(outputTestFrame(1,[]byte("PRIVATE_SECRET_OUTPUT")));err!=nil{
  t.Fatal(err)
 }
 w.Close()
 w.Close()
 if _,_,err=w.ReadAfter(0);err==nil{t.Fatal("closed window readable")}
 if _,_,err=w.RetainedRange();err==nil{t.Fatal("closed window range readable")}
 if err=w.AppendFromHost(outputTestFrame(2,[]byte("late")));err==nil{
  t.Fatal("late PTY output appended after Disconnect")
 }
 if len(w.buf)!=0{t.Fatal("closed PTY plaintext retained")}
}

func TestRemotePTYOutputWindowConcurrentDuplicateCannotDoubleAppend(t *testing.T){
 w,err:=NewRemotePTYOutputWindow(outputTestHandle)
 if err!=nil{t.Fatal(err)}
 const races=25
 var wg sync.WaitGroup
 wg.Add(races)
 var accepted int
 var mu sync.Mutex
 for i:=0;i<races;i++{
  go func(){
   defer wg.Done()
   err:=w.AppendFromHost(outputTestFrame(1,[]byte("once")))
   if err==nil{mu.Lock();accepted++;mu.Unlock()}
  }()
 }
 wg.Wait()
 if accepted!=1{t.Fatalf("same sequence concurrently accepted %d times",accepted)}
 // A duplicate permanently retires the logical session; never return
 // arbitrarily shuffled or partially trusted bytes as successful output.
 if _,_,err:=w.ReadAfter(0);err==nil{t.Fatal("duplicate did not retire PTY stream")}
}
