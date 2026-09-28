import base64
import json
import pathlib
import sqlite3
import sys

MAX_ROWS = 1000
MAX_OBJECTS = 5000
MAX_CELL_BYTES = 256 * 1024


def emit(value):
    sys.stdout.write(json.dumps(value, ensure_ascii=False, separators=(",", ":")))
    sys.stdout.write("\n")
    sys.stdout.flush()


def normalize_cell(value):
    if value is None or isinstance(value, (bool, int, float)):
        return value, False
    if isinstance(value, bytes):
        if len(value) > MAX_CELL_BYTES:
            return {"_taskdeck_truncated": True, "_taskdeck_bytes": len(value), "_taskdeck_type": "blob"}, True
        return {"$bytes_base64": base64.b64encode(value).decode("ascii")}, False
    if isinstance(value, str):
        encoded = value.encode("utf-8")
        if len(encoded) > MAX_CELL_BYTES:
            return {"_taskdeck_truncated": True, "_taskdeck_bytes": len(encoded), "_taskdeck_type": "text"}, True
        return value, False
    text = str(value)
    encoded = text.encode("utf-8")
    if len(encoded) > MAX_CELL_BYTES:
        return {"_taskdeck_truncated": True, "_taskdeck_bytes": len(encoded), "_taskdeck_type": "value"}, True
    return text, False


def normalize_rows(rows):
    output = []
    truncated = False
    for row in rows:
        normalized = []
        for value in row:
            cell, cell_truncated = normalize_cell(value)
            normalized.append(cell)
            truncated = truncated or cell_truncated
        output.append(normalized)
    return output, truncated


def open_connection(request):
    path = request["file"]
    busy_timeout_ms = int(request.get("busy_timeout_ms", 5000))
    read_only = bool(request.get("read_only", False))
    timeout_seconds = max(0.0, busy_timeout_ms / 1000.0)

    if read_only:
        uri = pathlib.Path(path).as_uri() + "?mode=ro"
        connection = sqlite3.connect(uri, uri=True, timeout=timeout_seconds)
        connection.execute("PRAGMA query_only=ON")
    else:
        connection = sqlite3.connect(path, timeout=timeout_seconds)
    connection.execute("PRAGMA busy_timeout=" + str(busy_timeout_ms))
    return connection


def operation_ping(connection, payload):
    row = connection.execute("SELECT 1").fetchone()
    if row is None or row[0] != 1:
        raise RuntimeError("SQLite ping returned an unexpected result")
    return {"ok": True}


def operation_list_objects(connection, payload):
    rows = connection.execute(
        """
        SELECT name, type
        FROM sqlite_master
        WHERE type IN ('table','view')
          AND name NOT LIKE 'sqlite_%'
        ORDER BY name
        LIMIT ?
        """,
        (MAX_OBJECTS + 1,),
    ).fetchall()
    truncated = len(rows) > MAX_OBJECTS
    rows = rows[:MAX_OBJECTS]
    objects = [
        {"kind": row[1], "name": row[0], "catalog": "main"}
        for row in rows
    ]
    return {"objects": objects, "truncated": truncated}


def operation_describe_object(connection, payload):
    name = str(payload.get("name", "")).strip()
    if not name:
        raise ValueError("SQLite object name is required")
    schema = connection.execute(
        "SELECT type, sql FROM sqlite_master WHERE name = ? AND type IN ('table','view') LIMIT 1",
        (name,),
    ).fetchone()
    columns = connection.execute(
        'SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?) ORDER BY cid',
        (name,),
    ).fetchall()
    indexes = connection.execute(
        'SELECT seq, name, "unique", origin, partial FROM pragma_index_list(?) ORDER BY seq',
        (name,),
    ).fetchall()
    return {
        "kind": schema[0] if schema else "object",
        "name": name,
        "catalog": "main",
        "sql": schema[1] if schema else None,
        "columns": [
            {
                "cid": row[0],
                "name": row[1],
                "type": row[2],
                "not_null": bool(row[3]),
                "default": row[4],
                "primary_key": bool(row[5]),
            }
            for row in columns
        ],
        "indexes": [
            {
                "seq": row[0],
                "name": row[1],
                "unique": bool(row[2]),
                "origin": row[3],
                "partial": bool(row[4]),
            }
            for row in indexes
        ],
    }


