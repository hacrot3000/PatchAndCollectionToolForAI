package dbmysql

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

type Handler struct {
	manifest  dbadapter.Manifest
	client    Client
	config    Config
	connected bool
}

func NewHandler(taskdeckExecutable string) (*Handler, error) {
	manifest, err := BuiltinManifest(taskdeckExecutable)
	if err != nil {
		return nil, err
	}
	return &Handler{manifest: manifest}, nil
}

func (h *Handler) Manifest() dbadapter.Manifest {
	return h.manifest
}

func (h *Handler) Handle(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	switch request.Operation {
	case dbadapter.OpConnect:
		return h.connect(ctx, request)
	case dbadapter.OpDisconnect:
		h.config.Secret = ""
		h.config = Config{}
		h.client = Client{}
		h.connected = false
		return map[string]bool{"disconnected": true}, nil
	case dbadapter.OpPing:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		result, err := h.query(ctx, "SELECT 1 AS taskdeck_ping", 1)
		if err != nil {
			return nil, mysqlProtocolError("PING_FAILED", err)
		}
		if len(result.Rows) != 1 {
			return nil, &dbadapter.ProtocolError{Code: "PING_FAILED", Message: "MySQL ping returned no row"}
		}
		return map[string]bool{"ok": true}, nil
	case dbadapter.OpListCatalogs:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		return h.listCatalogs(ctx)
	case dbadapter.OpListObjects:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ListObjectsPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		return h.listObjects(ctx, payload)
	case dbadapter.OpDescribeObject:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.DescribeObjectPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		return h.describeObject(ctx, payload)
	case dbadapter.OpBrowseRows:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.BrowseRowsPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeBrowseRowsPayload(payload)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_BROWSE", err)
		}
		return h.browseRows(ctx, normalized)
	case dbadapter.OpMutateRows:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		if h.config.ReadOnly {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "MySQL connection is read-only"}
		}
		var payload dbadapter.MutateRowsPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeMutateRowsPayload(payload)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_MUTATION", err)
		}
		return h.mutateRows(ctx, normalized)
	case dbadapter.OpObjectAction:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ObjectActionPayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeObjectActionPayload(payload)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_OBJECT_ACTION", err)
		}
		return h.objectAction(ctx, normalized)
	case dbadapter.OpExecute:
		if protocolErr := h.requireConnected(); protocolErr != nil {
			return nil, protocolErr
		}
		var payload dbadapter.ExecutePayload
		if err := decodePayload(request.Payload, &payload); err != nil {
			return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
		}
		normalized, err := dbadapter.NormalizeExecutePayload(payload)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_QUERY", err)
		}
		statement, err := normalizeSingleStatement(normalized.Statement)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_QUERY", err)
		}
		if h.config.ReadOnly {
			if err := readOnlyStatement(statement); err != nil {
				return nil, mysqlProtocolError("READ_ONLY", err)
			}
		}
		config := h.config
		if catalog := firstNonEmpty(normalized.Catalog, normalized.Schema); catalog != "" {
			config.Database = catalog
		}
		result, err := h.queryWithConfig(ctx, config, statement, normalized.MaxRows)
		if err != nil {
			return nil, mysqlProtocolError("QUERY_FAILED", err)
		}
		h.attachEditableSelectInfo(ctx, config, statement, &result)
		return result, nil
	default:
		return nil, &dbadapter.ProtocolError{
			Code:    "UNSUPPORTED_OPERATION",
			Message: fmt.Sprintf("MySQL adapter does not support operation %q", request.Operation),
		}
	}
}

func (h *Handler) connect(ctx context.Context, request dbadapter.Envelope) (interface{}, *dbadapter.ProtocolError) {
	var payload dbadapter.ConnectPayload
	if err := decodePayload(request.Payload, &payload); err != nil {
		return nil, mysqlProtocolError("INVALID_PAYLOAD", err)
	}
	config, err := ConfigFromConnectPayload(payload)
	if err != nil {
		return nil, mysqlProtocolError("INVALID_CONFIG", err)
	}
	client, err := FindClient()
	if err != nil {
		return nil, mysqlProtocolError("CLIENT_UNAVAILABLE", err)
	}

	output, err := runClient(ctx, client, config, "SELECT 1 AS taskdeck_connect")
	if err != nil {
		config.Secret = ""
		return nil, mysqlProtocolError("CONNECT_FAILED", err)
	}
	result, err := parseXMLResult(output.stdout, 1)
	if err != nil || len(result.Rows) != 1 {
		config.Secret = ""
		if err == nil {
			err = errors.New("MySQL connection probe returned no row")
		}
		return nil, mysqlProtocolError("CONNECT_FAILED", err)
	}

	h.config.Secret = ""
	h.config = config
	h.client = client
	h.connected = true
	return map[string]interface{}{
		"connected": true,
		"client": map[string]string{
			"flavor":  client.Flavor,
			"version": client.Version,
		},
	}, nil
}

