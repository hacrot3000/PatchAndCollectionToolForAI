package dbmongo

import (
	"encoding/json"
	"fmt"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
)

func mongoBrowseRowsOperationBody(database string, payload dbadapter.BrowseRowsPayload) (string, error) {
	if len(payload.Filters) != 0 {
		return "", fmt.Errorf("MongoDB Workbench filters are not enabled yet; use the Query tab for filtered finds")
	}
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	collectionLiteral, err := javascriptString(payload.Name)
	if err != nil {
		return "", err
	}
	direction := 1
	if len(payload.Sort) != 0 {
		if len(payload.Sort) != 1 || !strings.EqualFold(payload.Sort[0].Column, "document") {
			return "", fmt.Errorf("MongoDB Workbench supports only document/_id page sorting")
		}
		if payload.Sort[0].Direction == "desc" {
			direction = -1
		}
	}
	fetchLimit := payload.Limit + 1
	if fetchLimit > dbadapter.MaxRows+1 {
		fetchLimit = dbadapter.MaxRows + 1
	}
	return "    // __taskdeckBrowseRows\n" +
		"    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    let __cursor = __db.getCollection(" + collectionLiteral + ").find({});\n" +
		"    __cursor = __cursor.maxTimeMS(" + fmt.Sprint(maxFindTimeMS) + ").sort({_id:" + fmt.Sprint(direction) + "}).skip(" + fmt.Sprint(payload.Offset) + ").limit(" + fmt.Sprint(fetchLimit) + ");\n" +
		"    const __docs = __cursor.toArray();\n" +
		"    return {documents:__docs.slice(0," + fmt.Sprint(payload.Limit) + "),has_more:__docs.length>" + fmt.Sprint(payload.Limit) + "};", nil
}

func mongoDocumentsToBrowseResult(documents []interface{}, payload dbadapter.BrowseRowsPayload, readOnly bool) (dbadapter.BrowseRowsResult, error) {
	kind := strings.ToLower(strings.TrimSpace(payload.Kind))
	editable := !readOnly && kind != "view"
	reason := ""
	if readOnly {
		reason = "Connection is read-only"
	} else if kind == "view" {
		reason = "MongoDB views are opened read-only"
	}
	rows := make([]dbadapter.BrowseRow, 0, len(documents))
	for _, document := range documents {
		raw, err := json.Marshal(document)
		if err != nil {
			return dbadapter.BrowseRowsResult{}, fmt.Errorf("encode MongoDB document: %w", err)
		}
		cell := document
		if len(raw) > dbadapter.MaxCellBytes {
			cell = map[string]interface{}{
				"_taskdeck_truncated": true,
				"_taskdeck_bytes":     len(raw),
				"_taskdeck_type":      "document",
			}
			editable = false
			reason = "One or more documents exceed the safe editable cell limit"
		}
		identity := map[string]interface{}{}
		if object, ok := document.(map[string]interface{}); ok {
			if id, exists := object["_id"]; exists {
				identity["_id"] = id
			} else {
				editable = false
				if reason == "" {
					reason = "MongoDB document has no _id identity"
				}
			}
		} else {
			editable = false
			if reason == "" {
				reason = "MongoDB browse result is not a document"
			}
		}
		rows = append(rows, dbadapter.BrowseRow{Values: []interface{}{cell}, Identity: identity})
	}
	result := dbadapter.BrowseRowsResult{
		Columns: []dbadapter.BrowseColumn{{
			Name: "document", Type: "document", Nullable: false, Editable: editable,
		}},
		Rows: rows, Offset: payload.Offset, Limit: payload.Limit, Editable: editable, EditabilityReason: reason,
	}
	return result, nil
}

func validateMongoMutations(payload dbadapter.MutateRowsPayload) error {
	for i, mutation := range payload.Mutations {
		switch mutation.Action {
		case "insert":
			document, ok := mutation.Values["document"]
			if !ok || len(mutation.Values) != 1 {
				return fmt.Errorf("mutation %d MongoDB insert requires exactly one document value", i+1)
			}
			if _, ok := document.(map[string]interface{}); !ok {
				return fmt.Errorf("mutation %d MongoDB document must be a JSON object", i+1)
			}
		case "update":
			if len(mutation.Identity) != 1 {
				return fmt.Errorf("mutation %d MongoDB update requires exactly one _id identity", i+1)
			}
			if _, ok := mutation.Identity["_id"]; !ok {
				return fmt.Errorf("mutation %d MongoDB update requires _id identity", i+1)
			}
			document, ok := mutation.Values["document"]
			if !ok || len(mutation.Values) != 1 {
				return fmt.Errorf("mutation %d MongoDB update requires exactly one document value", i+1)
			}
			if _, ok := document.(map[string]interface{}); !ok {
				return fmt.Errorf("mutation %d MongoDB document must be a JSON object", i+1)
			}
		case "delete":
			if len(mutation.Identity) != 1 {
				return fmt.Errorf("mutation %d MongoDB delete requires exactly one _id identity", i+1)
			}
			if _, ok := mutation.Identity["_id"]; !ok {
				return fmt.Errorf("mutation %d MongoDB delete requires _id identity", i+1)
			}
		default:
			return fmt.Errorf("mutation %d has unsupported MongoDB action %q", i+1, mutation.Action)
		}
	}
	return nil
}