def quote_identifier(value):
    text = str(value or "").strip()
    if not text:
        raise ValueError("SQLite identifier is required")
    if len(text.encode("utf-8")) > 512 or "\x00" in text or "\r" in text or "\n" in text:
        raise ValueError("SQLite identifier is invalid")
    return '"' + text.replace('"', '""') + '"'


def decode_input_cell(value):
    if isinstance(value, dict) and set(value.keys()) == {"$bytes_base64"}:
        raw = value.get("$bytes_base64")
        if not isinstance(raw, str):
            raise ValueError("SQLite binary value must be base64 text")
        try:
            return base64.b64decode(raw, validate=True)
        except Exception as exc:
            raise ValueError("SQLite binary value is not valid base64") from exc
    if value is None or isinstance(value, (bool, int, float, str, bytes)):
        return value
    raise ValueError("SQLite grid values must be scalar")


def object_metadata(connection, name):
    schema = connection.execute(
        "SELECT type, sql FROM sqlite_master WHERE name = ? AND type IN ('table','view') LIMIT 1",
        (name,),
    ).fetchone()
    if schema is None:
        raise ValueError("SQLite table/view does not exist")
    columns = connection.execute(
        'SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?) ORDER BY cid',
        (name,),
    ).fetchall()
    primary = [
        (int(row[5]), row[1])
        for row in columns
        if int(row[5] or 0) > 0
    ]
    primary.sort(key=lambda item: item[0])
    primary_names = [item[1] for item in primary]

    rowid_alias = None
    sql = str(schema[1] or "")
    if schema[0] == "table" and not primary_names and "WITHOUT ROWID" not in sql.upper():
        visible_names = {str(row[1]).lower() for row in columns}
        for candidate in ("rowid", "_rowid_", "oid"):
            if candidate.lower() not in visible_names:
                rowid_alias = candidate
                break
    return {
        "kind": schema[0],
        "sql": schema[1],
        "columns": columns,
        "primary": primary_names,
        "rowid_alias": rowid_alias,
    }


def build_filters(metadata, filters):
    columns = {str(row[1]).lower(): str(row[1]) for row in metadata["columns"]}
    clauses = []
    params = []
    for item in filters or []:
        column = columns.get(str(item.get("column", "")).strip().lower())
        if not column:
            raise ValueError("SQLite filter references an unknown column")
        operator = str(item.get("operator", "")).strip().lower()
        quoted = quote_identifier(column)
        if operator == "is_null":
            clauses.append(quoted + " IS NULL")
            continue
        if operator == "not_null":
            clauses.append(quoted + " IS NOT NULL")
            continue
        value = decode_input_cell(item.get("value"))
        if value is None:
            if operator == "eq":
                clauses.append(quoted + " IS NULL")
                continue
            if operator == "ne":
                clauses.append(quoted + " IS NOT NULL")
                continue
            raise ValueError("SQLite NULL filters support only eq/ne/is_null/not_null")
        if operator == "eq":
            clauses.append(quoted + " = ?")
        elif operator == "ne":
            clauses.append(quoted + " <> ?")
        elif operator == "lt":
            clauses.append(quoted + " < ?")
        elif operator == "lte":
            clauses.append(quoted + " <= ?")
        elif operator == "gt":
            clauses.append(quoted + " > ?")
        elif operator == "gte":
            clauses.append(quoted + " >= ?")
        elif operator == "contains":
            clauses.append("CAST(" + quoted + " AS TEXT) LIKE '%' || ? || '%'")
        elif operator == "starts_with":
            clauses.append("CAST(" + quoted + " AS TEXT) LIKE ? || '%'")
        else:
            raise ValueError("unsupported SQLite filter operator")
        params.append(value)
    return clauses, params


