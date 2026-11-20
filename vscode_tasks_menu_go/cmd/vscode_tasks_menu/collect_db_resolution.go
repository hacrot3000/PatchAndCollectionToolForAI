package main

import (
    "context"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "sort"
    "strings"

    "bletonfc/vscode_tasks_menu/internal/dbadapter"
    "bletonfc/vscode_tasks_menu/internal/dbsession"
)

// Only table names extracted from Patch Tool's validated SELECT AST can enter
// this resolver; do not parse arbitrary SQL or guess a database from filenames.
var collectTableIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,63}$`)

const collectMaxUnqualifiedTables = 128

func validateCollectSourceTables(tables []string) error {
    if len(tables) > collectMaxUnqualifiedTables {
        return fmt.Errorf("COLLECT includes more than %d unqualified table names; configure a database in the saved connection or qualify table names", collectMaxUnqualifiedTables)
    }
    for _, name := range tables {
        if !collectTableIdentifier.MatchString(name) {
            return fmt.Errorf("COLLECT contains invalid unqualified table identifier")
        }
    }
    return nil
}

// chooseCollectSchema never chooses based on order, a filename, the connection
// display name or an arbitrary schema. All unqualified sources must occur in
// exactly one common visible schema; ambiguous or missing matches fail closed.
func chooseCollectSchema(tables []string, rows [][]interface{}) (string, error) {
    if err := validateCollectSourceTables(tables); err != nil { return "", err }
    if len(tables) == 0 { return "", errors.New("COLLECT has no unqualified table names to resolve") }
    if len(rows) >= dbadapter.MaxRows {
        return "", errors.New("COLLECT database discovery reached the metadata row limit; configure the database explicitly")
    }
    byTable := make(map[string]map[string]bool, len(tables))
    for _, name := range tables { byTable[name] = make(map[string]bool) }
    for _, row := range rows {
        if len(row) != 2 {
            return "", errors.New("COLLECT database discovery returned malformed metadata")
        }
        schema, okSchema := row[0].(string)
        table, okTable := row[1].(string)
        if !okSchema || !okTable || schema == "" {
            return "", errors.New("COLLECT database discovery returned invalid names")
        }
        if matches, known := byTable[table]; known {
            // Catalog entries from MySQL are authoritative only when the
            // returned table identifier exactly matches the validated source.
            matches[schema] = true
        }
    }
    var common map[string]bool
    for _, name := range tables {
        matches := byTable[name]
        if len(matches) == 0 {
            return "", fmt.Errorf("COLLECT table %q is not visible in information_schema; configure a database for the TaskDeck connection or use a qualified table name", name)
        }
        if common == nil {
            common = make(map[string]bool, len(matches))
            for schema := range matches { common[schema] = true }
            continue
        }
        for schema := range common {
            if !matches[schema] { delete(common, schema) }
        }
    }
    if len(common) == 0 {
        return "", errors.New("COLLECT unqualified tables are not all present in one common visible database; configure the saved connection database or qualify every table in the COLLECT AST")
    }
    if len(common) > 1 {
        candidates := make([]string, 0, len(common))
        for name := range common { candidates = append(candidates, name) }
        sort.Strings(candidates)
        if len(candidates) > 12 { candidates = candidates[:12] }
        return "", fmt.Errorf("COLLECT database is ambiguous: table names exist in %d common schemas (%s); set the saved connection database or fully qualify the source tables", len(common), strings.Join(candidates, ", "))
    }
    for schema := range common { return schema, nil }
    return "", errors.New("COLLECT database resolution failed")
}

func discoverCollectSchema(ctx context.Context, manager *dbsession.Manager, sessionID string, tables []string) (string, error) {
    if err := validateCollectSourceTables(tables); err != nil { return "", err }
    if len(tables) == 0 { return "", nil }
    unique := make(map[string]bool, len(tables))
    for _, table := range tables { unique[table] = true }
    names := make([]string, 0, len(unique))
    for table := range unique { names = append(names, table) }
    sort.Strings(names)

    // Names are UTF-8 hex literals, never interpolated as SQL syntax.
    // information_schema.TABLES works without a selected default database,
    // including on MySQL 5.5, and returns only tables visible to the account.
    hexValues := make([]string, len(names))
    for i, name := range names {
        hexValues[i] = "0x" + hex.EncodeToString([]byte(name))
    }
    sql := "SELECT TABLE_SCHEMA, TABLE_NAME FROM information_schema.TABLES " +
        "WHERE TABLE_NAME IN (" + strings.Join(hexValues, ", ") + ") " +
        "ORDER BY TABLE_SCHEMA, TABLE_NAME LIMIT 1000"
    reply, err := manager.Request(ctx, sessionID, dbadapter.OpExecute, dbadapter.ExecutePayload{
        Statement: sql,
        MaxRows: dbadapter.MaxRows,
    })
    if err != nil { return "", fmt.Errorf("COLLECT database metadata lookup failed: %w", err) }
    if reply.Error != nil {
        return "", fmt.Errorf("COLLECT cannot inspect information_schema.TABLES: %s; configure the TaskDeck connection database explicitly", reply.Error.Code)
    }
    var result dbadapter.ExecuteResult
    if err := json.Unmarshal(reply.Payload, &result); err != nil {
        return "", errors.New("COLLECT database discovery returned invalid metadata JSON")
    }
    if err := dbadapter.ValidateExecuteResult(result); err != nil { return "", err }
    if result.Truncated { return "", errors.New("COLLECT metadata discovery was truncated; configure the TaskDeck connection database explicitly") }
    if len(result.Columns) != 2 || !strings.EqualFold(result.Columns[0].Name, "TABLE_SCHEMA") ||
        !strings.EqualFold(result.Columns[1].Name, "TABLE_NAME") {
        return "", errors.New("COLLECT database discovery returned unexpected metadata columns")
    }
    return chooseCollectSchema(names, result.Rows)
}
