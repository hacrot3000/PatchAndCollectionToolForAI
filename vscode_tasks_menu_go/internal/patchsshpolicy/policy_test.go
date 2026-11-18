package patchsshpolicy

import (
	"strings"
	"testing"
)

func TestRemoteWhitelistAllowsReadOnlyCommands(t *testing.T) {
	cases := [][]string{
		{"pwd"}, {"ls"}, {"ls", "-lah", "src"}, {"ls", "-lah", "My project"},
		{"tree", "-L", "2", "./src"}, {"tree", "-a", "-d", "."},
		{"find", ".", "-maxdepth", "3", "-type", "f"},
		{"find", "src", "-maxdepth", "1"},
		{"find", ".", "-maxdepth", "3", "-iname", "*.go", "-type", "f"},
		{"find", ".", "-maxdepth", "2", "-mindepth", "1", "-name", "*.md", "-print"},
		{"grep", "-nriF", "--include=*.go", "--", "some text", "src"},
		{"grep", "-n", "--", "-error", "src/error.log"},
		{"grep", "-rn", "--exclude=*.log", "TODO", "."},
		{"rg", "-n", "-i", "-g", "*.go", "--", "MyFunc(", "src"},
		{"rg", "-n", "--hidden", "--", "func main", "."},
		{"rg", "-n", "--", "$(id); never execute", "src"},
		{"cat", "src/file with space.txt"},
		{"head", "-n", "50", "file.txt"},
		{"tail", "-n", "10", "file.txt"},
		{"wc", "-l", "file.txt", "src/second.txt"},
		{"sed", "-n", "10,25p", "src/main.go"},
		{"nl", "-ba", "src/main.go"},
	}
	for _, argv := range cases {
		got, err := Validate(argv)
		if err != nil || got == "" {
			t.Fatalf("%v: got %q, error %v", argv, got, err)
		}
	}
}

func TestRemoteSearchPatternsAreShellQuoted(t *testing.T) {
	got, err := Validate([]string{"rg", "-n", "--", "It's $(id); echo BAD", "My project/test file.go"})
	if err != nil {
		t.Fatal(err)
	}
	want := "rg -n -- 'It'\\''s $(id); echo BAD' 'My project/test file.go'"
	if got != want {
		t.Fatalf("unsafe quoting: got %q want %q", got, want)
	}
	if strings.Contains(got, "; echo BAD;") {
		t.Fatalf("unquoted shell command leaked: %q", got)
	}
}

func TestRemoteWhitelistRejectsExecutableScriptAndInjection(t *testing.T) {
	cases := [][]string{
		nil, {"bash", "check.sh"}, {"sh", "script.sh"}, {"python3", "x.py"},
		{"python", "-c", "print(1)"}, {"node", "x.js"}, {"pwsh", "script.ps1"},
		{"./script.sh"}, {"git", "status"}, {"curl", "https://x"},
		{"ls", ";", "id"}, {"ls", "$(id)"}, {"ls", "src;id"},
		{"ls", "../parent"}, {"ls", "/etc"}, {"ls", "-R"}, {"ls", "--color=always"},
		{"tree", "-o", "log.txt"}, {"tree", "-L", "0"}, {"tree", "-L", "100"},
		{"find", "."}, {"find", ".", "-maxdepth", "4"},
		{"find", ".", "-maxdepth", "2", "-exec", "id"},
		{"find", ".", "-maxdepth", "2", "-execdir", "bash"},
		{"find", ".", "-maxdepth", "2", "-delete"},
		{"find", ".", "-maxdepth", "2", "-fprint", "output.txt"},
		{"find", ".", "-maxdepth", "2", "-type", "l"},
		{"find", ".", "-maxdepth", "2", "-name", "$(id)"},
		{"ls", "src", "-la"}, {"pwd", "garbage"},
		{"grep", "-f", "payload.sh", "file.txt"},
		{"grep", "--binary-files=text", "secret", "."},
		{"grep", "--include=../../../etc", "foo", "."},
		{"rg", "--pre", "script.sh", "text", "."},
		{"rg", "-e", "pattern", "."},
		{"rg", "-g", "-unsafe", "pattern", "."},
		{"rg", "text", "../secrets"},
		{"grep", "text", "/etc/passwd"},
		{"cat", "-"}, {"cat", "../../keys"},
		{"head", "-c", "100", "file.txt"},
		{"head", "-n", "99999", "file.txt"},
		{"tail", "-f", "logfile"},
		{"wc", "--files0-from=secrets", "file"},
		{"nl", "-ba", "-"},
		{"sed", "-i", "1p", "file"},
		{"sed", "-n", "1;d", "file"},
		{"sed", "-n", "20,10p", "file"},
	}
	for _, argv := range cases {
		if command, err := Validate(argv); err == nil {
			t.Fatalf("unsafe argv accepted: %v -> %q", argv, command)
		}
	}
}