func (h *Handler) requireConnected() *dbadapter.ProtocolError {
	if h == nil || !h.connected {
		return &dbadapter.ProtocolError{Code: "NOT_CONNECTED", Message: "MySQL adapter is not connected"}
	}
	return nil
}

func (h *Handler) query(ctx context.Context, statement string, maxRows int) (dbadapter.ExecuteResult, error) {
	return h.queryWithConfig(ctx, h.config, statement, maxRows)
}

func (h *Handler) queryWithConfig(ctx context.Context, config Config, statement string, maxRows int) (dbadapter.ExecuteResult, error) {
	output, err := runClient(ctx, h.client, config, statement)
	if err != nil {
		return dbadapter.ExecuteResult{}, err
	}
	return parseXMLResult(output.stdout, maxRows)
}

func (h *Handler) listCatalogs(ctx context.Context) (interface{}, *dbadapter.ProtocolError) {
	result, err := h.query(
		ctx,
		"SELECT SCHEMA_NAME AS name FROM information_schema.SCHEMATA ORDER BY SCHEMA_NAME",
		dbadapter.MaxRows,
	)
	if err != nil {
		return nil, mysqlProtocolError("LIST_CATALOGS_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return []dbadapter.Object{}, nil
	}
	index, err := resultColumnIndex(result, "name")
	if err != nil {
		return nil, mysqlProtocolError("LIST_CATALOGS_FAILED", err)
	}
	objects := make([]dbadapter.Object, 0, len(result.Rows))
	for _, row := range result.Rows {
		name := resultCellString(row[index])
		if name == "" {
			continue
		}
		objects = append(objects, dbadapter.Object{Kind: "database", Name: name, Catalog: name})
	}
	return objects, nil
}

func (h *Handler) listObjects(ctx context.Context, payload dbadapter.ListObjectsPayload) (interface{}, *dbadapter.ProtocolError) {
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{
			Code:    "CATALOG_REQUIRED",
			Message: "MySQL catalog/database is required",
		}
	}
	query := "SELECT TABLE_SCHEMA AS catalog, TABLE_NAME AS name, " +
		"CASE WHEN TABLE_TYPE = 'VIEW' THEN 'view' ELSE 'table' END AS kind " +
		"FROM information_schema.TABLES WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" ORDER BY TABLE_NAME"
	result, err := h.query(ctx, query, dbadapter.MaxRows)
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return map[string]interface{}{
			"objects":   []dbadapter.Object{},
			"truncated": result.Truncated,
		}, nil
	}
	catalogIndex, err := resultColumnIndex(result, "catalog")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	nameIndex, err := resultColumnIndex(result, "name")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	kindIndex, err := resultColumnIndex(result, "kind")
	if err != nil {
		return nil, mysqlProtocolError("LIST_OBJECTS_FAILED", err)
	}
	objects := make([]dbadapter.Object, 0, len(result.Rows))
	for _, row := range result.Rows {
		kind := resultCellString(row[kindIndex])
		if payload.Kind != "" && !strings.EqualFold(strings.TrimSpace(payload.Kind), kind) {
			continue
		}
		objects = append(objects, dbadapter.Object{
			Kind:    kind,
			Name:    resultCellString(row[nameIndex]),
			Catalog: resultCellString(row[catalogIndex]),
		})
	}
	return map[string]interface{}{
		"objects":   objects,
		"truncated": result.Truncated,
	}, nil
}

