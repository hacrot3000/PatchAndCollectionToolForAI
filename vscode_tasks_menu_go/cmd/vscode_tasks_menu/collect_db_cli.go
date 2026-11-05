package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"

    "bletonfc/vscode_tasks_menu/internal/config"
    "bletonfc/vscode_tasks_menu/internal/dbadapter"
    "bletonfc/vscode_tasks_menu/internal/dbmysql"
    "bletonfc/vscode_tasks_menu/internal/dbprofile"
    "bletonfc/vscode_tasks_menu/internal/dbsession"
    "bletonfc/vscode_tasks_menu/internal/secretstore"
    "bletonfc/vscode_tasks_menu/internal/sshprofile"
    "bletonfc/vscode_tasks_menu/internal/sshtunnel"
)

// collectDBInput is internal to the Python SELECT-only AST compiler and the
// same-OS-user TaskDeck binary. Never accept credentials or filesystem paths
// from the COLLECT request ZIP. There is no network endpoint for this helper.
type collectDBInput struct {
    Profile string `json:"profile"`
    SQL string `json:"sql"`
    MaxRows int `json:"max_rows"`
    TimeoutSeconds int `json:"timeout_sec"`
}
type collectDBOutput struct {
    Engine string `json:"engine"`
    Transport string `json:"transport"`
    Columns []string `json:"columns"`
    Rows [][]interface{} `json:"rows"`
    Truncated bool `json:"truncated"`
}

