import base64
import json
import pathlib
import signal
import sqlite3
import sys

MAX_ROWS = 1000
MAX_CELL_BYTES = 256 * 1024
connection = None


def emit(value):
    sys.stdout.write(json.dumps(value, ensure_ascii=False, separators=(",", ":")) + "\n")
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


def open_connection(config):
    path = str(config.get("file", "")).strip()
    if not path:
        raise ValueError("SQLite transaction file is required")
    read_only = bool(config.get("read_only", False))
    busy_timeout_ms = int(config.get("busy_timeout_ms", 5000))
    timeout_seconds = max(0.0, busy_timeout_ms / 1000.0)
    if read_only:
        uri = pathlib.Path(path).as_uri() + "?mode=ro"
        conn = sqlite3.connect(uri, uri=True, timeout=timeout_seconds)
        conn.execute("PRAGMA query_only=ON")
    else:
        conn = sqlite3.connect(path, timeout=timeout_seconds)
    conn.execute("PRAGMA busy_timeout=" + str(busy_timeout_ms))
    conn.execute("BEGIN")
    return conn


def execute(statement, max_rows):
    statement = str(statement or "").strip()
    if not statement:
        raise ValueError("SQLite statement is required")
    max_rows = int(max_rows or MAX_ROWS)
    if max_rows < 1 or max_rows > MAX_ROWS:
        raise ValueError("SQLite max_rows is outside the allowed range")
    cursor = connection.execute(statement)
    if cursor.description:
        columns = [{"name": item[0], "type": "sqlite"} for item in cursor.description]
        raw_rows = cursor.fetchmany(max_rows + 1)
        truncated = len(raw_rows) > max_rows
        rows, cell_truncated = normalize_rows(raw_rows[:max_rows])
        return {"columns": columns, "rows": rows, "affected_rows": 0, "truncated": bool(truncated or cell_truncated)}
    affected = cursor.rowcount if cursor.rowcount and cursor.rowcount > 0 else 0
    return {"columns": [], "rows": [], "affected_rows": affected, "truncated": False}


def interrupt_handler(_signum, _frame):
    if connection is not None:
        try:
            connection.interrupt()
        except Exception:
            pass


def main():
    global connection
    signal.signal(signal.SIGINT, interrupt_handler)
    first = sys.stdin.readline()
    if not first:
        raise RuntimeError("SQLite transaction worker did not receive configuration")
    config = json.loads(first)
    connection = open_connection(config)
    emit({"ok": True, "result": {"active": True, "message": "SQLite transaction started"}})

    for line in sys.stdin:
        if not line.strip():
            continue
        request = json.loads(line)
        operation = str(request.get("operation", "")).strip()
        payload = request.get("payload") or {}
        try:
            if operation == "execute":
                result = execute(payload.get("statement"), payload.get("max_rows", MAX_ROWS))
            elif operation == "commit":
                connection.commit()
                emit({"ok": True, "result": {"active": False, "message": "SQLite transaction committed"}})
                return
            elif operation == "rollback":
                connection.rollback()
                emit({"ok": True, "result": {"active": False, "message": "SQLite transaction rolled back"}})
                return
            else:
                raise ValueError("unsupported SQLite transaction operation: " + operation)
            emit({"ok": True, "result": result})
        except Exception as exc:
            message = str(exc)
            if len(message) > 4096:
                message = message[:4096]
            emit({"ok": False, "error": message})
    try:
        connection.rollback()
    except Exception:
        pass


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        message = str(exc)
        if len(message) > 4096:
            message = message[:4096]
        emit({"ok": False, "error": message})
        raise SystemExit(1)