func (h *Handler) describeObject(ctx context.Context, payload dbadapter.DescribeObjectPayload) (interface{}, *dbadapter.ProtocolError) {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_REQUIRED", Message: "MySQL object name is required"}
	}
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_REQUIRED", Message: "MySQL catalog/database is required"}
	}
	query := "SELECT COLUMN_NAME AS name, COLUMN_TYPE AS type, IS_NULLABLE AS nullable, " +
		"COLUMN_DEFAULT AS default_value, EXTRA AS extra " +
		"FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) +
		" ORDER BY ORDINAL_POSITION"
	result, err := h.query(ctx, query, dbadapter.MaxRows)
	if err != nil {
		return nil, mysqlProtocolError("DESCRIBE_FAILED", err)
	}
	if len(result.Rows) == 0 {
		return map[string]interface{}{
			"kind":      firstNonEmpty(strings.TrimSpace(payload.Kind), "table"),
			"name":      name,
			"catalog":   catalog,
			"columns":   []map[string]interface{}{},
			"indexes":   []map[string]interface{}{},
			"truncated": result.Truncated,
		}, nil
	}

	indexes := make(map[string]int)
	for _, column := range []string{"name", "type", "nullable", "default_value", "extra"} {
		index, err := resultColumnIndex(result, column)
		if err != nil {
			return nil, mysqlProtocolError("DESCRIBE_FAILED", err)
		}
		indexes[column] = index
	}
	columns := make([]map[string]interface{}, 0, len(result.Rows))
	for _, row := range result.Rows {
		column := map[string]interface{}{
			"name":     resultCellString(row[indexes["name"]]),
			"type":     resultCellString(row[indexes["type"]]),
			"nullable": strings.EqualFold(resultCellString(row[indexes["nullable"]]), "YES"),
			"extra":    resultCellString(row[indexes["extra"]]),
		}
		if row[indexes["default_value"]] != nil {
			column["default"] = resultCellString(row[indexes["default_value"]])
		} else {
			column["default"] = nil
		}
		columns = append(columns, column)
	}
	indexDetails, indexTruncated, err := h.describeIndexes(ctx, catalog, name)
	if err != nil {
		return nil, mysqlProtocolError("DESCRIBE_FAILED", err)
	}
	return map[string]interface{}{
		"kind":      firstNonEmpty(strings.TrimSpace(payload.Kind), "table"),
		"name":      name,
		"catalog":   catalog,
		"columns":   columns,
		"indexes":   indexDetails,
		"truncated": result.Truncated || indexTruncated,
	}, nil
}

func (h *Handler) describeIndexes(ctx context.Context, catalog, name string) ([]map[string]interface{}, bool, error) {
	query := "/* taskdeck_describe_indexes */ SELECT INDEX_NAME AS name, NON_UNIQUE AS non_unique, " +
		"COLUMN_NAME AS column_name, SEQ_IN_INDEX AS seq, INDEX_TYPE AS index_type " +
		"FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) +
		" ORDER BY INDEX_NAME, SEQ_IN_INDEX"
	result, err := h.query(ctx, query, dbadapter.MaxRows)
	if err != nil {
		return nil, false, err
	}
	if len(result.Rows) == 0 {
		return []map[string]interface{}{}, result.Truncated, nil
	}
	fields := map[string]int{}
	for _, field := range []string{"name", "non_unique", "column_name", "seq", "index_type"} {
		index, err := resultColumnIndex(result, field)
		if err != nil {
			return nil, false, err
		}
		fields[field] = index
	}
	indexes := make([]map[string]interface{}, 0, len(result.Rows))
	for _, row := range result.Rows {
		indexes = append(indexes, map[string]interface{}{
			"name":        resultCellString(row[fields["name"]]),
			"column_name": resultCellString(row[fields["column_name"]]),
			"sequence":    resultCellString(row[fields["seq"]]),
			"unique":      resultCellString(row[fields["non_unique"]]) == "0",
			"type":        resultCellString(row[fields["index_type"]]),
		})
	}
	return indexes, result.Truncated, nil
}

type mysqlBrowseColumn struct {
	Name         string
	Type         string
	Nullable     bool
	CharacterSet string
	Collation    string
}

type mysqlIndexColumn struct {
	IndexName string
	Name      string
	Nullable  bool
}



func (h *Handler) objectAction(ctx context.Context, payload dbadapter.ObjectActionPayload) (interface{}, *dbadapter.ProtocolError) {
	kind := strings.ToLower(strings.TrimSpace(payload.Kind))
	if kind != "" && kind != "table" && kind != "view" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "MySQL object action supports tables and views only"}
	}
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_REQUIRED", Message: "MySQL catalog/database is required"}
	}
	objectSQL, err := mysqlQualifiedIdentifier(catalog, payload.Name)
	if err != nil {
		return nil, mysqlProtocolError("INVALID_OBJECT", err)
	}
	switch payload.Action {
	case "count_rows":
		result, err := h.query(ctx, "/* taskdeck_count */ SELECT COUNT(*) AS row_count FROM "+objectSQL, 1)
		if err != nil {
			return nil, mysqlProtocolError("COUNT_FAILED", err)
		}
		if len(result.Rows) != 1 {
			return nil, &dbadapter.ProtocolError{Code: "COUNT_FAILED", Message: "MySQL count returned no row"}
		}
		index, err := resultColumnIndex(result, "row_count")
		if err != nil {
			return nil, mysqlProtocolError("COUNT_FAILED", err)
		}
		count, err := strconv.ParseInt(strings.TrimSpace(resultCellString(result.Rows[0][index])), 10, 64)
		if err != nil {
			return nil, mysqlProtocolError("COUNT_FAILED", err)
		}
		out := dbadapter.ObjectActionResult{Count: &count}
		return out, nil
	case "truncate":
		if h.config.ReadOnly {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "MySQL connection is read-only"}
		}
		if kind == "view" {
			return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "MySQL views cannot be truncated"}
		}
		if _, err := runClient(ctx, h.client, h.config, "/* taskdeck_object_action */ TRUNCATE TABLE "+objectSQL); err != nil {
			return nil, mysqlProtocolError("TRUNCATE_FAILED", err)
		}
		return dbadapter.ObjectActionResult{Message: "Table truncated"}, nil
	case "drop":
		if h.config.ReadOnly {
			return nil, &dbadapter.ProtocolError{Code: "READ_ONLY", Message: "MySQL connection is read-only"}
		}
		statement := "DROP TABLE " + objectSQL
		if kind == "view" {
			statement = "DROP VIEW " + objectSQL
		}
		if _, err := runClient(ctx, h.client, h.config, "/* taskdeck_object_action */ "+statement); err != nil {
			return nil, mysqlProtocolError("DROP_FAILED", err)
		}
		return dbadapter.ObjectActionResult{Message: "Object dropped"}, nil
	default:
		return nil, &dbadapter.ProtocolError{Code: "INVALID_OBJECT_ACTION", Message: "Unsupported MySQL object action"}
	}
}

