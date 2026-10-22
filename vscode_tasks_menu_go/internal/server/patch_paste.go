package server

import (
 "archive/zip"
 "bytes"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "regexp"
 "strings"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

const maxPatchPasteBytes = 8 << 20

type patchPasteRequest struct {
 Kind string `json:"kind"`
 Name string `json:"name"`
 Text string `json:"text"`
}

var patchPasteSlugRE = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// unwrapPasteFence allows a text-only AI to wrap its entire response in one
// Markdown code fence without including that fence in the executable payload.
func unwrapPasteFence(input string) string {
 s := strings.TrimSpace(strings.TrimPrefix(input, "\ufeff"))
 if !strings.HasPrefix(s, "```") { return s }
 line := strings.IndexByte(s, '\n')
 if line < 0 || !strings.HasSuffix(s, "```") { return s }
 inner := strings.TrimSpace(s[line+1:len(s)-3])
 if strings.Contains(inner, "```") { return s }
 return inner
}

func patchPasteSlug(value string) string {
 value = patchPasteSlugRE.ReplaceAllString(strings.TrimSpace(value), "-")
 value = strings.Trim(value, "-_")
 if len(value) > 48 { value = value[:48] }
 if value == "" { value = "text" }
 return value
}

func parsePastedCollectJSON(input string) ([]byte, error) {
 text := unwrapPasteFence(input)
 if !strings.HasPrefix(text, "{") {
  // Python is never executed for COLLECT. Only a declared literal is read.
  assignment := regexp.MustCompile(`(?m)^\s*COLLECT_REQUEST_JSON\s*=`)
  loc := assignment.FindStringIndex(text)
  if loc == nil { return nil, fmt.Errorf("COLLECT requires JSON or a Python COLLECT_REQUEST_JSON triple-quoted literal; arbitrary Python is not executed") }
  expression := strings.TrimSpace(text[loc[1]:])
  expression = strings.TrimPrefix(expression, "r")
  expression = strings.TrimPrefix(expression, "R")
  var quote string
  if strings.HasPrefix(expression, "\"\"\"") { quote = "\"\"\"" } else if strings.HasPrefix(expression, "'''") { quote = "'''" } else {
   return nil, fmt.Errorf("COLLECT_REQUEST_JSON must use triple quotes")
  }
  expression = expression[len(quote):]
  end := strings.Index(expression, quote)
  if end < 0 { return nil, fmt.Errorf("unclosed COLLECT_REQUEST_JSON literal") }
  text = expression[:end]
 }
 if !utf8.ValidString(text) || len(text) > maxPatchPasteBytes { return nil, fmt.Errorf("COLLECT text must be UTF-8 within 8 MiB") }
 return []byte(text), nil
}

func ensurePastedIdentity(project json.RawMessage, identity patchtool.ProjectIdentity) error {
 var found struct { Key string `json:"key"`; ID string `json:"id"` }
 if len(project) == 0 || string(project) == "null" { return fmt.Errorf("missing project.key/project.id; copy both values from the current AI Pack") }
 if err := json.Unmarshal(project, &found); err != nil { return fmt.Errorf("invalid project identity: %w", err) }
 if found.Key == "" || found.ID == "" || found.Key != identity.Key || found.ID != identity.ID {
  return fmt.Errorf("project identity does not match this TaskDeck workspace (both key and id must match)")
 }
 return nil
}

func pastedCollectionPackage(data []byte, identity patchtool.ProjectIdentity, name string) ([]byte, error) {
 var request struct {
  Project json.RawMessage `json:"project"`
  Actions []json.RawMessage `json:"actions"`
 }
 if err := json.Unmarshal(data, &request); err != nil { return nil, fmt.Errorf("invalid COLLECT JSON: %w", err) }
 if err := ensurePastedIdentity(request.Project, identity); err != nil { return nil, err }
 if len(request.Actions) == 0 { return nil, fmt.Errorf("COLLECT requires at least one action") }
 var packed bytes.Buffer
 zw := zip.NewWriter(&packed)
 member, err := zw.Create("CODE_COLLECTION_REQUEST_" + name + ".json")
 if err != nil { return nil, err }
 if _, err = member.Write(data); err != nil { return nil, err }
 if err = zw.Close(); err != nil { return nil, err }
 return packed.Bytes(), nil
}

func pastedPythonPackage(src string, identity patchtool.ProjectIdentity, name string) ([]byte, error) {
 src = unwrapPasteFence(src)
 if !utf8.ValidString(src) || len(src) > maxPatchPasteBytes { return nil, fmt.Errorf("PATCH Python must be UTF-8 within 8 MiB") }
 if strings.TrimSpace(src) == "" || strings.ContainsRune(src, 0) { return nil, fmt.Errorf("PATCH Python is empty or contains NUL") }
 // Optional identity declarations must agree with the active workspace.
 for _, pair := range []struct { key, expected string }{
  {"TASKDECK_PROJECT_KEY:", identity.Key}, {"TASKDECK_PROJECT_ID:", identity.ID},
 } {
  header := regexp.MustCompile(`(?m)^\s*#\s*`+pair.key+`\s*(\S+)\s*$`)
  for _, found := range header.FindAllStringSubmatch(src, -1) {
   if found[1] != pair.expected { return nil, fmt.Errorf("Python %s header does not match this project", pair.key) }
  }
 }
 manifest := map[string]any{
  "schema_version": 1,
  "project": map[string]string{"key":identity.Key, "id":identity.ID},
  "patch":map[string]string{"id":"cleartext-"+name, "summary":"Text-only AI Python PATCH; review before execution"},
 }
 if !strings.HasSuffix(src, "\n") { src += "\n" }
 manifestBytes, err := json.MarshalIndent(manifest,"","  ")
 if err != nil { return nil, err }
 var packed bytes.Buffer
 zw := zip.NewWriter(&packed)
 for _, entry := range []struct{ path string; data []byte }{
  {"PATCH_TOOL_MANIFEST.json",append(manifestBytes, '\n')},
  {"entrypoint.py",[]byte(src)},
 } {
  f, err := zw.Create(entry.path)
  if err != nil { return nil, err }
  if _, err = f.Write(entry.data); err != nil { return nil, err }
 }
 if err = zw.Close(); err != nil { return nil, err }
 return packed.Bytes(), nil
}

// patchPaste only packages already supplied source text. It does not evaluate
// pasted Python, change project source, or bypass the normal queue/preflight.
func (s *Server) patchPaste(w http.ResponseWriter, r *http.Request) {
 if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
 r.Body = http.MaxBytesReader(w, r.Body, maxPatchPasteBytes+4096)
 defer r.Body.Close()
 if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json") {
  http.Error(w, "JSON Content-Type required", http.StatusUnsupportedMediaType); return
 }
 dec := json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 var request patchPasteRequest
 if err := dec.Decode(&request); err != nil { http.Error(w, "invalid paste request: "+err.Error(), http.StatusBadRequest); return }
 var extra any
 if err := dec.Decode(&extra); err != io.EOF { http.Error(w, "exactly one paste request is required", http.StatusBadRequest); return }
 if !utf8.ValidString(request.Text) || len(request.Text) == 0 || len(request.Text) > maxPatchPasteBytes {
  http.Error(w, "paste must contain valid UTF-8 text within 8 MiB", http.StatusBadRequest); return
 }
 identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
 if err != nil { http.Error(w, "project identity unavailable: "+err.Error(), http.StatusConflict); return }
 slug := patchPasteSlug(request.Name)
 var body []byte
 switch request.Kind {
 case "patch-python":
  body, err = pastedPythonPackage(request.Text, identity, slug)
 case "collect":
  var data []byte
  data, err = parsePastedCollectJSON(request.Text)
  if err == nil { body, err = pastedCollectionPackage(data, identity, slug) }
 default:
  http.Error(w, "paste kind must be patch-python or collect", http.StatusBadRequest); return
 }
 if err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
 dir, err := s.patchUploadDir()
 if err != nil { http.Error(w, "patchs queue directory unavailable or unsafe", http.StatusConflict); return }
 var salt [4]byte
 if _, err := rand.Read(salt[:]); err != nil { http.Error(w, "cannot generate unique queue name", http.StatusInternalServerError); return }
 prefix := "PATCH_CLEAR_TEXT_"
 if request.Kind == "collect" { prefix = "CODE_COLLECTION_REQUEST_CLEAR_TEXT_" }
 filename := fmt.Sprintf("%s%s_%s_%s.zip", prefix, slug, time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(salt[:]))
 resourceID := filepath.ToSlash(filepath.Join("patchs", filename))
 lease, ok := s.acquireSharedMutation(w, r, "patch.upload", resourceID)
 if !ok { return }
 defer s.releaseSharedMutation(lease)
 target := filepath.Join(dir, filename)
 // Publish only a completely written ZIP. A queue refresh must never
 // discover a partially written manifest or Python payload.
 file, err := os.CreateTemp(dir, ".taskdeck-patch-paste-*")
 if err != nil { http.Error(w, "cannot create PATCH/COLLECT queue temp file", http.StatusInternalServerError); return }
 tempName := file.Name()
 defer os.Remove(tempName)
 if _, err = file.Write(body); err != nil { file.Close(); http.Error(w,"queue write failed",http.StatusInternalServerError); return }
 if err = file.Sync(); err != nil { file.Close(); http.Error(w,"queue sync failed",http.StatusInternalServerError); return }
 if err = file.Chmod(0o644); err != nil { file.Close(); http.Error(w,"queue chmod failed",http.StatusInternalServerError); return }
 if err = file.Close(); err != nil { http.Error(w,"queue close failed",http.StatusInternalServerError); return }
 if _, err = os.Lstat(target); err == nil { http.Error(w,"queue target already exists",http.StatusConflict); return } else if !os.IsNotExist(err) {
  http.Error(w,"queue target unavailable",http.StatusConflict); return
 }
 if err = os.Rename(tempName,target); err != nil { http.Error(w,"queue publish failed",http.StatusInternalServerError); return }
 digest := sha256.Sum256(body)
 s.auditSharedSuccess(r,"patch.upload","patch_package",resourceID,map[string]any{"input_mode":"paste","kind":request.Kind,"size":len(body)})
 writeJSON(w,http.StatusCreated,patchUploadResult{
  Name:filename, QueuePath:resourceID, Size:int64(len(body)), SHA256:hex.EncodeToString(digest[:]),
 })
}
