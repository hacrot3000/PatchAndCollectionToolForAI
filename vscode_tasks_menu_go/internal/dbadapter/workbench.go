package dbadapter

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MaxMutations          = 100
	MaxSortColumns        = 16
	MaxFilters            = 16
	MaxIdentityColumns    = 64
	MaxObjectActionReason = 1024
)

type RowSort struct {
	Column    string `json:"column"`
	Direction string `json:"direction"`
}

type RowFilter struct {
	Column   string      `json:"column"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value,omitempty"`
}

type BrowseRowsPayload struct {
	Catalog string      `json:"catalog,omitempty"`
	Schema  string      `json:"schema,omitempty"`
	Kind    string      `json:"kind,omitempty"`
	Name    string      `json:"name"`
	Offset  int         `json:"offset,omitempty"`
	Limit   int         `json:"limit,omitempty"`
	Sort    []RowSort   `json:"sort,omitempty"`
	Filters []RowFilter `json:"filters,omitempty"`
}

type BrowseColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Nullable bool   `json:"nullable,omitempty"`
	Editable bool   `json:"editable,omitempty"`
	Identity bool   `json:"identity,omitempty"`
}

type BrowseRow struct {
	Values   []interface{}          `json:"values"`
	Identity map[string]interface{} `json:"identity,omitempty"`
}

type BrowseRowsResult struct {
	Columns           []BrowseColumn `json:"columns"`
	Rows              []BrowseRow    `json:"rows"`
	Offset            int            `json:"offset"`
	Limit             int            `json:"limit"`
	HasMore           bool           `json:"has_more"`
	TotalRows         *int64         `json:"total_rows,omitempty"`
	Editable          bool           `json:"editable"`
	EditabilityReason string         `json:"editability_reason,omitempty"`
}

type RowMutation struct {
	Action   string                 `json:"action"`
	Identity map[string]interface{} `json:"identity,omitempty"`
	Values   map[string]interface{} `json:"values,omitempty"`
}

type MutateRowsPayload struct {
	Catalog   string        `json:"catalog,omitempty"`
	Schema    string        `json:"schema,omitempty"`
	Kind      string        `json:"kind,omitempty"`
	Name      string        `json:"name"`
	Mutations []RowMutation `json:"mutations"`
}

type RowMutationResult struct {
	Index        int            `json:"index"`
	Action       string         `json:"action"`
	AffectedRows int64          `json:"affected_rows,omitempty"`
	Error        *ProtocolError `json:"error,omitempty"`
}

type MutateRowsResult struct {
	Results []RowMutationResult `json:"results"`
}

type ObjectActionPayload struct {
	Catalog string `json:"catalog,omitempty"`
	Schema  string `json:"schema,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name"`
	Action  string `json:"action"`
}

type ObjectActionResult struct {
	Count        *int64 `json:"count,omitempty"`
	AffectedRows int64  `json:"affected_rows,omitempty"`
	Message      string `json:"message,omitempty"`
}

func NormalizeBrowseRowsPayload(payload BrowseRowsPayload) (BrowseRowsPayload, error) {
	payload.Catalog = strings.TrimSpace(payload.Catalog)
	payload.Schema = strings.TrimSpace(payload.Schema)
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Name = strings.TrimSpace(payload.Name)
	if err := validateText("database object name", payload.Name, 512, true); err != nil {
		return BrowseRowsPayload{}, err
	}
	if err := validateOptionalScope(payload.Catalog, payload.Schema, payload.Kind); err != nil {
		return BrowseRowsPayload{}, err
	}
	if payload.Offset < 0 {
		return BrowseRowsPayload{}, fmt.Errorf("database row offset must not be negative")
	}
	if payload.Limit == 0 {
		payload.Limit = 100
	}
	if payload.Limit < 1 || payload.Limit > MaxRows {
		return BrowseRowsPayload{}, fmt.Errorf("database row limit must be between 1 and %d", MaxRows)
	}
	if len(payload.Sort) > MaxSortColumns {
		return BrowseRowsPayload{}, fmt.Errorf("database row sort exceeds %d columns", MaxSortColumns)
	}
	for i := range payload.Sort {
		payload.Sort[i].Column = strings.TrimSpace(payload.Sort[i].Column)
		payload.Sort[i].Direction = strings.ToLower(strings.TrimSpace(payload.Sort[i].Direction))
		if err := validateText("database sort column", payload.Sort[i].Column, 512, true); err != nil {
			return BrowseRowsPayload{}, fmt.Errorf("sort %d: %w", i+1, err)
		}
		switch payload.Sort[i].Direction {
		case "asc", "desc":
		default:
			return BrowseRowsPayload{}, fmt.Errorf("sort %d has unsupported direction %q", i+1, payload.Sort[i].Direction)
		}
	}
	if len(payload.Filters) > MaxFilters {
		return BrowseRowsPayload{}, fmt.Errorf("database row filters exceed %d entries", MaxFilters)
	}
	for i := range payload.Filters {
		filter := &payload.Filters[i]
		filter.Column = strings.TrimSpace(filter.Column)
		filter.Operator = strings.ToLower(strings.TrimSpace(filter.Operator))
		if err := validateText("database filter column", filter.Column, 512, true); err != nil {
			return BrowseRowsPayload{}, fmt.Errorf("filter %d: %w", i+1, err)
		}
		switch filter.Operator {
		case "eq", "ne", "lt", "lte", "gt", "gte", "contains", "starts_with", "is_null", "not_null":
		default:
			return BrowseRowsPayload{}, fmt.Errorf("filter %d has unsupported operator %q", i+1, filter.Operator)
		}
		if filter.Operator != "is_null" && filter.Operator != "not_null" {
			if err := validateWorkbenchCell(filter.Value); err != nil {
				return BrowseRowsPayload{}, fmt.Errorf("filter %d value: %w", i+1, err)
			}
		}
	}
	return payload, nil
}