func (h *Handler) mutateRows(ctx context.Context, payload dbadapter.MutateRowsPayload) (interface{}, *dbadapter.ProtocolError) {
	kind := strings.ToLower(strings.TrimSpace(payload.Kind))
	if kind == "view" {
		return nil, &dbadapter.ProtocolError{Code: "READ_ONLY_OBJECT", Message: "MySQL views are not mutated by the grid editor"}
	}
	if kind != "" && kind != "table" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "MySQL row mutations support tables only"}
	}
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_REQUIRED", Message: "MySQL catalog/database is required"}
	}
	columns, identityColumns, err := h.browseMetadata(ctx, catalog, payload.Name)
	if err != nil {
		return nil, mysqlProtocolError("MUTATION_METADATA_FAILED", err)
	}
	if len(columns) == 0 {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_NOT_FOUND", Message: "MySQL table has no visible columns"}
	}
	columnByName := make(map[string]mysqlBrowseColumn, len(columns))
	for _, column := range columns {
		columnByName[strings.ToLower(column.Name)] = column
	}
	tableSQL, err := mysqlQualifiedIdentifier(catalog, payload.Name)
	if err != nil {
		return nil, mysqlProtocolError("INVALID_OBJECT", err)
	}
	needsIdentity := false
	for _, mutation := range payload.Mutations {
		if mutation.Action == "update" || mutation.Action == "delete" {
			needsIdentity = true
			break
		}
	}
	if needsIdentity && len(identityColumns) == 0 {
		return nil, &dbadapter.ProtocolError{
			Code: "ROW_IDENTITY_UNAVAILABLE",
			Message: "MySQL table has no non-null primary or unique key for safe update/delete",
		}
	}

	results := make([]dbadapter.RowMutationResult, 0, len(payload.Mutations))
	for mutationIndex, mutation := range payload.Mutations {
		statement, buildErr := buildMySQLMutation(tableSQL, columnByName, identityColumns, mutation)
		item := dbadapter.RowMutationResult{Index: mutationIndex, Action: mutation.Action}
		if buildErr != nil {
			item.Error = mysqlProtocolError("INVALID_MUTATION", buildErr)
			results = append(results, item)
			continue
		}
		affected, runErr := h.executeAffectedRows(ctx, statement)
		if runErr != nil {
			item.Error = mysqlProtocolError("MUTATION_FAILED", runErr)
			results = append(results, item)
			continue
		}
		item.AffectedRows = affected
		if affected != 1 {
			item.Error = &dbadapter.ProtocolError{
				Code: "ROW_NOT_CHANGED",
				Message: fmt.Sprintf("MySQL %s affected %d rows; expected exactly 1", mutation.Action, affected),
			}
		}
		results = append(results, item)
	}
	out := dbadapter.MutateRowsResult{Results: results}
	if err := dbadapter.ValidateMutateRowsResult(out); err != nil {
		return nil, mysqlProtocolError("MUTATION_RESULT_FAILED", err)
	}
	return out, nil
}

