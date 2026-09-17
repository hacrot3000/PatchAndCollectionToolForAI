package tasks

import (
	"strings"
	"testing"
)

func TestApplyEnvironmentOverrides(t *testing.T) {
	spec := Execution{Env: []string{"A=old", "KEEP=yes"}}
	if err := ApplyEnvironmentOverrides(&spec, map[string]string{"A":"new", "B":"two"}); err != nil { t.Fatal(err) }
	joined := strings.Join(spec.Env, "\n")
	for _, want := range []string{"A=new", "B=two", "KEEP=yes"} { if !strings.Contains(joined, want) { t.Fatalf("env missing %q in %q", want, joined) } }
}

func TestApplyEnvironmentOverridesRejectsInvalidName(t *testing.T) {
	spec := Execution{}
	if err := ApplyEnvironmentOverrides(&spec, map[string]string{"BAD-NAME":"x"}); err == nil { t.Fatal("expected invalid environment name error") }
}
