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


func TestWorkbenchBoundsRejectExcessFiltersMutationsAndCellBytes(t *testing.T) {
	filters := make([]RowFilter, MaxFilters+1)
	for i := range filters {
		filters[i] = RowFilter{Column: "id", Operator: "eq", Value: i}
	}
	if _, err := NormalizeBrowseRowsPayload(BrowseRowsPayload{Name: "users", Filters: filters}); err == nil || !strings.Contains(err.Error(), "filters") {
		t.Fatalf("expected filter bound rejection, got %v", err)
	}

	mutations := make([]RowMutation, MaxMutations+1)
	for i := range mutations {
		mutations[i] = RowMutation{Action: "insert", Values: map[string]interface{}{"name": "x"}}
	}
	if _, err := NormalizeMutateRowsPayload(MutateRowsPayload{Name: "users", Mutations: mutations}); err == nil || !strings.Contains(err.Error(), "mutations") {
		t.Fatalf("expected mutation bound rejection, got %v", err)
	}

	oversized := strings.Repeat("x", MaxCellBytes+1)
	result := BrowseRowsResult{
		Columns: []BrowseColumn{{Name: "value"}},
		Rows:    []BrowseRow{{Values: []interface{}{oversized}}},
		Offset:  0,
		Limit:   1,
	}
	if err := ValidateBrowseRowsResult(result); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected browse cell-size rejection, got %v", err)
	}

	if _, err := NormalizeMutateRowsPayload(MutateRowsPayload{
		Name: "users",
		Mutations: []RowMutation{{Action: "insert", Values: map[string]interface{}{"value": oversized}}},
	}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected mutation cell-size rejection, got %v", err)
	}
}

func TestObjectActionRejectsUnknownAction(t *testing.T) {
	if _, err := NormalizeObjectActionPayload(ObjectActionPayload{Name: "users", Action: "rename"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported object action rejection, got %v", err)
	}
}


func TestValidateMutateRowsResultRequiresOrderedIndexes(t *testing.T) {
	valid := MutateRowsResult{Results: []RowMutationResult{
		{Index: 0, Action: "insert", AffectedRows: 1},
		{Index: 1, Action: "update", AffectedRows: 1},
	}}
	if err := ValidateMutateRowsResult(valid); err != nil {
		t.Fatal(err)
	}

	outOfOrder := MutateRowsResult{Results: []RowMutationResult{
		{Index: 1, Action: "insert", AffectedRows: 1},
		{Index: 0, Action: "update", AffectedRows: 1},
	}}
	if err := ValidateMutateRowsResult(outOfOrder); err == nil || !strings.Contains(err.Error(), "expected 0") {
		t.Fatalf("expected out-of-order mutation index rejection, got %v", err)
	}
}