func buildMySQLMutation(tableSQL string, columns map[string]mysqlBrowseColumn, identityColumns []string, mutation dbadapter.RowMutation) (string, error) {
	switch mutation.Action {
	case "insert":
		assignments, values, err := mysqlMutationValues(columns, mutation.Values)
		if err != nil {
			return "", err
		}
		return "INSERT INTO " + tableSQL + " (" + strings.Join(assignments, ", ") + ") VALUES (" + strings.Join(values, ", ") + ")", nil
	case "update":
		where, err := mysqlMutationIdentity(columns, identityColumns, mutation.Identity)
		if err != nil {
			return "", err
		}
		names, values, err := mysqlMutationValues(columns, mutation.Values)
		if err != nil {
			return "", err
		}
		sets := make([]string, len(names))
		for i := range names {
			sets[i] = names[i] + " = " + values[i]
		}
		return "UPDATE " + tableSQL + " SET " + strings.Join(sets, ", ") + " WHERE " + where, nil
	case "delete":
		where, err := mysqlMutationIdentity(columns, identityColumns, mutation.Identity)
		if err != nil {
			return "", err
		}
		return "DELETE FROM " + tableSQL + " WHERE " + where, nil
	default:
		return "", fmt.Errorf("unsupported MySQL mutation %q", mutation.Action)
	}
}

func mysqlMutationValues(columns map[string]mysqlBrowseColumn, values map[string]interface{}) ([]string, []string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	names := make([]string, 0, len(keys))
	expressions := make([]string, 0, len(keys))
	for _, key := range keys {
		column, ok := columns[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			return nil, nil, fmt.Errorf("unknown MySQL column %q", key)
		}
		quoted, err := mysqlIdentifier(column.Name)
		if err != nil {
			return nil, nil, err
		}
		value, err := mysqlValueExpression(values[key])
		if err != nil {
			return nil, nil, fmt.Errorf("column %q: %w", column.Name, err)
		}
		names = append(names, quoted)
		expressions = append(expressions, value)
	}
	return names, expressions, nil
}

func mysqlMutationIdentity(columns map[string]mysqlBrowseColumn, identityColumns []string, identity map[string]interface{}) (string, error) {
	if len(identityColumns) == 0 {
		return "", fmt.Errorf("MySQL row identity is unavailable")
	}
	provided := make(map[string]interface{}, len(identity))
	for key, value := range identity {
		provided[strings.ToLower(strings.TrimSpace(key))] = value
	}
	if len(provided) != len(identityColumns) {
		return "", fmt.Errorf("MySQL row identity does not match the stable key")
	}
	parts := make([]string, 0, len(identityColumns))
	for _, identityColumn := range identityColumns {
		column, ok := columns[strings.ToLower(identityColumn)]
		if !ok {
			return "", fmt.Errorf("MySQL identity column %q is unavailable", identityColumn)
		}
		value, ok := provided[strings.ToLower(identityColumn)]
		if !ok || value == nil {
			return "", fmt.Errorf("MySQL identity column %q is missing", identityColumn)
		}
		quoted, err := mysqlIdentifier(column.Name)
		if err != nil {
			return "", err
		}
		expression, err := mysqlIdentityValueExpression(column, value)
		if err != nil {
			return "", err
		}
		parts = append(parts, quoted+" = "+expression)
	}
	return strings.Join(parts, " AND "), nil
}

func (h *Handler) executeAffectedRows(ctx context.Context, statement string) (int64, error) {
	result, err := h.query(ctx, "/* taskdeck_mutation */ "+statement+"; SELECT ROW_COUNT() AS affected_rows", 1)
	if err != nil {
		return 0, err
	}
	if len(result.Rows) != 1 {
		return 0, fmt.Errorf("MySQL mutation did not return affected row count")
	}
	index, err := resultColumnIndex(result, "affected_rows")
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(resultCellString(result.Rows[0][index]))
	affected, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse MySQL affected row count %q: %w", value, err)
	}
	return affected, nil
}

