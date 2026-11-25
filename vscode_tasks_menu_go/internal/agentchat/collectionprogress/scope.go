package collectionprogress

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "strings"
)

const maxFingerprintRequestBytes = 256 << 10
const maxFingerprintDepth = 64

// decodeCanonical rejects ambiguous duplicate JSON keys rather than treating
// them as proof of identical requests. This is fingerprinting, NOT native
// schema validation or an admission decision. Native COLLECT remains the
// authoritative validator.
func decodeCanonical(d *json.Decoder, depth int) (any, error) {
 if depth > maxFingerprintDepth { return nil, errors.New("request fingerprint nesting limit") }
 token, err := d.Token()
 if err != nil { return nil, err }
 switch token {
 case json.Delim('{'):
  obj := map[string]any{}
  for d.More() {
   k, err := d.Token()
   if err != nil { return nil, err }
   key, ok := k.(string)
   if !ok { return nil, errors.New("invalid request key") }
   if _, exists := obj[key]; exists { return nil, errors.New("ambiguous request key") }
   value, err := decodeCanonical(d, depth+1)
   if err != nil { return nil, err }
   obj[key] = value
  }
  end, err := d.Token()
  if err != nil || end != json.Delim('}') { return nil, errors.New("invalid request object") }
  return obj, nil
 case json.Delim('['):
  arr := []any{}
  for d.More() {
   value, err := decodeCanonical(d, depth+1)
   if err != nil { return nil, err }
   arr = append(arr, value)
  }
  end, err := d.Token()
  if err != nil || end != json.Delim(']') { return nil, errors.New("invalid request array") }
  return arr, nil
 default:
  if _, ok := token.(json.Delim); ok { return nil, errors.New("invalid request delimiter") }
  return token, nil
 }
}

func canonicalRequest(raw []byte) (map[string]any, error) {
 if len(raw)==0 || len(raw)>maxFingerprintRequestBytes { return nil, errors.New("request fingerprint size limit") }
 d:=json.NewDecoder(strings.NewReader(string(raw)))
 d.UseNumber()
 v,err:=decodeCanonical(d,0)
 if err!=nil{return nil,err}
 if _,err=d.Token();err!=io.EOF{return nil,errors.New("trailing request data")}
 obj,ok:=v.(map[string]any)
 if !ok{return nil,errors.New("request must be an object")}
 return obj,nil
}

func fingerprintObject(obj map[string]any) (string,error) {
 delete(obj,"id")
 delete(obj,"title")
 if actions,ok:=obj["actions"].([]any);ok {
  for _,v:=range actions {
   if a,ok:=v.(map[string]any);ok {delete(a,"id");delete(a,"title")}
  }
 }
 body,err:=json.Marshal(obj)
 if err!=nil{return "",err}
 digest:=sha256.Sum256(body)
 return hex.EncodeToString(digest[:]),nil
}

// CanonicalRequestFingerprint preserves all execution-affecting fields,
// including action order, path/range/query/symbol, project, limits and
// ai_context. Only display metadata is stripped; numbers are not rounded.
func CanonicalRequestFingerprint(raw []byte)(string,error) {
 obj,err:=canonicalRequest(raw)
 if err!=nil{return "",err}
 return fingerprintObject(obj)
}

// ScopeDescriptor is metadata only. The objective binding already contains
// owner, both workspace identities, project and generation. The source
// revision must be supplied by a trusted caller, never inferred from a
// provider string or an artifact filename.
type ScopeDescriptor struct {
 ObjectiveKey string `json:"objective_key"`
 RequestDigest string `json:"request_digest"`
 SourceRevisionDigest string `json:"source_revision_digest,omitempty"`
}

func BindCollectionScope(objective ObjectiveBinding, projectKey,projectID string, raw []byte, sourceRevisionDigest string)(ScopeDescriptor,error){
 if !objective.Valid()||projectKey==""||projectID==""||(sourceRevisionDigest!=""&&!validDigest(sourceRevisionDigest)){
  return ScopeDescriptor{},errors.New("invalid collection scope binding")
 }
 obj,err:=canonicalRequest(raw)
 if err!=nil{return ScopeDescriptor{},err}
 project,ok:=obj["project"].(map[string]any)
 if !ok||len(project)!=2||project["key"]!=projectKey||project["id"]!=projectID{
  return ScopeDescriptor{},errors.New("request project identity mismatch")
 }
 actions,ok:=obj["actions"].([]any)
 if !ok||len(actions)==0{return ScopeDescriptor{},errors.New("request actions unavailable")}
 digest,err:=fingerprintObject(obj)
 if err!=nil{return ScopeDescriptor{},err}
 return ScopeDescriptor{ObjectiveKey:objective.Key,RequestDigest:digest,SourceRevisionDigest:sourceRevisionDigest},nil
}

// SameVerifiedScope is necessary but NOT sufficient to skip a native read.
// Coverage and receipt proof still have to be checked by the caller. Missing
// source revision never licenses cache reuse.
func (s ScopeDescriptor) SameVerifiedScope(other ScopeDescriptor)bool {
 return validDigest(s.ObjectiveKey)&&validDigest(s.RequestDigest)&&validDigest(s.SourceRevisionDigest)&&s==other
}
