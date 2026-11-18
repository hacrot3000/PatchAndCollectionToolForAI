package patchsshpolicy

import "testing"

func TestRemoteWhitelistAllowsReadOnlyCommands(t *testing.T) {
	cases := [][]string{
		{"pwd"}, {"ls"}, {"ls", "-lah", "src"},
		{"tree", "-L", "2", "./src"}, {"tree", "-a", "-d", "."},
		{"find", ".", "-maxdepth", "3", "-type", "f"},
		{"find", "src", "-maxdepth", "1"},
	}
	for _, argv := range cases {
		got, err := Validate(argv)
		if err != nil || got == "" {
			t.Fatalf("%v: got %q, error %v", argv, got, err)
		}
	}
}

func TestRemoteWhitelistRejectsExecutableScriptAndInjection(t *testing.T) {
	cases := [][]string{
		nil, {"bash", "check.sh"}, {"sh", "script.sh"}, {"python3", "x.py"},
		{"python", "-c", "print(1)"}, {"node", "x.js"}, {"pwsh", "script.ps1"},
		{"./script.sh"}, {"git", "status"}, {"curl", "https://x"},
		{"ls", ";", "id"}, {"ls", "$(id)"}, {"ls", "src;id"}, {"ls", "a b"},
		{"ls", "../parent"}, {"ls", "/etc"}, {"ls", "-R"}, {"ls", "--color=always"},
		{"tree", "-o", "log.txt"}, {"tree", "-L", "0"}, {"tree", "-L", "100"},
		{"find", "."}, {"find", ".", "-maxdepth", "4"},
		{"find", ".", "-maxdepth", "2", "-exec", "id"},
		{"find", ".", "-maxdepth", "2", "-delete"},
		{"find", ".", "-maxdepth", "2", "-type", "l"},
		{"ls", "src", "-la"}, {"pwd", "garbage"},
	}
	for _, argv := range cases {
		if command, err := Validate(argv); err == nil {
			t.Fatalf("unsafe argv accepted: %v -> %q", argv, command)
		}
	}
}