func (h *Handler) browseRows(ctx context.Context, payload dbadapter.BrowseRowsPayload) (interface{}, *dbadapter.ProtocolError) {
	catalog := firstNonEmpty(payload.Catalog, payload.Schema, h.config.Database)
	if catalog == "" {
		return nil, &dbadapter.ProtocolError{Code: "CATALOG_REQUIRED", Message: "MySQL catalog/database is required"}
	}
	kind := strings.ToLower(strings.TrimSpace(payload.Kind))
	if kind != "" && kind != "table" && kind != "view" {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_TYPE_UNSUPPORTED", Message: "MySQL row browser supports tables and views"}
	}
	columns, identity, err := h.browseMetadata(ctx, catalog, payload.Name)
	if err != nil {
		return nil, mysqlProtocolError("BROWSE_METADATA_FAILED", err)
	}
	if len(columns) == 0 {
		return nil, &dbadapter.ProtocolError{Code: "OBJECT_NOT_FOUND", Message: "MySQL table/view has no visible columns"}
	}
	columnByName := make(map[string]mysqlBrowseColumn, len(columns))
	for _, column := range columns {
		columnByName[strings.ToLower(column.Name)] = column
	}
	tableSQL, err := mysqlQualifiedIdentifier(catalog, payload.Name)
	if err != nil {
		return nil, mysqlProtocolError("INVALID_OBJECT", err)
	}

	whereParts := make([]string, 0, len(payload.Filters))
	for _, filter := range payload.Filters {
		column, ok := columnByName[strings.ToLower(filter.Column)]
		if !ok {
			return nil, &dbadapter.ProtocolError{Code: "INVALID_FILTER", Message: fmt.Sprintf("unknown MySQL filter column %q", filter.Column)}
		}
		expression, err := mysqlFilterExpression(column.Name, filter)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_FILTER", err)
		}
		whereParts = append(whereParts, expression)
	}

	orderParts := make([]string, 0, len(payload.Sort))
	for _, sortItem := range payload.Sort {
		column, ok := columnByName[strings.ToLower(sortItem.Column)]
		if !ok {
			return nil, &dbadapter.ProtocolError{Code: "INVALID_SORT", Message: fmt.Sprintf("unknown MySQL sort column %q", sortItem.Column)}
		}
		quoted, err := mysqlIdentifier(column.Name)
		if err != nil {
			return nil, mysqlProtocolError("INVALID_SORT", err)
		}
		orderParts = append(orderParts, quoted+" "+strings.ToUpper(sortItem.Direction))
	}
	if len(orderParts) == 0 {
		for _, name := range identity {
			quoted, err := mysqlIdentifier(name)
			if err != nil {
				return nil, mysqlProtocolError("INVALID_IDENTITY", err)
			}
			orderParts = append(orderParts, quoted+" ASC")
		}
	}

	fetchLimit := payload.Limit
	if fetchLimit < dbadapter.MaxRows {
		fetchLimit++
	}
	query := "/* taskdeck_browse */ SELECT * FROM " + tableSQL
	if len(whereParts) != 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	if len(orderParts) != 0 {
		query += " ORDER BY " + strings.Join(orderParts, ", ")
	}
	query += " LIMIT " + strconv.Itoa(fetchLimit) + " OFFSET " + strconv.Itoa(payload.Offset)

	result, err := h.query(ctx, query, fetchLimit)
	if err != nil {
		return nil, mysqlProtocolError("BROWSE_FAILED", err)
	}
	hasMore := len(result.Rows) > payload.Limit
	if hasMore {
		result.Rows = result.Rows[:payload.Limit]
	} else if payload.Limit == dbadapter.MaxRows && len(result.Rows) == payload.Limit {
		hasMore = true
	}

	identitySet := make(map[string]struct{}, len(identity))
	for _, name := range identity {
		identitySet[strings.ToLower(name)] = struct{}{}
	}
	editable := !h.config.ReadOnly && kind != "view" && len(identity) != 0
	reason := ""
	switch {
	case h.config.ReadOnly:
		reason = "Connection is read-only"
	case kind == "view":
		reason = "Views are opened read-only"
	case len(identity) == 0:
		reason = "No non-null primary or unique key is available for stable row identity"
	}

	browseColumns := make([]dbadapter.BrowseColumn, len(result.Columns))
	resultIndex := make(map[string]int, len(result.Columns))
	for i, resultColumn := range result.Columns {
		meta, ok := columnByName[strings.ToLower(resultColumn.Name)]
		if !ok {
			return nil, &dbadapter.ProtocolError{Code: "BROWSE_FAILED", Message: fmt.Sprintf("MySQL result returned unknown column %q", resultColumn.Name)}
		}
		_, isIdentity := identitySet[strings.ToLower(meta.Name)]
		browseColumns[i] = dbadapter.BrowseColumn{
			Name: meta.Name, Type: meta.Type, Nullable: meta.Nullable,
			Editable: editable, Identity: isIdentity,
		}
		resultIndex[strings.ToLower(meta.Name)] = i
	}
	rows := make([]dbadapter.BrowseRow, 0, len(result.Rows))
	for _, source := range result.Rows {
		row := dbadapter.BrowseRow{Values: append([]interface{}(nil), source...)}
		if len(identity) != 0 {
			row.Identity = make(map[string]interface{}, len(identity))
			for _, identityColumn := range identity {
				index, ok := resultIndex[strings.ToLower(identityColumn)]
				if !ok {
					return nil, &dbadapter.ProtocolError{Code: "BROWSE_FAILED", Message: "MySQL identity column is missing from row result"}
				}
				row.Identity[identityColumn] = source[index]
			}
		}
		rows = append(rows, row)
	}
	out := dbadapter.BrowseRowsResult{
		Columns: browseColumns, Rows: rows, Offset: payload.Offset, Limit: payload.Limit,
		HasMore: hasMore, Editable: editable, EditabilityReason: reason,
	}
	if err := dbadapter.ValidateBrowseRowsResult(out); err != nil {
		return nil, mysqlProtocolError("BROWSE_RESULT_FAILED", err)
	}
	return out, nil
}

