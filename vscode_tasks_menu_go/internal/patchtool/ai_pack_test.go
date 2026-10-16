package patchtool

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAIPackFixture(t *testing.T, root, prompt string) string {
	t.Helper()
	entry := filepath.Join(root, "patchtool", "python_patch_entry.py")
	docsRoot := filepath.Join(root, "patchtool", "_patch_lib", "docs")
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("# test\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range aiPackRequiredDocs {
		if err := os.WriteFile(filepath.Join(docsRoot, name), []byte("fixture "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(docsRoot, "CAPABILITY_LEDGER.md"), []byte("internal-only-v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompts := map[string]any{
		"version": 1,
		"vi": prompt,
		"en": "English prompt",
		"ru": "Russian prompt",
	}
	raw, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(docsRoot, aiPackPromptDocName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestBuildAIPackContainsOnlyEssentialContractDocs(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)

	first, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached || first.DocCount != len(aiPackRequiredDocs) || !strings.Contains(first.Prompt, "Prompt version 1") || first.ProjectIdentity.ID == "" || !strings.Contains(first.Prompt, first.ProjectIdentity.ID) || first.Fingerprint == "" {
		t.Fatalf("unexpected first AI Pack result: %+v", first)
	}
	if first.Path != "artifacts/patch_tool/ai_pack/"+aiPackZipName {
		t.Fatalf("unexpected AI Pack path: %q", first.Path)
	}

	packPath := filepath.Join(workspace, filepath.FromSlash(first.Path))
	zr, err := zip.OpenReader(packPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != len(aiPackRequiredDocs)+2 {
		t.Fatalf("AI Pack file count=%d want=%d", len(zr.File), len(aiPackRequiredDocs)+2)
	}
	names := map[string]bool{}
	for _, file := range zr.File {
		names[file.Name] = true
	}
	for _, want := range aiPackRequiredDocs {
		if !names[want] {
			t.Fatalf("AI Pack missing essential document %q", want)
		}
	}
	for _, name := range []string{"PROJECT_IDENTITY.json", "PROJECT_IDENTITY_INSTRUCTIONS.txt"} {
		if !names[name] { t.Fatalf("AI Pack missing required identity evidence %q",name) }
	}
	idFile,err:=zr.File[len(zr.File)-2].Open()
	if err!=nil { t.Fatal(err) }
	_ = idFile.Close()

	for _, forbidden := range []string{
		aiPackPromptDocName,
		"PROMPT_VI.txt", "PROMPT_EN.txt", "PROMPT_RU.txt",
		"START_HERE.md", "MANIFEST.json", "CAPABILITY_LEDGER.md",
	} {
		if names[forbidden] {
			t.Fatalf("AI Pack must not include non-essential file %q", forbidden)
		}
	}

	second, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached || second.Fingerprint != first.Fingerprint || second.GeneratedAt != first.GeneratedAt {
		t.Fatalf("matching essential docs should reuse cache: first=%+v second=%+v", first, second)
	}
}

func TestBuildAIPackRebuildsOnlyWhenPackagedDocsChange(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	docsRoot := filepath.Join(root, "patchtool", "_patch_lib", "docs")

	first, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsRoot, "AI_USAGE_CONTRACT.md"), []byte("contract-v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Cached || second.Fingerprint == first.Fingerprint {
		t.Fatalf("changed packaged docs must rebuild AI Pack: first=%+v second=%+v", first, second)
	}

	if err := os.WriteFile(filepath.Join(docsRoot, "CAPABILITY_LEDGER.md"), []byte("internal-only-v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if !third.Cached || third.Fingerprint != second.Fingerprint {
		t.Fatalf("changed excluded docs must not rebuild AI Pack: second=%+v third=%+v", second, third)
	}

	promptPath := filepath.Join(docsRoot, aiPackPromptDocName)
	raw, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(raw), "Prompt version 1", "Prompt version 2", 1)
	if err := os.WriteFile(promptPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	fourth, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	if !fourth.Cached || fourth.Fingerprint != third.Fingerprint || fourth.GeneratedAt != third.GeneratedAt || !strings.Contains(fourth.Prompt, "Prompt version 2") || fourth.ProjectIdentity.ID != third.ProjectIdentity.ID {
		t.Fatalf("changed prompt must refresh Copy Prompt without rebuilding ZIP: third=%+v fourth=%+v", third, fourth)
	}
}

func TestBuildAIPackRejectsMissingPromptOrEssentialDoc(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	docsRoot := filepath.Join(root, "patchtool", "_patch_lib", "docs")

	if err := os.Remove(filepath.Join(docsRoot, aiPackPromptDocName)); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck")); err == nil || !strings.Contains(err.Error(), "missing from the active Patch Tool docs") {
		t.Fatalf("missing standard prompt doc should fail closed, err=%v", err)
	}

	entry = writeAIPackFixture(t, root, "Prompt version 1")
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	if err := os.Remove(filepath.Join(docsRoot, aiPackRequiredDocs[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAIPack(workspace, filepath.Join(root, "taskdeck")); err == nil || !strings.Contains(err.Error(), "required AI Pack document") {
		t.Fatalf("missing essential AI Pack doc should fail closed, err=%v", err)
	}
}


func TestAIPackRulesAreProjectSpecificAndInvalidateOnlyOwnCache(t *testing.T) {
    root:=t.TempDir()
    workspace:=filepath.Join(root,"my-project")
    if err:=os.MkdirAll(workspace,0o755);err!=nil{t.Fatal(err)}
    entry:=writeAIPackFixture(t,root,"Standard instructions")
    t.Setenv("TASKDECK_PATCH_RUNTIME",entry)
    initial,err:=BuildAIPack(workspace,filepath.Join(root,"taskdeck"))
    if err!=nil{t.Fatal(err)}
    if initial.ProjectIdentity.Key!="my-project" || initial.ProjectIdentity.ID=="" {
        t.Fatalf("first build must return identity, not only cache path: %+v",initial)
    }
    if initial.ProjectRules!="" {t.Fatalf("unexpected default project rules: %q",initial.ProjectRules)}
    rules:="Requires PHP 5.5. No PHP 7+ syntax; keep existing coding style."
    updated,err:=SaveAIPackProjectRules(workspace,rules)
    if err!=nil{t.Fatal(err)}
    if updated.ProjectIdentity!=initial.ProjectIdentity || updated.Rules!=rules {t.Fatalf("save response=%+v",updated)}
    disk,err:=os.ReadFile(filepath.Join(workspace,aiPackRulesName))
    if err!=nil{t.Fatal(err)}
    if !strings.Contains(string(disk),rules){t.Fatalf("project rules not persisted: %s",disk)}
    next,err:=BuildAIPack(workspace,filepath.Join(root,"taskdeck"))
    if err!=nil{t.Fatal(err)}
    if next.Cached || next.Fingerprint==initial.Fingerprint || next.ProjectRules!=rules ||
        !strings.Contains(next.Prompt,rules) || next.ProjectIdentity!=initial.ProjectIdentity {
        t.Fatalf("AI Pack missing rules / UUID / cache invalidation: %+v",next)
    }
    zr,err:=zip.OpenReader(filepath.Join(workspace,filepath.FromSlash(next.Path)))
    if err!=nil{t.Fatal(err)}
    names:=map[string]bool{}
    for _,file:=range zr.File{names[file.Name]=true}
    _=zr.Close()
    if !names["PROJECT_RULES.md"] || !names["PROJECT_IDENTITY.json"] {t.Fatalf("missing identity or rules in ZIP: %+v",names)}
    repeat,err:=BuildAIPack(workspace,filepath.Join(root,"taskdeck"))
    if err!=nil||!repeat.Cached||repeat.Fingerprint!=next.Fingerprint {t.Fatalf("unchanged rules should reuse cache: %+v %v",repeat,err)}
    other:=filepath.Join(root,"different")
    if err:=os.MkdirAll(other,0o755);err!=nil{t.Fatal(err)}
    otherPack,err:=BuildAIPack(other,filepath.Join(root,"taskdeck"))
    if err!=nil{t.Fatal(err)}
    if otherPack.ProjectRules!="" || otherPack.ProjectIdentity.ID==next.ProjectIdentity.ID || strings.Contains(otherPack.Prompt,rules){
        t.Fatalf("rules leaked between projects: %+v",otherPack)
    }
}

func TestAIPackRulesRejectMalformedAndForeignIdentity(t *testing.T){
    workspace:=t.TempDir()
    identity,err:=EnsureProjectIdentity(workspace)
    if err!=nil{t.Fatal(err)}
    configPath:=filepath.Join(workspace,aiPackRulesName)
    if err:=os.WriteFile(configPath,[]byte(`{"version":1,"project_identity":{"key":"foreign","id":"not-this-project"},"rules":"malicious"}`),0o600);err!=nil{t.Fatal(err)}
    if _,err:=ReadAIPackProjectRules(workspace);err==nil {t.Fatal("foreign project rules were accepted")}
    if _,err:=SaveAIPackProjectRules(workspace,"new rules");err==nil {t.Fatal("foreign project rules were overwritten")}
    raw,err:=os.ReadFile(configPath);if err!=nil{t.Fatal(err)}
    if !strings.Contains(string(raw),"foreign"){t.Fatal("foreign identity file was changed")}
    if err:=os.Remove(configPath);err!=nil{t.Fatal(err)}
    if _,err:=SaveAIPackProjectRules(workspace,"PHP 5.5 only");err!=nil{t.Fatal(err)}
    loaded,err:=ReadAIPackProjectRules(workspace)
    if err!=nil||loaded.ProjectIdentity!=identity||loaded.Rules!="PHP 5.5 only" {t.Fatalf("wrong saved project rules: %+v %v",loaded,err)}
    if _,err:=SaveAIPackProjectRules(workspace,strings.Repeat("x",maxProjectRulesBytes+1));err==nil{t.Fatal("oversized rules accepted")}
}
