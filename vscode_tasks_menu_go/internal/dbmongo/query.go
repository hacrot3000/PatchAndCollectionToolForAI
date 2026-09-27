package dbmongo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type FindQuery struct {
	Op         string                 `json:"op"`
	Database   string                 `json:"database,omitempty"`
	Collection string                 `json:"collection"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Projection map[string]interface{} `json:"projection,omitempty"`
	Sort       map[string]interface{} `json:"sort,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
}

func ParseFindQuery(statement string, maxRows int) (FindQuery, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return FindQuery{}, errors.New("MongoDB query JSON is required")
	}
	if len(statement) > dbadapter.MaxMessageBytes/2 {
		return FindQuery{}, fmt.Errorf("MongoDB query exceeds %d bytes", dbadapter.MaxMessageBytes/2)
	}
	if maxRows < 1 || maxRows > dbadapter.MaxRows {
		return FindQuery{}, fmt.Errorf("MongoDB max rows must be between 1 and %d", dbadapter.MaxRows)
	}

	var query FindQuery
	decoder := json.NewDecoder(strings.NewReader(statement))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&query); err != nil {
		return FindQuery{}, fmt.Errorf("invalid MongoDB query JSON: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return FindQuery{}, errors.New("MongoDB query JSON must contain exactly one object")
		}
		return FindQuery{}, fmt.Errorf("invalid trailing MongoDB query data: %w", err)
	}
	query.Op = strings.ToLower(strings.TrimSpace(query.Op))
	if query.Op == "" {
		query.Op = "find"
	}
	if query.Op != "find" {
		return FindQuery{}, fmt.Errorf("MongoDB operation %q is not supported in the initial read-oriented adapter", query.Op)
	}
	query.Database = strings.TrimSpace(query.Database)
	if err := validateDatabaseName(query.Database, false); err != nil {
		return FindQuery{}, err
	}
	query.Collection = strings.TrimSpace(query.Collection)
	if query.Collection == "" {
		return FindQuery{}, errors.New("MongoDB collection is required")
	}
	if len(query.Collection) > 512 || strings.ContainsAny(query.Collection, "\x00\r\n") {
		return FindQuery{}, errors.New("MongoDB collection is invalid")
	}
	if query.Filter == nil {
		query.Filter = map[string]interface{}{}
	}
	if err := validateNoServerSideCode(query.Filter); err != nil {
		return FindQuery{}, err
	}
	if err := validateNoServerSideCode(query.Projection); err != nil {
		return FindQuery{}, err
	}
	if err := validateNoServerSideCode(query.Sort); err != nil {
		return FindQuery{}, err
	}
	if query.Limit < 0 {
		return FindQuery{}, errors.New("MongoDB limit must not be negative")
	}
	if query.Limit == 0 || query.Limit > maxRows {
		query.Limit = maxRows
	}
	return query, nil
}

func validateNoServerSideCode(value interface{}) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		for key, child := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "$where", "$function", "$accumulator":
				return fmt.Errorf("MongoDB operator %q is not allowed because it can execute server-side code", key)
			}
			if err := validateNoServerSideCode(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range typed {
			if err := validateNoServerSideCode(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func findOperationBody(database string, query FindQuery) (string, error) {
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	queryExpression, err := javascriptJSON(query)
	if err != nil {
		return "", err
	}
	fetchLimit := query.Limit + 1
	body := "    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    const __q = " + queryExpression + ";\n" +
		"    let __cursor = __db.getCollection(__q.collection).find(__q.filter || {}, __q.projection || undefined);\n" +
		"    if (__q.sort && Object.keys(__q.sort).length) __cursor = __cursor.sort(__q.sort);\n" +
		"    const __docs = __cursor.limit(" + fmt.Sprint(fetchLimit) + ").toArray();\n" +
		"    return {documents:__docs.slice(0," + fmt.Sprint(query.Limit) + "),truncated:__docs.length>" + fmt.Sprint(query.Limit) + "};"
	return body, nil
}

func documentsToExecuteResult(documents []interface{}, truncated bool) (dbadapter.ExecuteResult, error) {
	result := dbadapter.ExecuteResult{
		Columns:   []dbadapter.Column{{Name: "document", Type: "document"}},
		Rows:      make([][]interface{}, 0, len(documents)),
		Truncated: truncated,
	}
	if len(documents) > dbadapter.MaxRows {
		documents = documents[:dbadapter.MaxRows]
		result.Truncated = true
	}
	for _, document := range documents {
		raw, err := json.Marshal(document)
		if err != nil {
			return dbadapter.ExecuteResult{}, fmt.Errorf("encode MongoDB document: %w", err)
		}
		cell := document
		if len(raw) > dbadapter.MaxCellBytes {
			cell = map[string]interface{}{
				"_taskdeck_truncated": true,
				"_taskdeck_bytes":     len(raw),
			}
			result.Truncated = true
		}
		result.Rows = append(result.Rows, []interface{}{cell})
	}
	if err := dbadapter.ValidateExecuteResult(result); err != nil {
		return dbadapter.ExecuteResult{}, err
	}
	return result, nil
}

func compactJSON(value interface{}) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
