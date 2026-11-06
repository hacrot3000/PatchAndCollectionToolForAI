package server

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTerminalDetectsReleasePathsWithSpacesAndDecoratedLabels(t *testing.T) {
	workspace := t.TempDir()
	project := filepath.Join(workspace, "Documents", "My project", "Datbike", "dubike")
	paths := []string{
		filepath.Join(project, "app", "build", "outputs", "bundle", "release", "app-release.aab"),
		filepath.Join(project, "app", "build", "outputs", "apk", "release", "app-release.apk"),
		filepath.Join(project, "wear", "build", "outputs", "bundle", "release", "wear-release.aab"),
		filepath.Join(project, "wear", "build", "outputs", "apk", "release", "wear-release.apk"),
		filepath.Join(project, "wear", "build", "outputs", "mapping", "release", "mapping.txt"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mappingDir := filepath.Join(project, "release_version", "play-mapping")
	if err := os.MkdirAll(mappingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := strings.Join([]string{
		"🔄 Đã đồng bộ whats_new.json vào html/whats_new.json",
		"--------------------------------------------------------",
		"✅ HOÀN THÀNH!",
		"📍 File của bạn đã sẵn sàng tại:",
		"📱 Phone: versionCode=174",
		"⌚ Wear:  versionCode=175",
		"👉 " + paths[0],
		"👉 " + paths[1],
		"👉 " + paths[2],
		"👉 " + paths[3],
		"🧩 Wear R8 mapping: " + paths[4],
		"🗃️  Bản sao mapping theo AAB: " + mappingDir + "/",
		"Task kết thúc với mã trả về: 0",
	}, "\n")
	files := (&Server{Workspace: workspace}).downloadableFilesFromText(log)
	got := make([]string, 0, len(files))
	for _, file := range files {
		got = append(got, file.Path)
		if file.Name != filepath.Base(file.Path) {
			t.Fatalf("wrong basename for %q: %q", file.Path, file.Name)
		}
		if !strings.Contains(file.URL, "%20") {
			t.Fatalf("URL should encode spaces: %q", file.URL)
		}
		parsed, err := url.Parse(file.URL)
		if err != nil || parsed.Query().Get("path") != file.Path {
			t.Fatalf("URL does not round-trip file path %q: %s, %v", file.Path, file.URL, err)
		}
	}
	if !reflect.DeepEqual(got, paths) {
		t.Fatalf("detected paths = %#v; want %#v", got, paths)
	}
}

func TestTerminalDetectsQuotedEscapedAndSpecialCharacterPaths(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "My project", "build #1 [beta] & 100%.apk")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"👉 " + path,
		"Output: \"" + path + "\"",
		"Output: '" + path + "'",
		"👉 " + strings.ReplaceAll(path, " ", "\\ "),
		"Result: " + strings.TrimPrefix(path, workspace+string(filepath.Separator)),
	} {
		files := (&Server{Workspace: workspace}).downloadableFilesFromText(line)
		if len(files) != 1 || files[0].Path != path {
			t.Errorf("line %q: detected %#v, want %q", line, files, path)
		}
	}
}

func TestTerminalPreservesFilenameEndingInBracketAndWorkspaceBoundary(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "report [done]")
	if err := os.WriteFile(inside, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret report.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		line string
		want string
	}{
		{"👉 " + inside, inside},
		{"👉 " + outside, ""},
	} {
		got := (&Server{Workspace: workspace}).downloadableFilesFromText(tc.line)
		if tc.want == "" {
			if len(got) != 0 {
				t.Fatalf("outside workspace path was detected: %#v", got)
			}
		} else if len(got) != 1 || got[0].Path != tc.want {
			t.Fatalf("line %q: detected %#v; want %q", tc.line, got, tc.want)
		}
	}
}
