package main

import (
 "errors"
 "strings"
 "unicode/utf8"
)

// consoleSourceLine stores byte offsets in the FULL authenticated preview.
// A newline belongs to the line immediately before it; the final newline
// never creates a phantom editable line.
type consoleSourceLine struct {
 start int
 contentEnd int
 end int
 ending string
}

func consoleSourceLines(data string) ([]consoleSourceLine,error) {
 if len(data)>4096||!utf8.ValidString(data) {
  return nil,errors.New("invalid authenticated text preview")
 }
 spans:=make([]consoleSourceLine,0,16)
 for start:=0;start<len(data); {
  stop:=len(data)
  end:=len(data)
  ending:=""
  if relative:=strings.IndexByte(data[start:],'\n');relative>=0 {
   stop=start+relative
   end=stop+1
   ending="\n"
   if stop>start&&data[stop-1]=='\r' {
    stop--
    ending="\r\n"
   }
  }
  spans=append(spans,consoleSourceLine{
   start:start,contentEnd:stop,end:end,ending:ending,
  })
  start=end
 }
 return spans,nil
}

// Pick the original nearby EOL style. Never normalize the rest of the file.
func consoleLineEnding(spans []consoleSourceLine,index int) string {
 if index>=0&&index<len(spans)&&spans[index].ending!="" {
  return spans[index].ending
 }
 for i:=len(spans)-1;i>=0;i-- {
  if spans[i].ending!="" {return spans[i].ending}
 }
 return "\n"
}

// Insert before existing one-based line N or append with N==lineCount+1.
// Empty files allow N=1. The same host-side 4KiB SHA-256 CAS operation
// protects this complete result; no new remote mutator is introduced.
func consoleInsertLine(data string,index int,newLine string)(string,error) {
 spans,err:=consoleSourceLines(data)
 if err!=nil||index<1||index>len(spans)+1||
  !validConsoleLiteral(newLine,true) {
  return "",errors.New("invalid line insertion")
 }
 var result string
 if len(spans)==0 {
  // A single empty line is distinct from an empty file.
  result=newLine+"\n"
 } else if index<=len(spans) {
  pos:=spans[index-1].start
  separator:=consoleLineEnding(spans,index-1)
  result=data[:pos]+newLine+separator+data[pos:]
 } else {
  separator:=consoleLineEnding(spans,len(spans)-1)
  if spans[len(spans)-1].ending!=""{
   // Keep the original last line terminated.
   result=data+newLine+separator
  }else{
   // Keep the originally unterminated final line unterminated.
   result=data+separator+newLine
  }
 }
 if len(result)>4096||result==data {
  return "",errors.New("line insertion over 4096 bytes or no change")
 }
 return result,nil
}

// Delete exactly one existing source line. For an unterminated last line,
// also drop its preceding EOL so deleting "b" from "a\nb" leaves "a",
// not "a\n". Other bytes and original EOL style are untouched.
func consoleDeleteLine(data string,index int)(string,string,error) {
 spans,err:=consoleSourceLines(data)
 if err!=nil||index<1||index>len(spans) {
  return "","",errors.New("invalid line deletion")
 }
 target:=spans[index-1]
 removed:=data[target.start:target.contentEnd]
 var result string
 if target.ending!="" {
  result=data[:target.start]+data[target.end:]
 } else if index>1 {
  prior:=spans[index-2]
  result=data[:prior.contentEnd]
 } else {
  result=""
 }
 if result==data {
  return "","",errors.New("line deletion produced no change")
 }
 return removed,result,nil
}


// Replace inclusive one-based source lines with literal UTF-8 lines. Only the
// selected lines are rewritten; all bytes before/after the range and the
// selected range's final line-ending remain unchanged. Incoming LF separators
// use the EOL style at the selection; a trailing LF inserts a visible blank
// line, never silently changes the original EOF policy.
func consoleReplaceLineRange(data string, first, last int, replacement string) (string, error) {
 spans, err := consoleSourceLines(data)
 if err != nil || first < 1 || last < first || last > len(spans) ||
  len(replacement) > 4096 || !utf8.ValidString(replacement) {
  return "", errors.New("invalid authenticated line range")
 }
 for _, line := range strings.Split(replacement, "\n") {
  if !validConsoleLiteral(line, true) {
   return "", errors.New("range contains control or invisible characters")
  }
 }
 // A trailing LF in the typed replacement is only meaningful before an
 // existing terminator. Refuse it at an unterminated EOF instead of silently
 // changing EOF topology or inventing a phantom editable source line.
 if spans[last-1].ending == "" && strings.HasSuffix(replacement, "\n") {
  return "", errors.New("range edit would change unterminated EOF")
 }
 eol := consoleLineEnding(spans, first-1)
 expanded := strings.ReplaceAll(replacement, "\n", eol)
 from := spans[first-1].start
 through := spans[last-1]
 next := data[:from] + expanded + through.ending + data[through.end:]
 if len(next) > 4096 || next == data {
  return "", errors.New("range edit exceeds 4096 bytes or changes nothing")
 }
 return next, nil
}
