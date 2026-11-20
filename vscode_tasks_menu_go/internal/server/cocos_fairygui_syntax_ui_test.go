package server

import (
    "strings"
    "testing"

    webassets "bletonfc/vscode_tasks_menu/web"
)

func TestCocosFairyGuiSyntaxBindings(t *testing.T) {
    read := func(name string) string {
        t.Helper()
        content, err := webassets.Files.ReadFile("featuremods/" + name)
        if err != nil { t.Fatal(err) }
        return string(content)
    }
    fixtures := []struct {
        module string
        requires []string
    }{
        {"editor.js", []string{
            "{id:'typescript',label:'TypeScript',extensions:['ts','mts','cts'],codeMirror:'typescript'",
            "{id:'cocosjson',label:'Cocos JSON',extensions:['fire','scene','prefab','anim','meta'],codeMirror:'json'",
            "{id:'fairygui',label:'FairyGUI Project',extensions:['fairy'],codeMirror:'xml'",
        }},
        {"filecompare.js", []string{
            "if(language.id==='cocosjson')return 'json'",
            "if(language.id==='fairygui')return 'xml'",
        }},
        {"editorcompletion.js", []string{
            "const cocosTypeScriptSnippets=[",
            "function isCocosTypeScriptDocument(doc)",
            "isCocosTypeScriptDocument(context.state.doc)",
            "snippet('ccComponent3'",
            "snippet('ccComponent2'",
            "snippet('fguiCreateObject'",
        }},
    }
    for _, fixture := range fixtures {
        source := read(fixture.module)
        for _, expected := range fixture.requires {
            if !strings.Contains(source, expected) {
                t.Errorf("%s missing %q", fixture.module, expected)
            }
        }
    }
}
