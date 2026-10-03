package server

import (
	"strings"
	"testing"
)

func TestSelectedFileDetectionSendsLiveWorkingDirectory(t *testing.T) {
	for _, want := range []string{
		"'/api/sessions/'+encodeURIComponent(view.meta.id)+'/cwd'",
		"state?.local===true",
		"target_type||''",
		"JSON.stringify({text:text.slice(0,65536),context:context.slice(0,65536),cwd})",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing live CWD selected-file detection %q", want)
		}
	}
}
