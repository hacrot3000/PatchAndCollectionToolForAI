package dbmysql

import (
	"context"
	"fmt"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func (h *Handler) attachEditableSelectInfo(ctx context.Context, config Config, statement string, result *dbadapter.ExecuteResult) {
	if result == nil || firstSQLKeyword(statement) != "SELECT" {
		return
	}
	info := &dbadapter.ExecuteEditInfo{Editable: false}
	result.Edit = info

	target, err := analyzeEditableSelect(statement)
	if err != nil {
		info.EditabilityReason = trimEditReason(err.Error())
		return
	}
	catalog := firstNonEmpty(target.Catalog, config.Database, h.config.Database)
	info.Catalog = catalog
	info.Kind = "table"
	info.Name = target.Table
	if catalog == "" {
		info.EditabilityReason = "No database is selected for the target table"
		return
	}

	kind, err := h.mysqlObjectKind(ctx, catalog, target.Table)
	if err != nil {
		info.EditabilityReason = trimEditReason("Cannot inspect target table: " + err.Error())
		return
	}
	info.Kind = kind
	if kind != "table" {
		info.EditabilityReason = "Only direct SELECTs from a base table are editable"
		return
	}
	columns, identityColumns, err := h.browseMetadata(ctx, catalog, target.Table)
	if err != nil {
		info.EditabilityReason = trimEditReason("Cannot inspect target table columns: " + err.Error())
		return
	}
	if len(columns) == 0 {
		info.EditabilityReason = "Target table has no visible columns"
		return
	}
	columnByName := make(map[string]mysqlBrowseColumn, len(columns))
	for _, column := range columns {
		columnByName[strings.ToLower(column.Name)] = column
	}
	if len(identityColumns) == 0 {
		info.EditabilityReason = "Target table has no non-null primary or unique key"
		return
	}
	if h.config.ReadOnly || config.ReadOnly {
		info.EditabilityReason = "Connection is read-only"
		return
	}

	resultIndex := make(map[string]int, len(result.Columns))
	info.Columns = make([]dbadapter.BrowseColumn, len(result.Columns))
	identitySet := make(map[string]struct{}, len(identityColumns))
	for _, name := range identityColumns {
		identitySet[strings.ToLower(name)] = struct{}{}
	}
	for index, resultColumn := range result.Columns {
		key := strings.ToLower(strings.TrimSpace(resultColumn.Name))
		meta, ok := columnByName[key]
		if !ok {
			info.Columns = nil
			info.EditabilityReason = fmt.Sprintf("Result column %q is not a direct target-table column", resultColumn.Name)
			return
		}
		if _, duplicate := resultIndex[key]; duplicate {
			info.Columns = nil
			info.EditabilityReason = fmt.Sprintf("Result contains duplicate column %q", resultColumn.Name)
			return
		}
		resultIndex[key] = index
		_, identity := identitySet[key]
		info.Columns[index] = dbadapter.BrowseColumn{
			Name:       meta.Name,
			Type:       meta.Type,
			Nullable:   meta.Nullable,
			Editable:   true,
			Identity:   identity,
		}
	}
	for _, identityColumn := range identityColumns {
		if _, ok := resultIndex[strings.ToLower(identityColumn)]; !ok {
			info.EditabilityReason = "Select the primary/unique key column(s) to enable editing: " + strings.Join(identityColumns, ", ")
			for i := range info.Columns {
				info.Columns[i].Editable = false
			}
			return
		}
	}

	info.RowIdentities = make([]map[string]interface{}, len(result.Rows))
	for rowIndex, row := range result.Rows {
		identity := make(map[string]interface{}, len(identityColumns))
		for _, identityColumn := range identityColumns {
			value := row[resultIndex[strings.ToLower(identityColumn)]]
			if value == nil {
				info.RowIdentities = nil
				info.EditabilityReason = "A result row has a NULL primary/unique key value"
				for i := range info.Columns {
					info.Columns[i].Editable = false
				}
				return
			}
			identity[identityColumn] = value
		}
		info.RowIdentities[rowIndex] = identity
	}
	info.Editable = true
	info.EditabilityReason = ""
}

func (h *Handler) mysqlObjectKind(ctx context.Context, catalog, name string) (string, error) {
	query := "SELECT CASE WHEN TABLE_TYPE = 'BASE TABLE' THEN 'table' WHEN TABLE_TYPE = 'VIEW' THEN 'view' ELSE 'object' END AS kind " +
		"FROM information_schema.TABLES WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) + " LIMIT 1"
	result, err := h.query(ctx, query, 1)
	if err != nil {
		return "", err
	}
	if len(result.Rows) != 1 {
		return "", fmt.Errorf("target object does not exist")
	}
	index, err := resultColumnIndex(result, "kind")
	if err != nil {
		return "", err
	}
	kind := strings.ToLower(strings.TrimSpace(resultCellString(result.Rows[0][index])))
	switch kind {
	case "table", "view":
		return kind, nil
	default:
		return "object", nil
	}
}

func trimEditReason(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
