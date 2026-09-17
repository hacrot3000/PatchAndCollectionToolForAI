package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestViewportFixPreventsDocumentScroll(t *testing.T) {
	js, err := webassets.Files.ReadFile("featuremods/viewportfix.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(js)
	for _, want := range []string{
		"html,body{width:100%;height:100%;min-height:0;overflow:hidden",
		"body{height:100vh;height:100dvh;display:flex;flex-direction:column}",
		"body>main{flex:1 1 auto;height:auto!important;min-height:0",
		"grid-template-rows:minmax(0,1fr)",
		"body>main>section{min-width:0;min-height:0",
		"#panes{flex:1 1 auto;min-width:0;min-height:0",
		"#panes>.pane>.terminal{min-width:0;min-height:0;overflow:hidden}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("viewportfix.js missing viewport constraint %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/viewportfix.js';") {
		t.Fatal("next.js does not load viewportfix.js")
	}
}
