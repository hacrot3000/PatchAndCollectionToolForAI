package main

import (
    "bytes"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/dbprofile"
)

func TestCollectDBRejectsMutationAndUnsafeSQL(t *testing.T) {
    base := collectDBInput{Profile: "HaiTacLocal", MaxRows: 200, TimeoutSeconds: 30}
    for _, sql := range []string{
        "UPDATE t SET a=1", "DELETE FROM t", "SELECT 1; DROP TABLE t",
        "SELECT 1 /* injected */", "SELECT LOAD_FILE('/etc/passwd')",
        "SELECT 1 INTO OUTFILE '/tmp/x'", "SELECT SLEEP(5)", "SELECT GET_LOCK('x', 1)",
        "SELECT 1 --comment", "SELECT 1 FOR UPDATE",
    } {
        req := base
        req.SQL = sql
        if err := validateCollectDBInput(req); err == nil {
            t.Fatalf("unsafe SQL accepted: %q", sql)
        }
    }
    base.SQL = "SELECT id, name FROM users ORDER BY id LIMIT 200"
    if err := validateCollectDBInput(base); err != nil { t.Fatal(err) }
    base.Profile = "profile;rm -rf"
    if validateCollectDBInput(base) == nil { t.Fatal("malformed profile accepted") }
    base.Profile = "HaiTacLocal"
    base.MaxRows = 0
    if validateCollectDBInput(base) == nil { t.Fatal("invalid max_rows accepted") }
}

func TestCollectDBGlobalProfileNameUniqueness(t *testing.T) {
    path := filepath.Join(t.TempDir(), "db_profiles.json")
    store, err := dbprofile.NewStore(path)
    if err != nil { t.Fatal(err) }
    valid := dbprofile.Profile{
        ID: "a1", Name: "HaiTacLocal", AdapterID: "mysql-cli",
        Host: "127.0.0.1", Port: 9000,
    }
    if err := store.Save([]dbprofile.Profile{valid}); err != nil { t.Fatal(err) }
    got, err := findGlobalCollectProfile("HaiTacLocal", store)
    if err != nil || got.Port != 9000 {
        t.Fatalf("stored global profile not resolved: %+v %v", got, err)
    }
    if _, err := findGlobalCollectProfile("not-present", store); err == nil {
        t.Fatal("unknown name should fail")
    }
    other := valid
    other.ID = "a2"
    if err := store.Save([]dbprofile.Profile{valid, other}); err != nil { t.Fatal(err) }
    if _, err := findGlobalCollectProfile("HaiTacLocal", store); err == nil ||
        !strings.Contains(err.Error(), "ambiguous") {
        t.Fatalf("duplicate profile name must fail closed: %v", err)
    }
    got, err = findGlobalCollectProfile("a2", store)
    if err != nil || got.ID != "a2" { t.Fatalf("ID selection failed: %+v %v", got, err) }
    other.Host = "192.0.2.10"
    if err := store.Save([]dbprofile.Profile{other}); err != nil { t.Fatal(err) }
    if _, err := findGlobalCollectProfile("a2", store); err == nil {
        t.Fatal("direct remote DB profile must be rejected")
    }
}

func testCollectWorkspace(t *testing.T, shared bool) string {
    t.Helper()
    root := t.TempDir()
    if err := os.MkdirAll(filepath.Join(root, ".vscode"), 0o700); err != nil { t.Fatal(err) }
    config := "[shared_server]\nenabled = false\n"
    if shared { config = "[shared_server]\nenabled = true\nproject_id = ss_tool\n" }
    if err := os.WriteFile(filepath.Join(root, ".vscode", "vscode_tasks_menu.ini"), []byte(config), 0o600); err != nil { t.Fatal(err) }
    data := []byte(`{"project":{"key":"ss_tool","id":"2a94ed57-5587-480b-adae-7c5c720f84f1"}}`)
    if err := os.WriteFile(filepath.Join(root, ".python_patch_tool.json"), data, 0o600); err != nil { t.Fatal(err) }
    return root
}

func TestCollectDBWorkspaceReadOnlyIdentityAndSharedMode(t *testing.T) {
    root := testCollectWorkspace(t, false)
    before, err := os.ReadFile(filepath.Join(root, ".python_patch_tool.json"))
    if err != nil { t.Fatal(err) }
    if _, err := readCollectWorkspace(root); err != nil { t.Fatal(err) }
    after, _ := os.ReadFile(filepath.Join(root, ".python_patch_tool.json"))
    if !bytes.Equal(before, after) { t.Fatal("COLLECT touched project identity") }
    if _, err := readCollectWorkspace(testCollectWorkspace(t, true)); err == nil ||
        !strings.Contains(err.Error(), "shared-server") {
        t.Fatalf("shared-server mode must not access global credentials: %v", err)
    }
    if err := os.Remove(filepath.Join(root, ".python_patch_tool.json")); err != nil { t.Fatal(err) }
    if _, err := readCollectWorkspace(root); err == nil { t.Fatal("missing project identity accepted") }
}

func TestCollectDBFailsBeforeReadingCredentialsForUnsafeInput(t *testing.T) {
    root := testCollectWorkspace(t, false)
    request := map[string]interface{}{
        "profile": "HaiTacLocal", "sql": "DELETE FROM users", "max_rows": 20, "timeout_sec": 30,
    }
    raw, _ := json.Marshal(request)
    var out bytes.Buffer
    if err := runCollectDBSelect(root, bytes.NewReader(raw), &out); err == nil ||
        !strings.Contains(err.Error(), "SELECT") {
        t.Fatalf("unsafe SQL must fail before loading global profiles: %v", err)
    }
    if out.Len() != 0 { t.Fatal("bridge emitted data after rejecting unsafe SQL") }
}

func TestCollectDBLegacyUTF8StringLiterals(t *testing.T) {
    sql := "SELECT id FROM users WHERE name = CONVERT(0x4142 USING utf8mb4)"
    p := dbprofile.Profile{Options: map[string]string{"charset": "utf8"}}
    got := rewriteCollectLegacyCharset(sql, p)
    if !strings.Contains(got, "CONVERT(0x4142 USING utf8)") || strings.Contains(got, "utf8mb4") {
        t.Fatalf("legacy SQL still requires unsupported charset: %s", got)
    }
    p.Options["charset"] = "latin1"
    if x := rewriteCollectLegacyCharset(sql, p); !strings.Contains(x, "USING utf8)") {
        t.Fatalf("UTF-8 hex values must not be interpreted as latin1: %s", x)
    }
    p.Options["charset"] = "utf8mb4"
    if x := rewriteCollectLegacyCharset(sql, p); x != sql {
        t.Fatalf("modern MySQL SQL unexpectedly changed: %s", x)
    }
}
