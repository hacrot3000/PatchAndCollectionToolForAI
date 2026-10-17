package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func iosSyntaxSource(t *testing.T, name string) string {
    t.Helper()
    data, err := webassets.Files.ReadFile("featuremods/" + name)
    if err != nil { t.Fatal(err) }
    return string(data)
}

func TestSwiftSyntaxLoadedBeforeCompareAndEditor(t *testing.T) {
    source := iosSyntaxSource(t, "next.js")
    swift := strings.Index(source, "import '/featuremods/swiftsyntax.js';")
    compare := strings.Index(source, "import '/featuremods/filecompare.js';")
    editor := strings.Index(source, "import '/featuremods/editor.js';")
    if swift < 0 || compare <= swift || editor <= compare {
        t.Fatalf("Swift lexer must load before File Compare and editor: swift=%d compare=%d editor=%d", swift, compare, editor)
    }
}

func TestSwiftSyntaxWiredThroughEditorCompareAndCompletion(t *testing.T) {
    editor := iosSyntaxSource(t, "editor.js")
    compare := iosSyntaxSource(t, "filecompare.js")
    completion := iosSyntaxSource(t, "editorcompletion.js")
    swift := iosSyntaxSource(t, "swiftsyntax.js")
    cases := []struct {
        code string
        name string
        wants []string
    }{
        {editor, "editor", []string{
            "{id:'swift',label:'Swift',extensions:['swift'],swiftSyntax:true,completion:'swift'}",
            "if(language?.swiftSyntax)options.extraExtensions.push(swiftSyntaxExtension())",
            "{id:'objectivec',label:'Objective-C'",
            "{id:'objectivecpp',label:'Objective-C++'",
            "storyboard","stringsdict","xcconfig",
        }},
        {compare, "filecompare", []string{
            "TaskMenuSwiftSyntax.tokenizeText(text)",
            "compareLanguageID(pathValue)==='swift'",
            "['swift','cpp'",
        }},
        {completion, "completion", []string{
            "swift:profile(", "snippet('View','SwiftUI View'",
            "TaskMenuSwiftSyntax.lexicalModeAt(context.state.doc,context.pos)",
            "language.id==='swift'",
        }},
        {swift, "lexer", []string{
            "const swiftDocumentCache=new WeakMap()",
            "state.blockDepth++",
            "state.blockDepth--",
            "state.quote",
            "function tokenizeText(text)",
            "function lexicalModeAt(doc,pos)",
            "globalThis.TaskMenuSwiftSyntax=",
        }},
    }
    for _, tc := range cases {
        for _, want := range tc.wants {
            if !strings.Contains(tc.code,want) {
                t.Errorf("%s missing %q",tc.name,want)
            }
        }
    }
}
