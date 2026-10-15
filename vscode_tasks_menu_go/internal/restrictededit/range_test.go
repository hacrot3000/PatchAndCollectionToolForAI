package restrictededit

import (
 "bytes"
 "errors"
 "strings"
 "testing"
)

func TestReplaceExactlyOneViewedRegionOnly(t *testing.T){
 original:=[]byte("SECRET_ABOVE\nline2\nline3\nSECRET_BELOW\n")
 next,err:=ReplaceLines(original,2,[]string{"updated2","updated3"},5)
 if err!=nil{t.Fatal(err)}
 if string(next)!="SECRET_ABOVE\nupdated2\nupdated3\nSECRET_BELOW\n"{
  t.Fatalf("wrong replacement: %q",next)
 }
 if !bytes.Equal(original,[]byte("SECRET_ABOVE\nline2\nline3\nSECRET_BELOW\n")){
  t.Fatal("input modified in-place")
 }
}
func TestPreserveMixedLineEndingsAndFinalNoNewline(t *testing.T){
 source:=[]byte("first\r\nsecond\nlast")
 got,err:=ReplaceLines(source,1,[]string{"one","two","three"},3)
 if err!=nil{t.Fatal(err)}
 if string(got)!="one\r\ntwo\nthree"{t.Fatalf("line separators lost: %q",got)}
 if len(SplitLines([]byte("a\nb\n")))!=2{t.Fatal("terminal newline added an extra physical line")}
}
func TestRejectWindowEscapesAndTextInjection(t *testing.T){
 original:=[]byte("a\nb\nc")
 for _,tc:=range []struct{start,limit int; lines []string}{
  {0,3,[]string{"x"}},{4,3,[]string{"x"}},{3,3,[]string{"x","y"}},
  {1,1,[]string{"x","y"}},{1,3,nil},
  {1,3,[]string{"x\nexfil"}},{1,3,[]string{"x\rbroken"}},
  {1,3,[]string{"x\x00bad"}},{1,3,[]string{string([]byte{0xff})}},
  {1,3,[]string{strings.Repeat("a",MaxSingleLineBytes+1)}},
 }{
  _,err:=ReplaceLines(original,tc.start,tc.lines,tc.limit)
  if !errors.Is(err,ErrInvalidRange)&&!errors.Is(err,ErrInvalidText){
   t.Fatalf("accepted unsafe edit %+v: %v",tc,err)
  }
 }
}
func TestRejectBulkEditAndAllowIdenticalLineText(t *testing.T){
 src:=[]byte("same\nsame\nsame\n")
 after,err:=ReplaceLines(src,2,[]string{"same"},1)
 if err!=nil||!bytes.Equal(after,src){t.Fatalf("unchanged line failed: %v",err)}
 _,err=ReplaceLines(src,1,[]string{strings.Repeat("x",MaxSingleLineBytes),
  strings.Repeat("x",MaxSingleLineBytes),strings.Repeat("x",MaxSingleLineBytes)},2)
 if !errors.Is(err,ErrInvalidRange){t.Fatalf("large window not denied: %v",err)}
}
