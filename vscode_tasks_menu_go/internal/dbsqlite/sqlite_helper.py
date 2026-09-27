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
        elif operation == "execute":
            result = operation_execute(connection, payload, bool(request.get("read_only", False)))
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