func (h *Handler) browseMetadata(ctx context.Context, catalog, name string) ([]mysqlBrowseColumn, []string, error) {
	columnsQuery := "SELECT COLUMN_NAME AS name, COLUMN_TYPE AS type, IS_NULLABLE AS nullable, " +
		"CHARACTER_SET_NAME AS character_set, COLLATION_NAME AS collation " +
		"FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) + " ORDER BY ORDINAL_POSITION"
	columnResult, err := h.query(ctx, columnsQuery, dbadapter.MaxRows)
	if err != nil {
		return nil, nil, err
	}
	if len(columnResult.Rows) == 0 {
		return []mysqlBrowseColumn{}, nil, nil
	}
	nameIndex, err := resultColumnIndex(columnResult, "name")
	if err != nil {
		return nil, nil, err
	}
	typeIndex, err := resultColumnIndex(columnResult, "type")
	if err != nil {
		return nil, nil, err
	}
	nullableIndex, err := resultColumnIndex(columnResult, "nullable")
	if err != nil {
		return nil, nil, err
	}
	characterSetIndex, err := resultColumnIndex(columnResult, "character_set")
	if err != nil {
		return nil, nil, err
	}
	collationIndex, err := resultColumnIndex(columnResult, "collation")
	if err != nil {
		return nil, nil, err
	}
	columns := make([]mysqlBrowseColumn, 0, len(columnResult.Rows))
	for _, row := range columnResult.Rows {
		columns = append(columns, mysqlBrowseColumn{
			Name:         resultCellString(row[nameIndex]),
			Type:         resultCellString(row[typeIndex]),
			Nullable:     strings.EqualFold(resultCellString(row[nullableIndex]), "YES"),
			CharacterSet: resultCellString(row[characterSetIndex]),
			Collation:    resultCellString(row[collationIndex]),
		})
	}

	indexQuery := "SELECT INDEX_NAME AS index_name, COLUMN_NAME AS column_name, SEQ_IN_INDEX AS seq, " +
		"NULLABLE AS nullable FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = " + mysqlTextExpression(catalog) +
		" AND TABLE_NAME = " + mysqlTextExpression(name) + " AND NON_UNIQUE = 0 " +
		"ORDER BY CASE WHEN INDEX_NAME = 'PRIMARY' THEN 0 ELSE 1 END, INDEX_NAME, SEQ_IN_INDEX"
	indexResult, err := h.query(ctx, indexQuery, dbadapter.MaxRows)
	if err != nil {
		return nil, nil, err
	}
	identity, err := chooseMySQLIdentity(indexResult)
	if err != nil {
		return nil, nil, err
	}
	return columns, identity, nil
}

func mysqlIdentityCandidates(result dbadapter.ExecuteResult) ([][]string, error) {
	if len(result.Rows) == 0 {
		return nil, nil
	}
	indexNameIndex, err := resultColumnIndex(result, "index_name")
	if err != nil {
		return nil, err
	}
	columnNameIndex, err := resultColumnIndex(result, "column_name")
	if err != nil {
		return nil, err
	}
	nullableIndex, err := resultColumnIndex(result, "nullable")
	if err != nil {
		return nil, err
	}
	var (
		current           string
		candidate         []string
		candidateNullable bool
		candidates        [][]string
	)
	flush := func() {
		if len(candidate) != 0 && !candidateNullable {
			candidates = append(candidates, append([]string(nil), candidate...))
		}
		candidate = nil
		candidateNullable = false
	}
	for _, row := range result.Rows {
		indexName := resultCellString(row[indexNameIndex])
		if current != "" && indexName != current {
			flush()
		}
		current = indexName
		candidate = append(candidate, resultCellString(row[columnNameIndex]))
		candidateNullable = candidateNullable || strings.EqualFold(resultCellString(row[nullableIndex]), "YES")
	}
	flush()
	return candidates, nil
}

func chooseMySQLIdentity(result dbadapter.ExecuteResult) ([]string, error) {
	candidates, err := mysqlIdentityCandidates(result)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	return append([]string(nil), candidates[0]...), nil
}

func mysqlQualifiedIdentifier(catalog, name string) (string, error) {
	catalogSQL, err := mysqlIdentifier(catalog)
	if err != nil {
		return "", err
	}
	nameSQL, err := mysqlIdentifier(name)
	if err != nil {
		return "", err
	}
	return catalogSQL + "." + nameSQL, nil
}

