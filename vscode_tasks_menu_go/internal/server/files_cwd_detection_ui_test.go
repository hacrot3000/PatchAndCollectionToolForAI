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

func TestSelectedFileDetectionCarriesNearestShellCommandContext(t *testing.T) {
	for _, want := range []string{
		"function shellPromptContextLine(line)",
		"for(let row=first-1;row>=0&&before.length<256&&budget>0;row--)",
		"if(shellPromptContextLine(clipped))break",
		"before.reverse()",
		"const context=before.concat(selected).join('\\n')",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing command-output context behavior %q", want)
		}
	}
}
