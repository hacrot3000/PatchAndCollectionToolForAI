package main

import (
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func TestCollectSchemaDiscoveryUniqueAcrossRequestedTables(t *testing.T) {
    tables := []string{"ep00_buff_columns", "ep08_buff_columns", "ep11_buff_columns"}
    rows := [][]interface{}{
        {"haitac", "ep00_buff_columns"},
        {"haitac", "ep08_buff_columns"},
        {"haitac", "ep11_buff_columns"},
        {"backup", "ep00_buff_columns"},
    }
    got, err := chooseCollectSchema(tables, rows)
    if err != nil || got != "haitac" {
        t.Fatalf("expected only schema containing all tables, got %q %v", got, err)
    }
}

func TestCollectSchemaDiscoveryRefusesAmbiguityNoGuess(t *testing.T) {
    rows := [][]interface{}{
        {"haitac", "ep00_buff_columns"},
        {"backup", "ep00_buff_columns"},
    }
    _, err := chooseCollectSchema([]string{"ep00_buff_columns"}, rows)
    if err == nil || !strings.Contains(err.Error(), "ambiguous") ||
        !strings.Contains(err.Error(), "backup") || !strings.Contains(err.Error(), "haitac") {
        t.Fatalf("ambiguous schema must fail with candidates: %v", err)
    }
}

func TestCollectSchemaDiscoveryFailsClosedForMissingOrCrossSchemaTables(t *testing.T) {
    tests := []struct{
        name string
        tables []string
        rows [][]interface{}
        message string
    }{
        {"absent", []string{"ep00_buff_columns"}, nil, "not visible"},
        {"case-unsafe", []string{"ep00_buff_columns"}, [][]interface{}{{"haitac", "EP00_buff_columns"}}, "not visible"},
        {"different-db", []string{"ep00_buff_columns", "ep08_buff_columns"},
            [][]interface{}{{"db_a", "ep00_buff_columns"}, {"db_b", "ep08_buff_columns"}}, "not all present"},
        {"malformed", []string{"ep00_buff_columns"}, [][]interface{}{{"haitac"}}, "malformed"},
        {"too-many-metadata", []string{"ep00_buff_columns"}, make([][]interface{}, dbadapter.MaxRows), "metadata row limit"},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T){
            _, err := chooseCollectSchema(tc.tables, tc.rows)
            if err == nil || !strings.Contains(err.Error(), tc.message) {
                t.Fatalf("expected %q failure, got %v", tc.message, err)
            }
        })
    }
}

func TestCollectSourceTableGuardRejectsInjectedAndOverlongNames(t *testing.T) {
    for _, tables := range [][]string{
        {"a; DROP DATABASE mysql"},
        {"db.table"},
        {""},
        {"9bad"},
        {strings.Repeat("a",65)},
        make([]string, collectMaxUnqualifiedTables+1),
    } {
        if err := validateCollectSourceTables(tables); err == nil {
            t.Fatalf("unsafe/unbounded source tables accepted: %#v", tables)
        }
    }
    for _, tables := range [][]string{nil, {"ep00_buff_columns"}, {"_a$1", "Z_02"}} {
        if err := validateCollectSourceTables(tables); err != nil {
            t.Fatalf("valid source tables rejected: %#v %v", tables, err)
        }
    }
}
