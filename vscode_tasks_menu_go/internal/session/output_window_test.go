package session

import (
 "bytes"
 "strings"
 "testing"
)

func TestOutputWindowCursorNeverSkipsEvictedBytes(t *testing.T){
 s:=&managedSession{scrollback:[]byte("HEADER"),outputTotal:6,maxScrollback:12}
 m:=&Manager{sessions:map[string]*managedSession{"x":s}}
 first,err:=m.ReadOutputWindow("x",0,4)
 if err!=nil||string(first.Data)!="HEAD"||first.Next!=4||first.Total!=6{
  t.Fatalf("wrong initial output window %+v %v",first,err)
 }
 s.appendOutput([]byte("ABCDEFGH"))
 if _,err:=m.ReadOutputWindow("x",0,4);err==nil{
  t.Fatal("evicted prefix was silently skipped")
 }
 second,err:=m.ReadOutputWindow("x",2,12)
 if err!=nil||second.Next!=14||string(second.Data)!="ADERABCDEFGH"{
  t.Fatalf("retained suffix should preserve absolute offsets %+v %v",second,err)
 }
 if _,err:=m.ReadOutputWindow("x",15,1);err==nil{
  t.Fatal("future cursor was accepted")
 }
 if _,err:=m.ReadOutputWindow("x",-1,1);err==nil{
  t.Fatal("negative cursor was accepted")
 }
}
func TestOutputWindowUTF8NeverSplitsRuneOrInventsOutput(t *testing.T){
 data:=[]byte("ab✓cd")
 s:=&managedSession{scrollback:append([]byte(nil),data...),outputTotal:int64(len(data)),maxScrollback:30}
 m:=&Manager{sessions:map[string]*managedSession{"x":s}}
 first,err:=m.ReadOutputWindow("x",0,3)
 if err!=nil||string(first.Data)!="ab"||first.Next!=2{
  t.Fatalf("rune split/truncation: %+v %v",first,err)
 }
 if _,err:=m.ReadOutputWindow("x",3,2);err==nil{
  t.Fatal("cursor in middle of UTF-8 rune accepted")
 }
 second,err:=m.ReadOutputWindow("x",first.Next,4)
 if err!=nil||string(second.Data)!="✓c"||second.Next!=6{
  t.Fatalf("rune boundary lost bytes: %+v %v",second,err)
 }
 last,err:=m.ReadOutputWindow("x",second.Next,4)
 if err!=nil||string(last.Data)!="d"||last.Next!=int64(len(data)){
  t.Fatalf("final cursor lost data: %+v %v",last,err)
 }
 if !bytes.Equal(append(append(first.Data,second.Data...),last.Data...),data){
  t.Fatal("window sequence does not reconstruct exact UTF-8 bytes")
 }
}
func TestOutputWindowRejectsInvalidUTF8AndLimit(t *testing.T){
 s:=&managedSession{scrollback:[]byte{'a',0xff,'z'},outputTotal:3,maxScrollback:50}
 m:=&Manager{sessions:map[string]*managedSession{"x":s}}
 if _,err:=m.ReadOutputWindow("x",0,3);err==nil{t.Fatal("binary output silently sanitized")}
 if _,err:=m.ReadOutputWindow("x",0,2049);err==nil{t.Fatal("unbounded window accepted")}
 if _,err:=m.ReadOutputWindow("missing",0,3);err==nil{t.Fatal("missing process accepted")}
 // Invalid current data does not corrupt the underlying buffer.
 if !strings.Contains(string(s.scrollback),"z"){t.Fatal("snapshot mutated PTY output")}
}
