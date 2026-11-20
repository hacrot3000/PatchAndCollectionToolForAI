package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func TestStructuredFormatterLoadedBeforeEditor(t *testing.T) {
    data, err := webassets.Files.ReadFile("featuremods/next.js")
    if err != nil { t.Fatal(err) }
    js := string(data)
    format := strings.Index(js, "import '/featuremods/structuredview.js';")
    editor := strings.Index(js, "import '/featuremods/editor.js';")
    compare := strings.Index(js, "import '/featuremods/filecompare.js';")
    if format < 0 || editor < 0 || compare < 0 || format >= editor || format >= compare {
        t.Fatalf("Structured View must load before File Compare and editor: format=%d compare=%d editor=%d",format,compare,editor)
    }
}

func TestStructuredEditorSyntaxAndReadOnlyView(t *testing.T) {
    read := func(file string) string {
        t.Helper()
        contents, err := webassets.Files.ReadFile("featuremods/"+file)
        if err != nil { t.Fatal(err) }
        return string(contents)
    }
    cases := []struct {
        name string
        wants []string
    }{
        {"editor.js", []string{
            "{id:'gradle',label:'Gradle / Groovy',extensions:['gradle','groovy','gvy','gy']",
            "{id:'csv',label:'CSV',extensions:['csv']",
            "{id:'tsv',label:'TSV',extensions:['tsv']",
            "{id:'proguard',label:'ProGuard / R8',extensions:['pro']",
            "const structuredSelect=document.createElement('select')",
            "globalThis.TaskMenuStructuredView?.setMode(view,structuredSelect.value)",
            "globalThis.TaskMenuStructuredView?.markStale(view)",
            "structuredMode:'raw'",
            "function syncEditorStructuredView(view)",
            "syncEditorStructuredView(view);",
        }},
        {"editorcompletion.js", []string{
            "groovy:profile(", "proguard:profile(",
            "snippet('plugins','Gradle plugin block'",
            "snippet('android','Android Gradle configuration'",
        }},
        {"structuredview.js", []string{
            "function formatJson(source)",
            "function formatXml(source)",
            "function parseDelimited(source,delimiter)",
            "function renderTree(root,parsed)",
            "function renderTable(host,rows)",
            "view.cm.state.doc.toString()",
            "view.body.classList.add('structured-hidden')",
            "view.structuredMode='raw'",
            "globalThis.TaskMenuStructuredView=",
        }},
    }
    for _, tc := range cases {
        js:=read(tc.name)
        for _,want := range tc.wants {
            if !strings.Contains(js,want) { t.Errorf("%s missing %q",tc.name,want) }
        }
    }
    formatter:=read("structuredview.js")
    for _,forbidden:=range []string{
        "view.cm.dispatch(", "setDirty(view", "saveEditor(", "innerHTML", "eval(",
        "fetch(", "https://", "http://",
    }{
        if strings.Contains(formatter,forbidden) { t.Errorf("Structured View must remain read-only/offline: %q",forbidden) }
    }
}
