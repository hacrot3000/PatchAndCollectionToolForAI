package agentchat

import (
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "sort"
 "strings"
)

// Advisory transport projection only; native Python preflight is authoritative.
const collectionShapeGuidance = `Native COLLECT: typed function calls MUST omit project (TaskDeck binds the verified active identity); standalone text JSON MUST include the exact project object with key and id, never a string. An action with paths requires a non-empty JSON array of project-relative strings, not path or a comma-delimited string. pack/zip require paths; file/range use singular path; range requires start_line and end_line. git.operations is an array of objects with op, e.g. [{"op":"status"}], never ["status"]. For unknown files start with {"actions":[{"type":"overview","path":"."}]} and use observed paths. Never invent paths. On schema rejection repair the exact field, use one smaller action, and never claim the rejected request provided evidence.`

type collectionActionContract struct {
 Required []string `json:"required"`
 Allowed []string `json:"allowed_fields"`
 Operations struct { Allowed []string `json:"allowed"` } `json:"operations"`
}
type nativeCollectionContract struct {
 ToolVersion string `json:"tool_version"`
 Actions map[string]collectionActionContract `json:"actions"`
 FieldShapes map[string]json.RawMessage `json:"provider_field_shapes"`
 Notes map[string]string `json:"provider_action_notes"`
 Examples map[string]json.RawMessage `json:"provider_examples"`
 Limits struct { Allowed []string `json:"allowed_fields"`; Hard map[string]int64 `json:"hard_maximum"` } `json:"limits"`
}

func collectionToolContract(raw string) (map[string]any, string) {
 props := map[string]any{
  "type": map[string]any{"type":"string"},
  "paths": map[string]any{"type":"array","minItems":1,"items":map[string]any{"type":"string"}},
  "path": map[string]any{"type":"string"},
  "operations": map[string]any{"type":"array","minItems":1,"items":map[string]any{"type":"object","properties":map[string]any{"op":map[string]any{"type":"string"}},"required":[]string{"op"},"additionalProperties":true}},
 }
 limitProps := map[string]any{}
 guidance := collectionShapeGuidance
 var native nativeCollectionContract
 if len(raw)>0 && len(raw)<=128<<10 && json.Unmarshal([]byte(raw), &native)==nil && len(native.Actions)>0 {
  names:=make([]string,0,len(native.Actions))
  for name:=range native.Actions { names=append(names,name) }
  sort.Strings(names)
  props["type"]=map[string]any{"type":"string","enum":names}
  allowedFields:=map[string]bool{}
  for _, name:=range names {for _,field:=range native.Actions[name].Allowed {allowedFields[field]=true}}
  for field, shape:=range native.FieldShapes {
   if field!="type" && allowedFields[field] && json.Valid(shape) {props[field]=shape}
  }
  if git,ok:=native.Actions["git"];ok && len(git.Operations.Allowed)>0 && native.FieldShapes["operations"]==nil {
   props["operations"]=map[string]any{"type":"array","minItems":1,"items":map[string]any{"type":"object","properties":map[string]any{"op":map[string]any{"type":"string","enum":git.Operations.Allowed}},"required":[]string{"op"},"additionalProperties":true}}
  }
  for _,field:=range native.Limits.Allowed {
   property:=map[string]any{"type":"integer","minimum":1}
   if ceiling:=native.Limits.Hard[field];ceiling>0{property["maximum"]=ceiling}
   limitProps[field]=property
  }
  fingerprint:=sha256.Sum256([]byte(raw))
  guidance+=fmt.Sprintf("\nInstalled native COLLECT contract version %s, SHA-256 %x. Action fields (required; allowed):",native.ToolVersion,fingerprint)
  for _,name:=range names {
   a:=native.Actions[name]
   guidance+=fmt.Sprintf("\n%s: required=%s; allowed=%s",name,strings.Join(a.Required,","),strings.Join(a.Allowed,","))
   if note:=native.Notes[name];note!=""{guidance+=". "+note}
   if example:=native.Examples[name];len(example)>0{var compact any;if json.Unmarshal(example,&compact)==nil{b,_:=json.Marshal(compact);guidance+=". Example shape: "+string(b)}}
  }
 } else {
  guidance+=" Installed action contract unavailable; use the native schema from the current AI Pack and the simple shapes above, not invented actions."
 }
 return map[string]any{"type":"object","properties":map[string]any{
  "id":map[string]any{"type":"string"},"title":map[string]any{"type":"string"},
  "actions":map[string]any{"type":"array","minItems":1,"items":map[string]any{"type":"object","properties":props,"required":[]string{"type"},"additionalProperties":true}},
  "limits":map[string]any{"type":"object","properties":limitProps,"additionalProperties":true},
  "ai_context":map[string]any{"type":"object","additionalProperties":true},
 },"required":[]string{"actions"},"additionalProperties":false},guidance
}
