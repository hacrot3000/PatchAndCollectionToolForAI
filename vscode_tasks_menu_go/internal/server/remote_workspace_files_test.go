package server

import "testing"

func TestRemoteWorkspaceRelativePathRejectsEscapeAndAbsolute(t *testing.T) {
	for _, value := range []string{"../etc/passwd", "a/../../etc/passwd", "/etc/passwd", "a\x00b", "a\nb"} {
		if _, err := remoteWorkspaceRelativePath(value); err == nil {
			t.Fatalf("path %q unexpectedly accepted", value)
		}
	}
	for input, want := range map[string]string{
		"": ".",
		".": ".",
		"src/main.go": "src/main.go",
		"src/./pkg/../main.go": "src/main.go",
		"src\\main.go": "src/main.go",
	} {
		got, err := remoteWorkspaceRelativePath(input)
		if err != nil {
			t.Fatalf("path %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("path %q normalized=%q want=%q", input, got, want)
		}
	}
}

func TestRemoteWorkspaceResolvePathStaysUnderConfiguredRoot(t *testing.T) {
	tests := []struct {
		root string
		rel  string
		want string
	}{
		{"/srv/app", ".", "/srv/app"},
		{"/srv/app", "src/main.go", "/srv/app/src/main.go"},
		{"/srv/app/", "src/../README.md", "/srv/app/README.md"},
		{"/", "etc/hosts", "/etc/hosts"},
	}
	for _, test := range tests {
		got, err := remoteWorkspaceResolvePath(test.root, test.rel)
		if err != nil {
			t.Fatalf("resolve root=%q rel=%q: %v", test.root, test.rel, err)
		}
		if got != test.want {
			t.Fatalf("resolve root=%q rel=%q got=%q want=%q", test.root, test.rel, got, test.want)
		}
	}
	for _, rel := range []string{"../secret", "../../secret", "/absolute"} {
		if _, err := remoteWorkspaceResolvePath("/srv/app", rel); err == nil {
			t.Fatalf("escape %q unexpectedly accepted", rel)
		}
	}
}

func TestRemoteWorkspaceFileBridgeKeepsRelativeBrowserContract(t *testing.T) {
	if got, err := remoteWorkspaceResolvePath("/srv/project", "nested/file.txt"); err != nil || got != "/srv/project/nested/file.txt" {
		t.Fatalf("resolved path=%q err=%v", got, err)
	}
}