def operation_browse_rows(connection, payload, read_only):
    name = str(payload.get("name", "")).strip()
    if not name:
        raise ValueError("SQLite object name is required")
    offset = int(payload.get("offset", 0))
    limit = int(payload.get("limit", 100))
    if offset < 0:
        raise ValueError("SQLite row offset must not be negative")
    if limit < 1 or limit > MAX_ROWS:
        raise ValueError("SQLite row limit is outside the allowed range")

    metadata = object_metadata(connection, name)
    columns = metadata["columns"]
    by_name = {str(row[1]).lower(): row for row in columns}
    clauses, params = build_filters(metadata, payload.get("filters") or [])

    order_parts = []
    for item in payload.get("sort") or []:
        column = by_name.get(str(item.get("column", "")).strip().lower())
        if column is None:
            raise ValueError("SQLite sort references an unknown column")
        direction = str(item.get("direction", "")).strip().lower()
        if direction not in ("asc", "desc"):
            raise ValueError("SQLite sort direction is invalid")
        order_parts.append(quote_identifier(column[1]) + " " + direction.upper())
    if not order_parts:
        if metadata["primary"]:
            order_parts = [quote_identifier(value) + " ASC" for value in metadata["primary"]]
        elif metadata["rowid_alias"]:
            order_parts = [quote_identifier(metadata["rowid_alias"]) + " ASC"]

    fetch_limit = limit + 1
    select_prefix = "SELECT "
    if metadata["rowid_alias"]:
        select_prefix += quote_identifier(metadata["rowid_alias"]) + ' AS "__taskdeck_rowid__", '
    select_prefix += "* FROM " + quote_identifier(name)
    if clauses:
        select_prefix += " WHERE " + " AND ".join(clauses)
    if order_parts:
        select_prefix += " ORDER BY " + ", ".join(order_parts)
    select_prefix += " LIMIT ? OFFSET ?"
    cursor = connection.execute(select_prefix, tuple(params + [fetch_limit, offset]))
    raw_rows = cursor.fetchall()
    has_more = len(raw_rows) > limit
    raw_rows = raw_rows[:limit]

    visible_columns = columns
    result_columns = []
    primary_set = {name.lower() for name in metadata["primary"]}
    editable = bool(not read_only and metadata["kind"] == "table" and (metadata["primary"] or metadata["rowid_alias"]))
    reason = ""
    if read_only:
        reason = "Connection is read-only"
    elif metadata["kind"] != "table":
        reason = "Views are opened read-only"
    elif not metadata["primary"] and not metadata["rowid_alias"]:
        reason = "No stable primary key or rowid is available"

    for row in visible_columns:
        column_name = str(row[1])
        result_columns.append({
            "name": column_name,
            "type": str(row[2] or "sqlite"),
            "nullable": not bool(row[3] or row[5]),
            "editable": editable,
            "identity": column_name.lower() in primary_set,
        })

    output_rows = []
    cell_truncated = False
    for raw_row in raw_rows:
        internal_rowid = None
        values = list(raw_row)
        if metadata["rowid_alias"]:
            internal_rowid = values.pop(0)
        normalized_values = []
        for value in values:
            cell, truncated = normalize_cell(value)
            normalized_values.append(cell)
            cell_truncated = cell_truncated or truncated

        identity = {}
        if metadata["primary"]:
            positions = {str(row[1]).lower(): index for index, row in enumerate(visible_columns)}
            for key in metadata["primary"]:
                raw_value = values[positions[key.lower()]]
                cell, truncated = normalize_cell(raw_value)
                if truncated:
                    editable = False
                    reason = "Stable row identity exceeds the safe cell limit"
                identity[key] = cell
        elif metadata["rowid_alias"]:
            identity["__rowid__"] = internal_rowid
        output_rows.append({"values": normalized_values, "identity": identity})

    if not editable:
        for column in result_columns:
            column["editable"] = False

    return {
        "columns": result_columns,
        "rows": output_rows,
        "offset": offset,
        "limit": limit,
        "has_more": has_more,
        "editable": editable,
        "editability_reason": reason,
        "truncated": cell_truncated,
    }


def mutation_identity(metadata, identity):
    if metadata["primary"]:
        expected = metadata["primary"]
        lowered = {str(key).lower(): value for key, value in (identity or {}).items()}
        if len(lowered) != len(expected):
            raise ValueError("SQLite row identity does not match the primary key")
        clauses = []
        params = []
        for name in expected:
            key = name.lower()
            if key not in lowered:
                raise ValueError("SQLite primary-key identity is incomplete")
            clauses.append(quote_identifier(name) + " IS ?")
            params.append(decode_input_cell(lowered[key]))
        return " AND ".join(clauses), params
    if metadata["rowid_alias"]:
        identity = identity or {}
        if set(identity.keys()) != {"__rowid__"}:
            raise ValueError("SQLite rowid identity is missing")
        return quote_identifier(metadata["rowid_alias"]) + " = ?", [decode_input_cell(identity["__rowid__"])]
    raise ValueError("SQLite stable row identity is unavailable")


