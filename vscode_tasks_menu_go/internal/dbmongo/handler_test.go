package dbmongo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func writeMongoshHandlerFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mongosh")
	marker := resultMarker
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo '2.5.1-fixture'; exit 0; fi\n" +
		"file=''\nprev=''\n" +
		"for arg in \"$@\"; do if [ \"$prev\" = \"--file\" ]; then file=\"$arg\"; break; fi; prev=\"$arg\"; done\n" +
		"if grep -q '__taskdeckBrowseRows' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"documents\":[{\"_id\":1,\"name\":\"Alice\"},{\"_id\":2,\"name\":\"Bob\"}],\"has_more\":false}}'\n" +
		"elif grep -q '__taskdeckMutateRows' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"results\":[{\"index\":0,\"action\":\"insert\",\"affected_rows\":1},{\"index\":1,\"action\":\"update\",\"affected_rows\":1},{\"index\":2,\"action\":\"delete\",\"affected_rows\":1}]}}'\n" +
		"elif grep -q '__taskdeckObjectAction' \"$file\" && grep -q 'countDocuments' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"count\":2}}'\n" +
		"elif grep -q '__taskdeckObjectAction' \"$file\" && grep -q 'deleteMany' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"affected_rows\":2,\"message\":\"Collection cleared\"}}'\n" +
		"elif grep -q '__taskdeckObjectAction' \"$file\" && grep -q '\\.drop()' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"message\":\"Collection dropped\"}}'\n" +
		"elif grep -q 'listDatabases' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"databases\":[{\"name\":\"admin\"},{\"name\":\"main\"}]}}'\n" +
		"elif grep -q 'getCollectionInfos({name:__name})' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"info\":[{\"name\":\"users\",\"type\":\"collection\"}],\"indexes\":[{\"name\":\"_id_\"}]}}'\n" +
		"elif grep -q 'getCollectionInfos();' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":[{\"name\":\"users\",\"type\":\"collection\"},{\"name\":\"active_users\",\"type\":\"view\"}]}'\n" +
		"elif grep -q '\\.find(' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"documents\":[{\"_id\":1,\"name\":\"Alice\"}],\"truncated\":false}}'\n" +
		"elif grep -q 'ping:1' \"$file\"; then\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":true,\"result\":{\"ok\":true}}'\n" +
		"else\n" +
		"  printf '%s\\n' '" + marker + "{\"ok\":false,\"error\":\"fixture operation not recognized\"}'\n" +
		"fi\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	currentPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+currentPath)
	return path
}

func mongoAdapterRequest(t *testing.T, id string, operation dbadapter.Operation, payload interface{}) dbadapter.Envelope {
	t.Helper()
	request, err := dbadapter.NewRequest(id, operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func connectMongoHandler(t *testing.T) *Handler {
	return connectMongoHandlerMode(t, true)
}

func connectMongoHandlerMode(t *testing.T, readOnly bool) *Handler {
	t.Helper()
	writeMongoshHandlerFixture(t)
	handler, err := NewHandler(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	_, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "connect-1", dbadapter.OpConnect, dbadapter.ConnectPayload{
		Host:     "db.example.com",
		Port:     27017,
		Username: "app",
		Secret:   "top-secret",
		Database: "main",
		ReadOnly: readOnly,
	}))
	if protocolErr != nil {
		t.Fatalf("connect error=%+v", protocolErr)
	}
	return handler
}

func TestHandlerConnectBrowseDescribeExecuteAndDisconnect(t *testing.T) {
	handler := connectMongoHandler(t)

	if !handler.connected || handler.client.Path == "" || handler.config.Password != "top-secret" {
		t.Fatalf("handler state=%+v config=%+v", handler, handler.config)
	}
	if _, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "ping-1", dbadapter.OpPing, nil)); protocolErr != nil {
		t.Fatalf("ping error=%+v", protocolErr)
	}

	catalogPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "catalog-1", dbadapter.OpListCatalogs, nil))
	if protocolErr != nil {
		t.Fatalf("catalog error=%+v", protocolErr)
	}
	catalogMap := catalogPayload.(map[string]interface{})
	catalogs := catalogMap["catalogs"].([]dbadapter.Object)
	if len(catalogs) != 2 || catalogs[1].Name != "main" {
		t.Fatalf("catalogs=%+v", catalogPayload)
	}

	objectsPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "objects-1", dbadapter.OpListObjects, dbadapter.ListObjectsPayload{Catalog: "main"}))
	if protocolErr != nil {
		t.Fatalf("objects error=%+v", protocolErr)
	}
	objectsMap := objectsPayload.(map[string]interface{})
	objects := objectsMap["objects"].([]dbadapter.Object)
	if len(objects) != 2 || objects[0].Name != "users" || objects[1].Kind != "view" {
		t.Fatalf("objects=%+v", objects)
	}

	detailPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "describe-1", dbadapter.OpDescribeObject, dbadapter.DescribeObjectPayload{Catalog: "main", Name: "users"}))
	if protocolErr != nil {
		t.Fatalf("describe error=%+v", protocolErr)
	}
	detail := detailPayload.(map[string]interface{})
	if detail["name"] != "users" || detail["catalog"] != "main" {
		t.Fatalf("detail=%+v", detail)
	}

	executePayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "execute-1", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "{\"op\":\"find\",\"collection\":\"users\",\"filter\":{\"active\":true}}",
		MaxRows:   10,
	}))
	if protocolErr != nil {
		t.Fatalf("execute error=%+v", protocolErr)
	}
	result := executePayload.(dbadapter.ExecuteResult)
	if len(result.Rows) != 1 {
		t.Fatalf("result=%+v", result)
	}
	document := result.Rows[0][0].(map[string]interface{})
	if document["name"] != "Alice" {
		t.Fatalf("document=%+v", document)
	}

	_, protocolErr = handler.Handle(context.Background(), mongoAdapterRequest(t, "execute-js", dbadapter.OpExecute, dbadapter.ExecutePayload{
		Statement: "{\"op\":\"find\",\"collection\":\"users\",\"filter\":{\"$where\":\"sleep(1000)\"}}",
		MaxRows:   10,
	}))
	if protocolErr == nil || protocolErr.Code != "INVALID_QUERY" {
		t.Fatalf("server-side JS was not rejected: %+v", protocolErr)
	}

	_, protocolErr = handler.Handle(context.Background(), mongoAdapterRequest(t, "disconnect-1", dbadapter.OpDisconnect, nil))
	if protocolErr != nil {
		t.Fatalf("disconnect error=%+v", protocolErr)
	}
	if handler.connected || handler.config.Password != "" {
		t.Fatalf("disconnect retained state: %+v", handler.config)
	}
}

