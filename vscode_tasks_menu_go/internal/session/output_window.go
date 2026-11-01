package session

import (
 "fmt"
 "unicode/utf8"
)

// OutputWindowReader is an OPTIONAL local TaskDeck session capability.
// Older brokers that cannot prove complete byte offsets fail closed. It
// deliberately does not provide stdin, PTY input or a generic stream.
type OutputWindowReader interface {
 ReadOutputWindow(id string, offset int64, limit int) (OutputWindow,error)
}
type OutputWindow struct {
 Offset int64
 Next int64
 Total int64
 Data []byte
}

// Snapshot and monotonic accounting occur atomically under the same lock
// as appendOutput. Never silently skip an evicted prefix of the scrollback,
// even when the current request is still within the retained suffix.
func (m *Manager) ReadOutputWindow(id string, offset int64, limit int)(OutputWindow,error){
 if m==nil||offset<0||limit<1||limit>2048{
  return OutputWindow{},fmt.Errorf("invalid output cursor")
 }
 s,ok:=m.Get(id)
 if !ok{return OutputWindow{},fmt.Errorf("task session missing")}
 s.mu.Lock()
 defer s.mu.Unlock()
 total:=s.outputTotal
 start:=total-int64(len(s.scrollback))
 if offset<start || offset>total {
  return OutputWindow{},fmt.Errorf("requested output bytes were evicted or cursor is ahead")
 }
 index:=int(offset-start)
 available:=len(s.scrollback)-index
 if available>limit{available=limit}
 if available==0{return OutputWindow{Offset:offset,Next:offset,Total:total},nil}
 // The E2EE terminal console supports UTF-8 text, not binary downloads.
 // A non-boundary cursor or invalid text is an explicit error, not
 // replacement characters or guessed byte offsets.
 if index>0 && !utf8.Valid(s.scrollback[:index]){
  // A dropped prefix may split a codepoint; check the cursor itself.
  if index<len(s.scrollback) && s.scrollback[index]&0xc0==0x80 {
   return OutputWindow{},fmt.Errorf("output cursor splits UTF-8 rune")
  }
 }
 if index<len(s.scrollback)&&s.scrollback[index]&0xc0==0x80{
  return OutputWindow{},fmt.Errorf("output cursor splits UTF-8 rune")
 }
 part:=s.scrollback[index:index+available]
 // A PTY read may end in the middle of a multibyte rune. Trim the
 // requested *end*, never advance past missing output.
 for len(part)>0&&!utf8.Valid(part){part=part[:len(part)-1]}
 if len(part)==0{return OutputWindow{},fmt.Errorf("output contains incomplete or invalid UTF-8")}
 data:=append([]byte(nil),part...)
 return OutputWindow{
  Offset:offset,Next:offset+int64(len(data)),Total:total,Data:data,
 },nil
}