def canonical_values(metadata, values):
    columns = {str(row[1]).lower(): str(row[1]) for row in metadata["columns"]}
    output = []
    for raw_name, raw_value in (values or {}).items():
        name = columns.get(str(raw_name).strip().lower())
        if not name:
            raise ValueError("SQLite mutation references an unknown column")
        output.append((name, decode_input_cell(raw_value)))
    output.sort(key=lambda item: item[0].lower())
    return output


def operation_mutate_rows(connection, payload, read_only):
    if read_only:
        raise ValueError("SQLite connection is read-only")
    name = str(payload.get("name", "")).strip()
    if not name:
        raise ValueError("SQLite object name is required")
    metadata = object_metadata(connection, name)
    if metadata["kind"] != "table":
        raise ValueError("SQLite views are not editable")
    table = quote_identifier(name)
    results = []

    for index, mutation in enumerate(payload.get("mutations") or []):
        action = str(mutation.get("action", "")).strip().lower()
        item = {"index": index, "action": action, "affected_rows": 0}
        try:
            if action == "insert":
                values = canonical_values(metadata, mutation.get("values") or {})
                if not values:
                    raise ValueError("SQLite insert requires values")
                columns = ", ".join(quote_identifier(name) for name, _ in values)
                placeholders = ", ".join("?" for _ in values)
                cursor = connection.execute(
                    "INSERT INTO " + table + " (" + columns + ") VALUES (" + placeholders + ")",
                    tuple(value for _, value in values),
                )
            elif action == "update":
                values = canonical_values(metadata, mutation.get("values") or {})
                if not values:
                    raise ValueError("SQLite update requires changed values")
                where, identity_params = mutation_identity(metadata, mutation.get("identity") or {})
                assignments = ", ".join(quote_identifier(name) + " = ?" for name, _ in values)
                cursor = connection.execute(
                    "UPDATE " + table + " SET " + assignments + " WHERE " + where,
                    tuple([value for _, value in values] + identity_params),
                )
            elif action == "delete":
                where, identity_params = mutation_identity(metadata, mutation.get("identity") or {})
                cursor = connection.execute(
                    "DELETE FROM " + table + " WHERE " + where,
                    tuple(identity_params),
                )
            else:
                raise ValueError("unsupported SQLite mutation action")

            affected = cursor.rowcount if cursor.rowcount and cursor.rowcount > 0 else 0
            item["affected_rows"] = affected
            if affected != 1:
                item["error"] = {
                    "code": "ROW_NOT_CHANGED",
                    "message": "SQLite " + action + " affected " + str(affected) + " rows; expected exactly 1",
                }
        except Exception as exc:
            message = str(exc)
            if len(message) > 4096:
                message = message[:4096]
            item["error"] = {"code": "MUTATION_FAILED", "message": message}
        results.append(item)

    connection.commit()
    return {"results": results}


def operation_object_action(connection, payload, read_only):
    name = str(payload.get("name", "")).strip()
    action = str(payload.get("action", "")).strip().lower()
    if not name:
        raise ValueError("SQLite object name is required")
    metadata = object_metadata(connection, name)
    target = quote_identifier(name)

    if action == "count_rows":
        row = connection.execute("SELECT COUNT(*) FROM " + target).fetchone()
        return {"count": int(row[0] if row else 0)}
    if read_only:
        raise ValueError("SQLite connection is read-only")
    if action == "truncate":
        if metadata["kind"] != "table":
            raise ValueError("SQLite views cannot be truncated")
        cursor = connection.execute("DELETE FROM " + target)
        connection.commit()
        affected = cursor.rowcount if cursor.rowcount and cursor.rowcount > 0 else 0
        return {"affected_rows": affected, "message": "Table rows deleted"}
    if action == "drop":
        statement = "DROP VIEW " if metadata["kind"] == "view" else "DROP TABLE "
        connection.execute(statement + target)
        connection.commit()
        return {"message": "Object dropped"}
    raise ValueError("unsupported SQLite object action")