func TestMongoOperationBodiesKeepNamesInsideStringBoundary(t *testing.T) {
	body, err := describeObjectOperationBody("main", "users\"; throw new Error('boom');//")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "users\"; throw new Error('boom');//") {
		t.Fatalf("raw collection name escaped script boundary: %s", body)
	}
	if !strings.Contains(body, "getCollectionInfos") {
		t.Fatalf("body=%s", body)
	}
}


func TestMongoWorkbenchBrowseUsesDocumentIdentity(t *testing.T) {
	handler := connectMongoHandler(t)
	defer handler.disconnect()

	payload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "browse-1", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main", Kind: "collection", Name: "users", Limit: 100,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	result, ok := payload.(dbadapter.BrowseRowsResult)
	if !ok || len(result.Rows) != 2 || len(result.Columns) != 1 || result.Columns[0].Type != "document" {
		t.Fatalf("browse result=%#v", payload)
	}
	if result.Editable || !strings.Contains(strings.ToLower(result.EditabilityReason), "read-only") {
		t.Fatalf("browse editability=%+v", result)
	}
	if result.Rows[0].Identity["_id"] == nil {
		t.Fatalf("missing MongoDB row identity: %+v", result.Rows[0])
	}
	document, ok := result.Rows[0].Values[0].(map[string]interface{})
	if !ok || document["name"] != "Alice" {
		t.Fatalf("document=%#v", result.Rows[0].Values[0])
	}
}

