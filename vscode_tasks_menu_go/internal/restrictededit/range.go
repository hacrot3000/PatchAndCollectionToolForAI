// Package restrictededit applies bounded, line-preserving edits to protected
// source windows. It never discloses surrounding file contents to callers.
package restrictededit

import (
 "bytes"
 "errors"
 "strings"
 "unicode/utf8"
)

var ErrInvalidRange=errors.New("invalid protected edit range")
var ErrInvalidText=errors.New("invalid protected replacement text")

const MaxReplacementBytes=64<<10
const MaxSingleLineBytes=16<<10

// SplitLines retains the newline bytes of each physical line. A trailing
// newline is not treated as an additional empty line.
func SplitLines(data []byte) [][]byte{
 if len(data)==0{return nil}
 lines:=bytes.SplitAfter(data,[]byte("\n"))
 if len(lines)>0&&len(lines[len(lines)-1])==0{lines=lines[:len(lines)-1]}
 return lines
}

// ReplaceLines replaces ONLY existing physical lines. Every original newline
// style (LF, CRLF or no final newline) is retained. Insert/delete operations
// are intentionally not supported: they would shift previously authorized
// source positions and risk granting access outside the visible window.
func ReplaceLines(current []byte,start int,replacement []string,maxLines int)([]byte,error){
 lines:=SplitLines(current)
 if start<1||len(replacement)==0||len(replacement)>maxLines||
  start>len(lines)||len(replacement)>len(lines)-start+1 {
  return nil,ErrInvalidRange
 }
 bytesIn:=0
 hasBOM:=bytes.HasPrefix(current,[]byte{0xEF,0xBB,0xBF})
 for i,text:=range replacement{
  if !utf8.ValidString(text)||bytes.IndexAny([]byte(text),"\r\n\x00")>=0||len(text)>MaxSingleLineBytes{
   return nil,ErrInvalidText
  }
  // A pre-existing UTF-8 BOM is an encoding marker, not editable content.
  // Tolerate its presence in a first-line editor snapshot without duplicating
  // it; reject attempts to inject a BOM anywhere else.
  checked:=text
  if hasBOM&&start==1&&i==0 {checked=strings.TrimPrefix(text,"\uFEFF")}
  if strings.ContainsRune(checked,'\uFEFF'){return nil,ErrInvalidText}
  bytesIn+=len(text)
  if bytesIn>MaxReplacementBytes{return nil,ErrInvalidText}
 }
 next:=make([]byte,0,len(current)+bytesIn)
 for i,line:=range lines {
  if i<start-1||i>=start-1+len(replacement){
   next=append(next,line...)
   continue
  }
  suffix:=""
  if bytes.HasSuffix(line,[]byte("\r\n")){suffix="\r\n"
  }else if bytes.HasSuffix(line,[]byte("\n")){suffix="\n"}
  text:=replacement[i-(start-1)]
  if i==0&&hasBOM {
   next=append(next,0xEF,0xBB,0xBF)
   text=strings.TrimPrefix(text,"\uFEFF")
  }
  next=append(next,text...)
  next=append(next,suffix...)
 }
 if !utf8.Valid(next){return nil,ErrInvalidText}
 return next,nil
}