func mysqlIdentifier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if err := validateMySQLIdentifier(value); err != nil {
		return "", err
	}
	return "`" + strings.ReplaceAll(value, "`", "``") + "`", nil
}

func validateMySQLIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("MySQL identifier is required")
	}
	if len(value) > 512 {
		return fmt.Errorf("MySQL identifier exceeds 512 bytes")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("MySQL identifier contains unsupported control characters")
	}
	return nil
}

func mysqlFilterExpression(column string, filter dbadapter.RowFilter) (string, error) {
	quoted, err := mysqlIdentifier(column)
	if err != nil {
		return "", err
	}
	switch filter.Operator {
	case "is_null":
		return quoted + " IS NULL", nil
	case "not_null":
		return quoted + " IS NOT NULL", nil
	}
	value, err := mysqlValueExpression(filter.Value)
	if err != nil {
		return "", err
	}
	if filter.Value == nil {
		switch filter.Operator {
		case "eq":
			return quoted + " IS NULL", nil
		case "ne":
			return quoted + " IS NOT NULL", nil
		default:
			return "", fmt.Errorf("MySQL NULL filter supports only eq/ne/is_null/not_null")
		}
	}
	switch filter.Operator {
	case "eq":
		return quoted + " = " + value, nil
	case "ne":
		return quoted + " <> " + value, nil
	case "lt":
		return quoted + " < " + value, nil
	case "lte":
		return quoted + " <= " + value, nil
	case "gt":
		return quoted + " > " + value, nil
	case "gte":
		return quoted + " >= " + value, nil
	case "contains":
		return quoted + " LIKE CONCAT('%', " + value + ", '%')", nil
	case "starts_with":
		return quoted + " LIKE CONCAT(" + value + ", '%')", nil
	default:
		return "", fmt.Errorf("unsupported MySQL filter operator %q", filter.Operator)
	}
}

func mysqlIdentityValueExpression(column mysqlBrowseColumn, value interface{}) (string, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(column.CharacterSet) == "" {
		return mysqlValueExpression(value)
	}
	characterSet := strings.TrimSpace(column.CharacterSet)
	if err := validateMySQLCollationToken(characterSet); err != nil {
		return "", fmt.Errorf("column %q character set: %w", column.Name, err)
	}
	expression := "CONVERT(0x" + hex.EncodeToString([]byte(text)) + " USING " + characterSet + ")"
	if collation := strings.TrimSpace(column.Collation); collation != "" {
		if err := validateMySQLCollationToken(collation); err != nil {
			return "", fmt.Errorf("column %q collation: %w", column.Name, err)
		}
		expression += " COLLATE " + collation
	}
	return expression, nil
}

func validateMySQLCollationToken(value string) error {
	if value == "" {
		return fmt.Errorf("MySQL character set/collation is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("unsupported MySQL character set/collation %q", value)
	}
	return nil
}

func mysqlValueExpression(value interface{}) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return mysqlTextExpression(typed), nil
	case bool:
		if typed {
			return "1", nil
		}
		return "0", nil
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case int32:
		return strconv.FormatInt(int64(typed), 10), nil
	case uint:
		return strconv.FormatUint(uint64(typed), 10), nil
	case uint64:
		return strconv.FormatUint(typed, 10), nil
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), nil
	case json.Number:
		if _, err := typed.Float64(); err != nil {
			return "", fmt.Errorf("invalid numeric MySQL value %q", typed)
		}
		return string(typed), nil
	default:
		return "", fmt.Errorf("unsupported MySQL scalar value type %T", value)
	}
}

func decodePayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid MySQL adapter payload: %w", err)
	}
	return nil
}

func mysqlProtocolError(code string, err error) *dbadapter.ProtocolError {
	message := strings.TrimSpace(err.Error())
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &dbadapter.ProtocolError{Code: code, Message: message}
}

func mysqlTextExpression(value string) string {
	return "CONVERT(0x" + hex.EncodeToString([]byte(value)) + " USING utf8mb4)"
}

func resultColumnIndex(result dbadapter.ExecuteResult, name string) (int, error) {
	for index, column := range result.Columns {
		if strings.EqualFold(column.Name, name) {
			return index, nil
		}
	}
	names := make([]string, 0, len(result.Columns))
	for _, column := range result.Columns {
		names = append(names, column.Name)
	}
	sort.Strings(names)
	return -1, fmt.Errorf("MySQL result missing column %q; got %v", name, names)
}

func resultCellString(value interface{}) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func RunAdapter(ctx context.Context, taskdeckExecutable string) error {
	handler, err := NewHandler(taskdeckExecutable)
	if err != nil {
		return err
	}
	return dbadapter.Serve(ctx, os.Stdin, os.Stdout, handler)
}