func TestMongoWorkbenchMutationsAndObjectActions(t *testing.T) {
	handler := connectMongoHandlerMode(t, false)
	defer handler.disconnect()

	browsePayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "browse-rw", dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{
		Catalog: "main", Kind: "collection", Name: "users", Limit: 100,
	}))
	if protocolErr != nil {
		t.Fatalf("browse error=%+v", protocolErr)
	}
	if !browsePayload.(dbadapter.BrowseRowsResult).Editable {
		t.Fatalf("write profile browse is not editable: %+v", browsePayload)
	}

	mutatePayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "mutate-rw", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main", Kind: "collection", Name: "users",
		Mutations: []dbadapter.RowMutation{
			{Action: "insert", Values: map[string]interface{}{"document": map[string]interface{}{"name": "Carol"}}},
			{Action: "update", Identity: map[string]interface{}{"_id": 1}, Values: map[string]interface{}{"document": map[string]interface{}{"_id": 1, "name": "Alicia"}}},
			{Action: "delete", Identity: map[string]interface{}{"_id": 2}},
		},
	}))
	if protocolErr != nil {
		t.Fatalf("mutate error=%+v", protocolErr)
	}
	mutations := mutatePayload.(dbadapter.MutateRowsResult)
	if len(mutations.Results) != 3 {
		t.Fatalf("mutation results=%+v", mutations)
	}
	for _, item := range mutations.Results {
		if item.Error != nil || item.AffectedRows != 1 {
			t.Fatalf("mutation item=%+v", item)
		}
	}

	countPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "count-1", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "collection", Name: "users", Action: "count_rows",
	}))
	if protocolErr != nil {
		t.Fatalf("count error=%+v", protocolErr)
	}
	count := countPayload.(dbadapter.ObjectActionResult)
	if count.Count == nil || *count.Count != 2 {
		t.Fatalf("count result=%+v", count)
	}

	truncatePayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "truncate-1", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "collection", Name: "users", Action: "truncate",
	}))
	if protocolErr != nil {
		t.Fatalf("truncate error=%+v", protocolErr)
	}
	truncate := truncatePayload.(dbadapter.ObjectActionResult)
	if truncate.AffectedRows != 2 {
		t.Fatalf("truncate result=%+v", truncate)
	}

	dropPayload, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "drop-1", dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{
		Catalog: "main", Kind: "collection", Name: "users", Action: "drop",
	}))
	if protocolErr != nil {
		t.Fatalf("drop error=%+v", protocolErr)
	}
	if !strings.Contains(strings.ToLower(dropPayload.(dbadapter.ObjectActionResult).Message), "dropped") {
		t.Fatalf("drop result=%+v", dropPayload)
	}
}

func TestMongoWorkbenchReadOnlyRejectsMutation(t *testing.T) {
	handler := connectMongoHandler(t)
	defer handler.disconnect()
	_, protocolErr := handler.Handle(context.Background(), mongoAdapterRequest(t, "mutate-ro", dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{
		Catalog: "main", Kind: "collection", Name: "users",
		Mutations: []dbadapter.RowMutation{{Action: "delete", Identity: map[string]interface{}{"_id": 1}}},
	}))
	if protocolErr == nil || protocolErr.Code != "READ_ONLY" {
		t.Fatalf("read-only mutation error=%+v", protocolErr)
	}
}

func TestMongoWorkbenchManifestCapabilities(t *testing.T) {
	manifest, err := BuiltinManifest("/opt/taskdeck")
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Capabilities.BrowseRows || !manifest.Capabilities.MutateRows || !manifest.Capabilities.ObjectActions {
		t.Fatalf("workbench capabilities=%+v", manifest.Capabilities)
	}
}
