package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectSymbolsFromTextSupportsCommonLanguages(t *testing.T) {
	tests := []struct {
		path string
		text string
		want []string
	}{
		{"main.go", "package main\ntype Worker struct {}\nfunc (w *Worker) Run() {}\nfunc helper() {}\n", []string{"Worker:struct", "Run:function", "helper:function"}},
		{"app.ts", "export class App {}\nexport async function boot() {}\nconst load = async () => {}\n  render() { return 1 }\n", []string{"App:class", "boot:function", "load:function", "render:method"}},
		{"worker.py", "class Worker:\n    def run(self):\n        pass\nasync def boot():\n    pass\n", []string{"Worker:class", "run:function", "boot:function"}},
		{"Thing.java", "public class Thing {\n public void run() { }\n}\n", []string{"Thing:class", "run:method"}},
		{"api.php", "final class Api {}\npublic function load() {}\n", []string{"Api:class", "load:function"}},
		{"lib.rs", "pub struct Store {}\npub async fn load() {}\nimpl Store {\n}\n", []string{"Store:struct", "load:function", "Store:impl"}},
		{"tool.sh", "build() { echo ok; }\nfunction deploy { echo ok; }\n", []string{"build:function", "deploy:function"}},
		{"tool.ps1", "function Deploy-App { }\n", []string{"Deploy-App:function"}},
		{"module.nim", "proc buildApp*(name: string): int =\n  discard\nThing* = object\n", []string{"buildApp:proc", "Thing:object"}},
		{"module.lua", "local function load_user(id)\nend\nfunction API.start()\nend\n", []string{"load_user:function", "API.start:function"}},
		{"Main.as", "public class Main {\n public function run():void { }\n}\n", []string{"Main:class", "run:function"}},
		{"schema.proto", "message User {}\nservice UserService {}\nrpc GetUser(GetUserRequest) returns (User);\n", []string{"User:message", "UserService:service", "GetUser:method"}},
		{"schema.graphql", "type User { id: ID! }\ninput UserInput { name: String! }\nscalar DateTime\n", []string{"User:type", "UserInput:input", "DateTime:scalar"}},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := projectSymbolsFromText(tc.path, tc.text, "", 100)
			actual := make([]string, 0, len(got))
			for _, item := range got {
				actual = append(actual, item.Name+":"+item.Kind)
				if item.Line < 1 || item.Column < 1 || item.Signature == "" {
					t.Fatalf("invalid symbol coordinates: %+v", item)
				}
			}
			if strings.Join(actual, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("symbols=%v want=%v", actual, tc.want)
			}
		})
	}
}

func TestProjectSymbolQueryScoreRanksExactPrefixContains(t *testing.T) {
	exact, ok := projectSymbolQueryScore("render", "render")
	if !ok { t.Fatal("exact query did not match") }
	prefix, ok := projectSymbolQueryScore("renderPage", "render")
	if !ok { t.Fatal("prefix query did not match") }
	contains, ok := projectSymbolQueryScore("preRenderPage", "render")
	if !ok { t.Fatal("contains query did not match") }
	if !(exact > prefix && prefix > contains) {
		t.Fatalf("scores exact=%d prefix=%d contains=%d", exact, prefix, contains)
	}
	if _, ok := projectSymbolQueryScore("update", "xyz"); ok {
		t.Fatal("unrelated symbol unexpectedly matched")
	}
}

func TestProjectSymbolsExactFileHandler(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "main.ts"), []byte("class Demo {}\nfunction start() {}\n"), 0o644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: root}

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/symbols?path=src%2Fmain.ts", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectSymbolSearchResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if got.ScannedFiles != 1 || len(got.Results) != 2 {
		t.Fatalf("response=%+v", got)
	}
	if got.Results[0].Path != "src/main.ts" || got.Results[0].Name != "Demo" || got.Results[1].Name != "start" {
		t.Fatalf("results=%+v", got.Results)
	}
}

func TestProjectSymbolSearchAcrossAttachedRootsAndIgnore(t *testing.T) {
	primary := t.TempDir()
	attached := t.TempDir()
	if err := os.MkdirAll(filepath.Join(primary, "src"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(primary, "src", "main.go"), []byte("package main\nfunc SharedTarget() {}\nfunc SharedTargetExtra() {}\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "lib"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(attached, "lib", "worker.ts"), []byte("export function SharedTargetWorker() {}\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(attached, "ignored"), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(attached, "ignored", "secret.ts"), []byte("function SharedTargetIgnored() {}\n"), 0o644); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(attached, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil { t.Fatal(err) }
	if err := writeWorkspaceRootStore(primary, workspaceRootStore{
		Version: 1,
		Roots: []workspaceRootRecord{{
			ID: "root-symbol-test", Name: "Attached", Path: attached,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}); err != nil { t.Fatal(err) }

	s := &Server{Workspace: primary}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/project/symbols?q=SharedTarget&limit=20", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got projectSymbolSearchResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil { t.Fatal(err) }
	if len(got.Results) < 3 {
		t.Fatalf("results=%+v", got.Results)
	}
	if got.Results[0].Name != "SharedTarget" || got.Results[0].Path != "src/main.go" {
		t.Fatalf("exact match should rank first: %+v", got.Results)
	}
	seenAttached := false
	for _, item := range got.Results {
		if item.Name == "SharedTargetIgnored" {
			t.Fatalf("ignored source leaked into symbol search: %+v", got.Results)
		}
		if item.Name == "SharedTargetWorker" && item.Path == "@root/root-symbol-test/lib/worker.ts" {
			seenAttached = true
		}
	}
	if !seenAttached {
		t.Fatalf("attached root symbol missing: %+v", got.Results)
	}
	if got.ScannedFiles < 2 || got.ScannedBytes == 0 {
		t.Fatalf("scan accounting missing: %+v", got)
	}
}

func TestProjectSymbolsRejectsTraversalAndBoundsLimit(t *testing.T) {
	root := t.TempDir()
	s := &Server{Workspace: root}
	bad := httptest.NewRecorder()
	s.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/project/symbols?path=..%2Fescape.go", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("traversal status=%d body=%s", bad.Code, bad.Body.String())
	}
	empty := httptest.NewRecorder()
	s.Handler().ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/api/project/symbols", nil))
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "\"results\":[]") {
		t.Fatalf("empty search status=%d body=%s", empty.Code, empty.Body.String())
	}
}