func ValidateBrowseRowsResult(result BrowseRowsResult) error {
	if result.Offset < 0 {
		return fmt.Errorf("database browse result offset must not be negative")
	}
	if result.Limit < 1 || result.Limit > MaxRows {
		return fmt.Errorf("database browse result limit must be between 1 and %d", MaxRows)
	}
	if len(result.Columns) > MaxColumns {
		return fmt.Errorf("database browse result exceeds %d columns", MaxColumns)
	}
	if len(result.Rows) > MaxRows || len(result.Rows) > result.Limit {
		return fmt.Errorf("database browse result exceeds row limit")
	}
	if result.TotalRows != nil && *result.TotalRows < 0 {
		return fmt.Errorf("database browse total_rows must not be negative")
	}
	if err := validateText("database editability reason", strings.TrimSpace(result.EditabilityReason), MaxObjectActionReason, false); err != nil {
		return err
	}
	for i, column := range result.Columns {
		if err := validateText("database browse column name", strings.TrimSpace(column.Name), 512, true); err != nil {
			return fmt.Errorf("column %d: %w", i+1, err)
		}
		if err := validateText("database browse column type", strings.TrimSpace(column.Type), 256, false); err != nil {
			return fmt.Errorf("column %d: %w", i+1, err)
		}
	}
	for i, row := range result.Rows {
		if len(row.Values) != len(result.Columns) {
			return fmt.Errorf("row %d has %d cells for %d columns", i+1, len(row.Values), len(result.Columns))
		}
		for j, cell := range row.Values {
			if err := validateWorkbenchCell(cell); err != nil {
				return fmt.Errorf("row %d cell %d: %w", i+1, j+1, err)
			}
		}
		if len(row.Identity) > MaxIdentityColumns {
			return fmt.Errorf("row %d identity exceeds %d columns", i+1, MaxIdentityColumns)
		}
		for key, value := range row.Identity {
			if err := validateText("database row identity column", strings.TrimSpace(key), 512, true); err != nil {
				return fmt.Errorf("row %d identity: %w", i+1, err)
			}
			if err := validateWorkbenchCell(value); err != nil {
				return fmt.Errorf("row %d identity %q: %w", i+1, key, err)
			}
		}
	}
	return nil
}