var collectProfileName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,96}$`)
var collectSelectPattern = regexp.MustCompile(`(?is)^SELECT\s`)

func validateCollectDBInput(req collectDBInput) error {
    if !collectProfileName.MatchString(req.Profile) {
        return errors.New("COLLECT database profile name is invalid")
    }
    if req.MaxRows < 1 || req.MaxRows > 1000000 {
        return errors.New("COLLECT max_rows must be 1..1000000")
    }
    if req.TimeoutSeconds < 1 || req.TimeoutSeconds > 600 {
        return errors.New("COLLECT timeout_sec must be 1..600")
    }
    sql := strings.TrimSpace(req.SQL)
    if len(sql) == 0 || len(sql) > 128*1024 || !collectSelectPattern.MatchString(sql) {
        return errors.New("COLLECT accepts a bounded, generated SELECT statement only")
    }
    // The Python layer only supplies compiler-generated SELECT; the second
    // gate forbids common raw SQL escape hatches and multi-statement execution.
    upper := strings.ToUpper(sql)
    for _, forbidden := range []string{";", "--", "/*", "*/", "#", " INTO ", " OUTFILE", " DUMPFILE",
        " LOAD_FILE(", " SLEEP(", " BENCHMARK(", " GET_LOCK(", " FOR UPDATE", " LOCK IN SHARE MODE"} {
        if strings.Contains(upper, forbidden) {
            return errors.New("COLLECT SELECT contains unsupported SQL syntax")
        }
    }
    return nil
}

func readCollectWorkspace(root string) (string, error) {
    if root == "" || !filepath.IsAbs(root) {
        return "", errors.New("COLLECT workspace must be an absolute path")
    }
    root = filepath.Clean(root)
    info, err := os.Lstat(root)
    if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
        return "", errors.New("COLLECT workspace must be an existing non-symlink directory")
    }
    // The global profile feature is only for the same-OS-user, single-user
    // TaskDeck. Shared-server users must use the owner's permissioned API;
    // a local CLI bridge would otherwise bypass per-project/SSH grants.
    configFile := filepath.Join(root, ".vscode", "vscode_tasks_menu.ini")
    statInfo, err := os.Lstat(configFile)
    if err != nil || !statInfo.Mode().IsRegular() || statInfo.Mode()&os.ModeSymlink != 0 {
        return "", errors.New("COLLECT global profile access requires an existing workspace TaskDeck configuration")
    }
    cfg, _, err := config.Load(root)
    if err != nil {
        return "", fmt.Errorf("read workspace TaskDeck config: %w", err)
    }
    if cfg.SharedServerEnabled {
        return "", errors.New("COLLECT global profile CLI is disabled in shared-server mode (per-user permission boundary)")
    }
    identityFile := filepath.Join(root, ".python_patch_tool.json")
    statInfo, err = os.Lstat(identityFile)
    if err != nil || !statInfo.Mode().IsRegular() || statInfo.Mode()&os.ModeSymlink != 0 || statInfo.Size() > 1024*1024 {
        return "", errors.New("COLLECT workspace project identity is not available")
    }
    data, err := os.ReadFile(identityFile)
    if err != nil {
        return "", errors.New("COLLECT workspace project identity cannot be read")
    }
    var record struct { Project struct { Key string `json:"key"`; ID string `json:"id"` } `json:"project"` }
    if json.Unmarshal(data, &record) != nil || record.Project.Key == "" || record.Project.ID == "" {
        return "", errors.New("COLLECT workspace requires both project.key and project.id")
    }
    return root, nil
}

func findGlobalCollectProfile(name string, store *dbprofile.Store) (dbprofile.Profile, error) {
    profiles, err := store.Load()
    if err != nil {
        return dbprofile.Profile{}, fmt.Errorf("read TaskDeck global database profiles: %w", err)
    }
    var found dbprofile.Profile
    matches := 0
    for _, p := range profiles {
        if p.Name == name || p.ID == name {
            matches++
            found = p
        }
    }
    if matches == 0 {
        return dbprofile.Profile{}, fmt.Errorf("TaskDeck global database connection %q was not found; check the exact connection name", name)
    }
    if matches > 1 {
        return dbprofile.Profile{}, fmt.Errorf("TaskDeck global database connection %q is ambiguous; use a unique profile ID", name)
    }
    if found.AdapterID != dbmysql.AdapterID {
        return dbprofile.Profile{}, fmt.Errorf("TaskDeck global connection %q is not a MySQL/MariaDB connection", name)
    }
    if found.Transport == dbprofile.TransportDirect && found.Host != "127.0.0.1" &&
        found.Host != "localhost" && found.Host != "::1" {
        return dbprofile.Profile{}, errors.New("COLLECT direct MySQL connections must use loopback; use a saved SSH tunnel for remote connections")
    }
    return found, nil
}

// rewriteCollectLegacyCharset preserves the safety property of the AST
// compiler's UTF-8 hex string literals even when the remote MySQL server
// predates utf8mb4. Bytes were encoded in UTF-8; never decode them as latin1.
func rewriteCollectLegacyCharset(sql string, profile dbprofile.Profile) string {
    charset := strings.TrimSpace(profile.Options["charset"])
    if charset == "" || strings.EqualFold(charset, "utf8mb4") {
        return sql
    }
    return strings.ReplaceAll(sql, " USING utf8mb4)", " USING utf8)")
}

// runCollectDBSelect uses the normal TaskDeck global profile, encrypted secret
// store, SSH tunnel and MySQL adapter. It forces ReadOnly irrespective of the
// browser's read/write profile setting. stdout carries only bounded query data.
func runCollectDBSelect(workspace string, input io.Reader, output io.Writer) error {
    root, err := readCollectWorkspace(workspace)
    if err != nil { return err }
    dec := json.NewDecoder(io.LimitReader(input, 192*1024))
    dec.DisallowUnknownFields()
    var req collectDBInput
    if err := dec.Decode(&req); err != nil { return errors.New("COLLECT helper request is not valid JSON") }
    var extra any
    if err := dec.Decode(&extra); err != io.EOF { return errors.New("COLLECT helper accepts exactly one JSON request") }
    if err := validateCollectDBInput(req); err != nil { return err }
    store, err := dbprofile.DefaultStore()
    if err != nil { return err }
    p, err := findGlobalCollectProfile(req.Profile, store)
    if err != nil { return err }
    secrets, err := secretstore.NewDefaultFileStore()
    if err != nil { return err }
    timeout := time.Duration(req.TimeoutSeconds) * time.Second
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    host, port := p.Host, p.Port
    var tunnelMgr *sshtunnel.Manager
    transport := string(p.Transport)
    if p.Transport == dbprofile.TransportSSHTunnel {
        sshStore, err := sshprofile.DefaultStore()
        if err != nil { return err }
        ssh, err := sshStore.Get(p.SSHProfileID)
        if err != nil { return fmt.Errorf("saved SSH connection for COLLECT is unavailable: %w", err) }
        exe, err := os.Executable()
        if err != nil { return err }
        dir, err := os.MkdirTemp("", "taskdeck-collect-ssh-")
        if err != nil { return err }
        defer os.RemoveAll(dir)
        tunnelMgr, err = sshtunnel.NewManager(secrets, sshtunnel.Options{RuntimeDir: dir, AskpassHelper: exe})
        if err != nil { return err }
        defer tunnelMgr.Close()
        t, err := tunnelMgr.Open(ctx, ssh, p.Host, p.Port)
        if err != nil { return fmt.Errorf("open saved TaskDeck SSH tunnel: %w", err) }
        host, port = t.LocalHost, t.LocalPort
    } else if p.Transport != "" && p.Transport != dbprofile.TransportDirect {
        return errors.New("unsupported TaskDeck database connection transport")
    } else {
        transport = "direct"
    }

    payload := dbadapter.ConnectPayload{
        Host: host, Port: port, Username: p.Username, Database: p.Database,
        ReadOnly: true, Options: p.Options,
    }
    if p.SecretRef != "" {
        secret, err := secrets.Get(p.SecretRef)
        if err != nil { return errors.New("TaskDeck database authentication secret is unavailable") }
        payload.Secret = string(secret)
        for i := range secret { secret[i] = 0 }
    }
    defer func() { payload.Secret = "" }()

    exe, err := os.Executable()
    if err != nil { return err }
    manifest, err := dbmysql.BuiltinManifest(exe)
    if err != nil { return err }
    registry := dbadapter.NewRegistry()
    if err := registry.Register(manifest); err != nil { return err }
    manager, err := dbsession.NewManager(registry, 1)
    if err != nil { return err }
    defer manager.Close()
    session, err := manager.Open(ctx, p.ID, p.AdapterID, dbadapter.ProcessOptions{Dir: root}, payload)
    payload.Secret = ""
    if err != nil { return fmt.Errorf("connect saved TaskDeck MySQL profile in read-only mode: %w", err) }
    defer manager.CloseSession(session.ID)

    limit := req.MaxRows+1
    if limit > dbadapter.MaxRows { limit = dbadapter.MaxRows }
    if limit < 1 { limit = dbadapter.MaxRows }
    reply, err := manager.Request(ctx, session.ID, dbadapter.OpExecute, dbadapter.ExecutePayload{
        Statement: rewriteCollectLegacyCharset(req.SQL, p), MaxRows: limit,
    })
    if err != nil { return fmt.Errorf("execute read-only COLLECT SELECT: %w", err) }
    if reply.Error != nil { return fmt.Errorf("COLLECT SELECT failed: %s", reply.Error.Code) }
    var result dbadapter.ExecuteResult
    if err := json.Unmarshal(reply.Payload, &result); err != nil {
        return errors.New("COLLECT MySQL result cannot be decoded")
    }
    if err := dbadapter.ValidateExecuteResult(result); err != nil { return err }
    names := make([]string, len(result.Columns))
    for i, column := range result.Columns { names[i] = column.Name }
    return json.NewEncoder(output).Encode(collectDBOutput{
        Engine: "mysql", Transport: transport, Columns: names, Rows: result.Rows,
        Truncated: result.Truncated || len(result.Rows) >= dbadapter.MaxRows,
    })
}
