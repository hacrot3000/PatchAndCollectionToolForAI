package remoteportal

import (
 "strings"
 "testing"
)

func TestCompromisedPHPGrantVerdictExactBooleanSchema(t *testing.T){
 for _,wire:=range []string{
  `{"ok":true,"allowed":true}`,
  "  {  \"allowed\" : true, \"ok\" : true }\n",
 }{
  if err:=verifyStrictPortalGrantVerdict([]byte(wire));err!=nil{
   t.Fatalf("valid exact PHP response rejected: %v",err)
  }
 }
 cases:=map[string]string{
  "denial":`{"ok":true,"allowed":false}`,
  "not ok":`{"ok":false,"allowed":true}`,
  "missing allowed":`{"ok":true}`,
  "missing ok":`{"allowed":true}`,
  "duplicated allow":`{"ok":true,"allowed":false,"allowed":true}`,
  "duplicated ok":`{"ok":false,"ok":true,"allowed":true}`,
  "case alias":`{"ok":true,"Allowed":true,"allowed":true}`,
  "uppercase ok":`{"OK":true,"allowed":true}`,
  "extra session token":`{"ok":true,"allowed":true,"taskdeck_cookie":"attacker"}`,
  "extra role":`{"ok":true,"allowed":true,"role":"admin"}`,
  "null":`{"ok":true,"allowed":null}`,
  "string literal":`{"ok":true,"allowed":"true"}`,
  "number":`{"ok":true,"allowed":1}`,
  "array":`{"ok":true,"allowed":[true]}`,
  "trailing object":`{"ok":true,"allowed":true}{"ok":true,"allowed":true}`,
  "not object":`[{"ok":true,"allowed":true}]`,
  "oversized":`{"ok":true,"allowed":true}`+strings.Repeat(" ",260),
 }
 for name,wire:=range cases{
  t.Run(name,func(t *testing.T){
   if err:=verifyStrictPortalGrantVerdict([]byte(wire));err==nil {
    t.Fatalf("compromised Portal JSON bypassed strict schema: %q",wire)
   }
  })
 }
}
