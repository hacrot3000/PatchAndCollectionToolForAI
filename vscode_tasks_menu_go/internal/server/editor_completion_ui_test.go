package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestEditorCompletionModuleIsLoadedAfterEditor(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	editor := strings.Index(js, "import '/featuremods/editor.js';")
	completion := strings.Index(js, "import '/featuremods/editorcompletion.js';")
	lsp := strings.Index(js, "import '/featuremods/lsp.js';")
	if editor < 0 || completion < 0 || lsp < 0 || !(editor < completion && completion < lsp) {
		t.Fatalf("completion module load order invalid: editor=%d completion=%d lsp=%d", editor, completion, lsp)
	}
}

func TestEditorCompletionUsesVendoredCodeMirrorHooks(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"globalThis.cm6?.autocompletion",
		"globalThis.cm6?.snippetCompletion",
		"globalThis.cm6.autocompletion({",
		"globalThis.cm6.snippetCompletion",
		"globalThis.cm6.startCompletion(view.cm)",
		"activateOnTyping:true",
		"activateOnTypingDelay:120",
		"maxRenderedOptions:80",
		"Ctrl+Space",
	} {
		if want == "Ctrl+Space" {
			continue // supplied by vendored CodeMirror completionKeymap; asserted in editor_ui_test.go
		}
		if !strings.Contains(js, want) {
			t.Fatalf("editor completion hook missing %q", want)
		}
	}
}

func TestEditorCompletionTierOneProfilesCoverRegisteredLanguages(t *testing.T) {
	editorData, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	completionData, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	editorJS := string(editorData)
	completionJS := string(completionData)

	for _, profile := range []string{
		"cpp", "go", "python", "javascript", "typescript", "json", "yaml", "markdown",
		"html", "css", "dockerfile", "makefile", "toml", "powershell", "kotlin", "csharp",
		"dart", "protobuf", "graphql", "actionscript", "nginx", "apache", "config", "xml",
		"java", "php", "sql", "rust", "cmake", "shell", "nim", "lua",
	} {
		if !strings.Contains(editorJS, "completion:'"+profile+"'") {
			t.Fatalf("language registry missing completion profile %q", profile)
		}
		if !strings.Contains(completionJS, "\n  "+profile+":profile(") {
			t.Fatalf("completion profile %q missing implementation", profile)
		}
	}
}

func TestEditorCompletionProvidesSnippetsAndSuppressesCommentsAndStrings(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function currentLexicalMode(context,syntax)",
		"blockComments",
		"lineComments",
		"return lineComment||blockClose?'comment':quote?'string':'code'",
		"if(currentLexicalMode(context,p.syntax)!=='code')return null",
		"function completionToken(context)",
		"validFor:/[A-Za-z_#$@][A-Za-z0-9_$@#.-]*/",
		"snippet('func','function'",
		"snippet('proc','procedure'",
		"snippet('function','function'",
		"snippet('class','class'",
		"snippet('message','message'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Tier 1 completion behavior missing %q", want)
		}
	}
}

func TestEditorCompletionDoesNotRequireRuntimePackageDownloads(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	js := strings.ToLower(string(data))
	for _, forbidden := range []string{"npm install", "npm ci", "npx ", "https://", "http://", "unpkg", "jsdelivr"} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("editor completion must remain vendored/offline; found %q", forbidden)
		}
	}
}

func TestEditorCompletionTierTwoUsesProjectCompletionAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function tier2Source(file)",
		"'/api/project/completions'",
		"text:context.state.doc.toString()",
		"line_prefix:position.linePrefix",
		"lexical_mode:mode",
		"position:{line:position.line,character:position.character}",
		"source==='project-path'",
		"replace_prefix",
		"override:[tier1Source(file),tier2Source(file)]",
		"new AbortController()",
		"{onDocChange:true}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Tier 2 completion behavior missing %q", want)
		}
	}
}

func TestEditorCompletionRecognizesImportContexts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editorcompletion.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"id==='cpp'",
		"id==='javascript'||id==='typescript'",
		"id==='python'",
		"id==='lua'",
		"id==='nim'",
		"id==='actionscript'",
		"id==='go'",
		"id==='php'",
		"id==='dart'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("import completion context missing %q", want)
		}
	}
}
