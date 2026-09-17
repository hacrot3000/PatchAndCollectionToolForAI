package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestEnvironmentProfilesFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/envprofiles.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{"Env: Default", "NAME=VALUE", "env-profile-selected", "parseEnvLines", "JSON.stringify({...payload,env})"} {
		if !strings.Contains(js, want) { t.Fatalf("env profiles module missing %q", want) }
	}
}