func javascriptEJSON(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode MongoDB EJSON payload: %w", err)
	}
	literal, err := javascriptString(string(data))
	if err != nil {
		return "", err
	}
	return "EJSON.parse(" + literal + ")", nil
}

func mongoMutateRowsOperationBody(database string, payload dbadapter.MutateRowsPayload) (string, error) {
	if err := validateMongoMutations(payload); err != nil {
		return "", err
	}
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	collectionLiteral, err := javascriptString(payload.Name)
	if err != nil {
		return "", err
	}
	mutationsLiteral, err := javascriptEJSON(payload.Mutations)
	if err != nil {
		return "", err
	}
	return "    // __taskdeckMutateRows\n" +
		"    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    const __collection = __db.getCollection(" + collectionLiteral + ");\n" +
		"    const __mutations = " + mutationsLiteral + ";\n" +
		"    const __results = [];\n" +
		"    for (let __i=0; __i<__mutations.length; __i++) {\n" +
		"      const __m = __mutations[__i];\n" +
		"      const __item = {index:__i,action:__m.action,affected_rows:0};\n" +
		"      try {\n" +
		"        let __affected = 0;\n" +
		"        if (__m.action === 'insert') {\n" +
		"          const __doc = __m.values.document;\n" +
		"          const __r = __collection.insertOne(__doc);\n" +
		"          __affected = __r && __r.acknowledged !== false ? 1 : 0;\n" +
		"        } else if (__m.action === 'update') {\n" +
		"          const __id = __m.identity._id;\n" +
		"          const __doc = __m.values.document;\n" +
		"          if (Object.prototype.hasOwnProperty.call(__doc,'_id') && EJSON.stringify(__doc._id) !== EJSON.stringify(__id)) throw new Error('MongoDB _id cannot be changed');\n" +
		"          __doc._id = __id;\n" +
		"          const __r = __collection.replaceOne({_id:__id},__doc);\n" +
		"          __affected = Number(__r.matchedCount || 0);\n" +
		"        } else if (__m.action === 'delete') {\n" +
		"          const __r = __collection.deleteOne({_id:__m.identity._id});\n" +
		"          __affected = Number(__r.deletedCount || 0);\n" +
		"        } else { throw new Error('Unsupported MongoDB mutation'); }\n" +
		"        __item.affected_rows = __affected;\n" +
		"        if (__affected !== 1) __item.error = {code:'ROW_NOT_CHANGED',message:'MongoDB '+__m.action+' affected '+__affected+' documents; expected exactly 1'};\n" +
		"      } catch (__error) { __item.error = {code:'MUTATION_FAILED',message:String(__error).slice(0,4096)}; }\n" +
		"      __results.push(__item);\n" +
		"    }\n" +
		"    return {results:__results};", nil
}

func mongoObjectActionOperationBody(database string, payload dbadapter.ObjectActionPayload) (string, error) {
	databaseLiteral, err := javascriptString(database)
	if err != nil {
		return "", err
	}
	collectionLiteral, err := javascriptString(payload.Name)
	if err != nil {
		return "", err
	}
	prefix := "    // __taskdeckObjectAction\n" +
		"    const __db = __taskdeckDb.getSiblingDB(" + databaseLiteral + ");\n" +
		"    const __collection = __db.getCollection(" + collectionLiteral + ");\n"
	switch payload.Action {
	case "count_rows":
		return prefix + "    const __count = __collection.countDocuments({}, {maxTimeMS:" + fmt.Sprint(maxFindTimeMS) + "});\n    return {count:Number(__count)};", nil
	case "truncate":
		return prefix + "    const __r = __collection.deleteMany({});\n    return {affected_rows:Number(__r.deletedCount || 0),message:'Collection cleared'};", nil
	case "drop":
		return prefix + "    const __ok = __collection.drop();\n    return {message:__ok?'Collection dropped':'Collection drop returned false'};", nil
	default:
		return "", fmt.Errorf("unsupported MongoDB object action %q", payload.Action)
	}
}