func NormalizeMutateRowsPayload(payload MutateRowsPayload) (MutateRowsPayload, error) {
	payload.Catalog = strings.TrimSpace(payload.Catalog)
	payload.Schema = strings.TrimSpace(payload.Schema)
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Name = strings.TrimSpace(payload.Name)
	if err := validateText("database object name", payload.Name, 512, true); err != nil {
		return MutateRowsPayload{}, err
	}
	if err := validateOptionalScope(payload.Catalog, payload.Schema, payload.Kind); err != nil {
		return MutateRowsPayload{}, err
	}
	if len(payload.Mutations) == 0 || len(payload.Mutations) > MaxMutations {
		return MutateRowsPayload{}, fmt.Errorf("database mutations must contain between 1 and %d entries", MaxMutations)
	}
	for i := range payload.Mutations {
		mutation := &payload.Mutations[i]
		mutation.Action = strings.ToLower(strings.TrimSpace(mutation.Action))
		switch mutation.Action {
		case "insert":
			if len(mutation.Identity) != 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d insert must not include row identity", i+1)
			}
			if len(mutation.Values) == 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d insert requires values", i+1)
			}
		case "update":
			if len(mutation.Identity) == 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d update requires row identity", i+1)
			}
			if len(mutation.Values) == 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d update requires changed values", i+1)
			}
		case "delete":
			if len(mutation.Identity) == 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d delete requires row identity", i+1)
			}
			if len(mutation.Values) != 0 {
				return MutateRowsPayload{}, fmt.Errorf("mutation %d delete must not include values", i+1)
			}
		default:
			return MutateRowsPayload{}, fmt.Errorf("mutation %d has unsupported action %q", i+1, mutation.Action)
		}
		if err := validateWorkbenchMap("identity", mutation.Identity, MaxIdentityColumns); err != nil {
			return MutateRowsPayload{}, fmt.Errorf("mutation %d: %w", i+1, err)
		}
		if err := validateWorkbenchMap("values", mutation.Values, MaxColumns); err != nil {
			return MutateRowsPayload{}, fmt.Errorf("mutation %d: %w", i+1, err)
		}
	}
	return payload, nil
}

func ValidateMutateRowsResult(result MutateRowsResult) error {
	if len(result.Results) > MaxMutations {
		return fmt.Errorf("database mutation result exceeds %d entries", MaxMutations)
	}
	for i, item := range result.Results {
		if item.Index != i {
			return fmt.Errorf("mutation result %d has index %d; expected %d", i+1, item.Index, i)
		}
		switch strings.ToLower(strings.TrimSpace(item.Action)) {
		case "insert", "update", "delete":
		default:
			return fmt.Errorf("mutation result %d has unsupported action %q", i+1, item.Action)
		}
		if item.AffectedRows < 0 {
			return fmt.Errorf("mutation result %d has negative affected_rows", i+1)
		}
		if item.Error != nil {
			if err := validateToken("mutation error code", strings.TrimSpace(item.Error.Code), 64, true); err != nil {
				return fmt.Errorf("mutation result %d: %w", i+1, err)
			}
			if err := validateText("mutation error message", strings.TrimSpace(item.Error.Message), 4096, true); err != nil {
				return fmt.Errorf("mutation result %d: %w", i+1, err)
			}
		}
	}
	return nil
}

func NormalizeObjectActionPayload(payload ObjectActionPayload) (ObjectActionPayload, error) {
	payload.Catalog = strings.TrimSpace(payload.Catalog)
	payload.Schema = strings.TrimSpace(payload.Schema)
	payload.Kind = strings.TrimSpace(payload.Kind)
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Action = strings.ToLower(strings.TrimSpace(payload.Action))
	if err := validateText("database object name", payload.Name, 512, true); err != nil {
		return ObjectActionPayload{}, err
	}
	if err := validateOptionalScope(payload.Catalog, payload.Schema, payload.Kind); err != nil {
		return ObjectActionPayload{}, err
	}
	switch payload.Action {
	case "count_rows", "truncate", "drop":
	default:
		return ObjectActionPayload{}, fmt.Errorf("unsupported database object action %q", payload.Action)
	}
	return payload, nil
}

func ValidateObjectActionResult(result ObjectActionResult) error {
	if result.Count != nil && *result.Count < 0 {
		return fmt.Errorf("database object action count must not be negative")
	}
	if result.AffectedRows < 0 {
		return fmt.Errorf("database object action affected_rows must not be negative")
	}
	return validateText("database object action message", strings.TrimSpace(result.Message), MaxObjectActionReason, false)
}

func validateOptionalScope(catalog, schema, kind string) error {
	for label, value := range map[string]string{
		"database catalog": catalog,
		"database schema":  schema,
		"database kind":    kind,
	} {
		if err := validateText(label, value, 512, false); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkbenchMap(label string, values map[string]interface{}, max int) error {
	if len(values) > max {
		return fmt.Errorf("database row %s exceeds %d columns", label, max)
	}
	for key, value := range values {
		if err := validateText("database row "+label+" column", strings.TrimSpace(key), 512, true); err != nil {
			return err
		}
		if err := validateWorkbenchCell(value); err != nil {
			return fmt.Errorf("column %q: %w", key, err)
		}
	}
	return nil
}

func validateWorkbenchCell(value interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("value cannot be encoded: %w", err)
	}
	if len(raw) > MaxCellBytes {
		return fmt.Errorf("value exceeds %d bytes", MaxCellBytes)
	}
	return nil
}