def operation_import_sql(connection, payload, read_only):
    if read_only:
        raise ValueError("SQLite connection is read-only")
    path_text = str(payload.get("path", "")).strip()
    if not path_text:
        raise ValueError("SQLite import path is required")
    path = pathlib.Path(path_text)
    if not path.is_file():
        raise ValueError("SQLite import path is not a regular file")
    imported_bytes = int(path.stat().st_size)
    pending = ""
    max_pending = 32 * 1024 * 1024

    def execute_complete(statement):
        text = statement.strip()
        if not text:
            return
        connection.execute(statement)

    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        for line in handle:
            start = 0
            for index, char in enumerate(line):
                if char != ";":
                    continue
                pending += line[start:index + 1]
                start = index + 1
                if sqlite3.complete_statement(pending):
                    execute_complete(pending)
                    pending = ""
                elif len(pending.encode("utf-8")) > max_pending:
                    raise ValueError("SQLite import contains a statement larger than 32 MiB")
            pending += line[start:]
            if len(pending.encode("utf-8")) > max_pending:
                raise ValueError("SQLite import contains a statement larger than 32 MiB")

    if pending.strip():
        if sqlite3.complete_statement(pending):
            execute_complete(pending)
        else:
            # A trailing comment is harmless; executescript also raises for
            # genuinely incomplete SQL so malformed imports still fail closed.
            connection.executescript(pending)
    connection.commit()
    return {"imported_bytes": imported_bytes, "message": "SQL import completed"}


def operation_execute(connection, payload, read_only):
    statement = str(payload.get("statement", "")).strip()
    if not statement:
        raise ValueError("SQLite statement is required")
    max_rows = int(payload.get("max_rows", MAX_ROWS))
    if max_rows < 1 or max_rows > MAX_ROWS:
        raise ValueError("SQLite max_rows is outside the allowed range")

    cursor = connection.execute(statement)
    if cursor.description:
        columns = [
            {"name": item[0], "type": "sqlite"}
            for item in cursor.description
        ]
        raw_rows = cursor.fetchmany(max_rows + 1)
        truncated = len(raw_rows) > max_rows
        raw_rows = raw_rows[:max_rows]
        rows, cell_truncated = normalize_rows(raw_rows)
        return {
            "columns": columns,
            "rows": rows,
            "affected_rows": 0,
            "truncated": bool(truncated or cell_truncated),
        }

    affected_rows = cursor.rowcount if cursor.rowcount and cursor.rowcount > 0 else 0
    if not read_only:
        connection.commit()
    return {
        "columns": [],
        "rows": [],
        "affected_rows": affected_rows,
        "truncated": False,
    }


def main():
    request = json.load(sys.stdin)
    operation = str(request.get("operation", "")).strip()
    payload = request.get("payload") or {}
    connection = open_connection(request)
    try:
        if operation in ("connect", "ping"):
            result = operation_ping(connection, payload)
        elif operation == "list_objects":
            result = operation_list_objects(connection, payload)
        elif operation == "describe_object":
            result = operation_describe_object(connection, payload)
        elif operation == "browse_rows":
            result = operation_browse_rows(connection, payload, bool(request.get("read_only", False)))
        elif operation == "mutate_rows":
            result = operation_mutate_rows(connection, payload, bool(request.get("read_only", False)))
        elif operation == "object_action":
            result = operation_object_action(connection, payload, bool(request.get("read_only", False)))
        elif operation == "execute":
            result = operation_execute(connection, payload, bool(request.get("read_only", False)))
        elif operation == "import_sql":
            result = operation_import_sql(connection, payload, bool(request.get("read_only", False)))
        else:
            raise ValueError("unsupported SQLite helper operation: " + operation)
        emit({"ok": True, "result": result})
    except Exception as exc:
        try:
            connection.rollback()
        except Exception:
            pass
        message = str(exc)
        if len(message) > 4096:
            message = message[:4096]
        emit({"ok": False, "error": message})
    finally:
        connection.close()


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        message = str(exc)
        if len(message) > 4096:
            message = message[:4096]
        emit({"ok": False, "error": message})
