package dbadapter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeBrowseRowsPayloadBoundsPagingAndSort(t *testing.T) {
	payload, err := NormalizeBrowseRowsPayload(BrowseRowsPayload{
		Catalog: " app ",
		Name:    " users ",
		Sort:    []RowSort{{Column: " id ", Direction: "DESC"}},
		Filters: []RowFilter{{Column: "email", Operator: "contains", Value: "@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Catalog != "app" || payload.Name != "users" || payload.Limit != 100 {
		t.Fatalf("unexpected normalized payload: %#v", payload)
	}
	if payload.Sort[0].Column != "id" || payload.Sort[0].Direction != "desc" {
		t.Fatalf("unexpected normalized sort: %#v", payload.Sort[0])
	}

	_, err = NormalizeBrowseRowsPayload(BrowseRowsPayload{Name: "users", Offset: -1})
	if err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("expected negative offset rejection, got %v", err)
	}
	_, err = NormalizeBrowseRowsPayload(BrowseRowsPayload{Name: "users", Limit: MaxRows + 1})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected oversized limit rejection, got %v", err)
	}
}

func TestValidateBrowseRowsResultRequiresBoundedAlignedRows(t *testing.T) {
	total := int64(2)
	result := BrowseRowsResult{
		Columns: []BrowseColumn{{Name: "id", Type: "bigint", Editable: true, Identity: true}, {Name: "name", Editable: true}},
		Rows: []BrowseRow{
			{Values: []interface{}{float64(1), "one"}, Identity: map[string]interface{}{"id": float64(1)}},
			{Values: []interface{}{float64(2), "two"}, Identity: map[string]interface{}{"id": float64(2)}},
		},
		Offset: 0, Limit: 100, TotalRows: &total, Editable: true,
	}
	if err := ValidateBrowseRowsResult(result); err != nil {
		t.Fatal(err)
	}
	result.Rows[0].Values = []interface{}{float64(1)}
	if err := ValidateBrowseRowsResult(result); err == nil || !strings.Contains(err.Error(), "cells") {
		t.Fatalf("expected row width rejection, got %v", err)
	}
}

func TestNormalizeMutateRowsPayloadRequiresStableIdentity(t *testing.T) {
	payload, err := NormalizeMutateRowsPayload(MutateRowsPayload{
		Name: "users",
		Mutations: []RowMutation{
			{Action: "insert", Values: map[string]interface{}{"name": "new"}},
			{Action: "update", Identity: map[string]interface{}{"id": float64(7)}, Values: map[string]interface{}{"name": "changed"}},
			{Action: "delete", Identity: map[string]interface{}{"id": float64(8)}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Mutations) != 3 {
		t.Fatalf("unexpected mutation count %d", len(payload.Mutations))
	}

	_, err = NormalizeMutateRowsPayload(MutateRowsPayload{
		Name: "users",
		Mutations: []RowMutation{{Action: "update", Values: map[string]interface{}{"name": "unsafe"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected missing identity rejection, got %v", err)
	}
}

func TestProtocolAcceptsWorkbenchOperationsAndValidatesBrowseResponse(t *testing.T) {
	for _, operation := range []Operation{OpBrowseRows, OpMutateRows, OpObjectAction} {
		if !KnownOperation(operation) {
			t.Fatalf("operation %q should be known", operation)
		}
	}
	result := BrowseRowsResult{
		Columns: []BrowseColumn{{Name: "id", Identity: true}},
		Rows:    []BrowseRow{{Values: []interface{}{float64(1)}, Identity: map[string]interface{}{"id": float64(1)}}},
		Offset: 0,
		Limit:  100,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	env := Envelope{
		Protocol: ProtocolName, Version: ProtocolVersion, Type: "response",
		RequestID: "r1", Operation: OpBrowseRows, Payload: raw,
	}
	if err := ValidateResponsePayload(env); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilitySetSupportsWorkbenchOperations(t *testing.T) {
	caps := CapabilitySet{BrowseRows: true, MutateRows: true, ObjectActions: true}
	if !caps.Supports(OpBrowseRows) || !caps.Supports(OpMutateRows) || !caps.Supports(OpObjectAction) {
		t.Fatalf("workbench capabilities are not exposed: %#v", caps)
	}
}
