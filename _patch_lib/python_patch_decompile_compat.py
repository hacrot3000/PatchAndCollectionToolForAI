#!/usr/bin/env python3
"""Read-only compatibility extractor for historical IDA/Ghidra COLLECT actions.

This module deliberately has no project mutation surface.  It indexes one
project-contained decompile text file in a temporary SQLite database, extracts
functions by address/name/regex, optional neighbouring functions and bounded
text-reference contexts, then deletes the temporary index.
"""
from __future__ import annotations

import mmap
import os
import re
import sqlite3
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

VERSION = "6.19.4"
MARKER_RE = re.compile(
    rb"^//----- \((?:0[xX])?([0-9A-Fa-f]+)\) -+\r?$",
    re.MULTILINE,
)
SYMBOL_TOKEN_RE = re.compile(r"([A-Za-z_~][A-Za-z0-9_:~<>]*)\s*\(")


class DecompileCompatError(ValueError):
    pass


@dataclass(frozen=True)
class FunctionRecord:
    row_id: int
    address: int
    start_offset: int
    end_offset: int
    marker_line: str
    signature: str
    symbol: str
    preview: str

    @property
    def address_hex(self) -> str:
        return f"0x{self.address:X}"


def _decode(data: bytes) -> str:
    return data.decode("utf-8", errors="replace")


def _extract_signature_and_symbol(block_head: bytes) -> tuple[str, str, str]:
    text = _decode(block_head)
    lines = text.splitlines()
    signature_lines: list[str] = []
    saw_code = False
    brace_seen = False
    for line in lines[1:]:
        stripped = line.strip()
        if not stripped:
            if saw_code and not brace_seen:
                signature_lines.append("")
            continue
        if stripped.startswith("//"):
            if not saw_code:
                continue
            if brace_seen:
                break
        saw_code = True
        signature_lines.append(line.rstrip())
        if "{" in line:
            brace_seen = True
            break
        if stripped.endswith(";"):
            break
        if len(signature_lines) >= 24:
            break
    signature = "\n".join(signature_lines).strip()
    compact = " ".join(part.strip() for part in signature.splitlines() if part.strip())
    symbol = ""
    matches = list(SYMBOL_TOKEN_RE.finditer(compact))
    if matches:
        symbol = matches[-1].group(1)
    preview = "\n".join(lines[:40])
    return signature, symbol, preview


def _normalize_address(value: int | str) -> int:
    if isinstance(value, int):
        if value < 0:
            raise DecompileCompatError("address must be non-negative")
        return value
    text = str(value).strip()
    try:
        return int(text, 16)
    except ValueError as exc:
        raise DecompileCompatError(f"invalid hexadecimal address: {value!r}") from exc


def _row_to_record(row) -> FunctionRecord:
    return FunctionRecord(
        row_id=int(row[0]),
        address=int(row[1]),
        start_offset=int(row[2]),
        end_offset=int(row[3]),
        marker_line=str(row[4]),
        signature=str(row[5]),
        symbol=str(row[6]),
        preview=str(row[7]),
    )


def _insert_record(conn: sqlite3.Connection, values: tuple) -> None:
    conn.execute(
        """
        INSERT INTO functions(
            address,start_offset,end_offset,marker_line,signature,symbol,preview
        ) VALUES(?,?,?,?,?,?,?)
        """,
        values,
    )


def _index_mmap(conn: sqlite3.Connection, mm: mmap.mmap) -> int:
    conn.executescript(
        """
        CREATE TABLE functions(
            id INTEGER PRIMARY KEY,
            address INTEGER NOT NULL,
            start_offset INTEGER NOT NULL,
            end_offset INTEGER NOT NULL,
            marker_line TEXT NOT NULL,
            signature TEXT NOT NULL,
            symbol TEXT NOT NULL,
            preview TEXT NOT NULL
        );
        CREATE INDEX idx_functions_address ON functions(address);
        CREATE INDEX idx_functions_symbol ON functions(symbol);
        """
    )
    previous: tuple[int, int, int, str] | None = None
    count = 0
    for match in MARKER_RE.finditer(mm):
        address = int(match.group(1), 16)
        start = match.start()
        line_end = mm.find(b"\n", match.end())
        marker_end = match.end() if line_end < 0 else line_end + 1
        marker_line = _decode(mm[match.start():match.end()])
        if previous is not None:
            p_address, p_start, p_marker_end, p_marker_line = previous
            head_end = min(start, p_marker_end + 16_384)
            signature, symbol, preview = _extract_signature_and_symbol(mm[p_start:head_end])
            _insert_record(
                conn,
                (p_address, p_start, start, p_marker_line, signature, symbol, preview),
            )
            count += 1
        previous = (address, start, marker_end, marker_line)
    if previous is not None:
        p_address, p_start, p_marker_end, p_marker_line = previous
        head_end = min(len(mm), p_marker_end + 16_384)
        signature, symbol, preview = _extract_signature_and_symbol(mm[p_start:head_end])
        _insert_record(
            conn,
            (p_address, p_start, len(mm), p_marker_line, signature, symbol, preview),
        )
        count += 1
    conn.commit()
    return count


def _select_direct(conn: sqlite3.Connection, action: dict) -> list[FunctionRecord]:
    base_sql = (
        "SELECT id,address,start_offset,end_offset,marker_line,signature,symbol,preview "
        "FROM functions"
    )
    if action.get("address") is not None:
        rows = conn.execute(
            base_sql + " WHERE address=? ORDER BY id",
            (_normalize_address(action["address"]),),
        ).fetchall()
        return [_row_to_record(row) for row in rows]

    name = str(action.get("name") or action.get("symbol") or "")
    mode = action.get("match", "contains")
    case_sensitive = bool(action.get("case_sensitive", True))
    max_matches = int(action.get("max_matches", 20))
    try:
        regex = re.compile(name, 0 if case_sensitive else re.I) if mode == "regex" else None
    except re.error as exc:
        raise DecompileCompatError(f"invalid decompile name regex: {exc}") from exc
    needle = name if case_sensitive else name.casefold()
    out: list[FunctionRecord] = []
    for row in conn.execute(base_sql + " ORDER BY id"):
        rec = _row_to_record(row)
        haystack = "\n".join((rec.symbol, rec.signature, rec.preview))
        compare = haystack if case_sensitive else haystack.casefold()
        if mode == "exact":
            symbol = rec.symbol if case_sensitive else rec.symbol.casefold()
            matched = symbol == needle
        elif mode == "contains":
            matched = needle in compare
        else:
            assert regex is not None
            matched = bool(regex.search(haystack))
        if matched:
            out.append(rec)
            if len(out) >= max_matches:
                break
    return out


def _expand_neighbors(
    conn: sqlite3.Connection,
    direct: Iterable[FunctionRecord],
    before: int,
    after: int,
) -> list[FunctionRecord]:
    result: list[FunctionRecord] = []
    seen: set[int] = set()
    for rec in direct:
        rows = conn.execute(
            """
            SELECT id,address,start_offset,end_offset,marker_line,signature,symbol,preview
            FROM functions WHERE id BETWEEN ? AND ? ORDER BY id
            """,
            (max(1, rec.row_id - before), rec.row_id + after),
        ).fetchall()
        for row in rows:
            item = _row_to_record(row)
            if item.row_id not in seen:
                seen.add(item.row_id)
                result.append(item)
    return result


def _line_bounds(mm: mmap.mmap, position: int, context_lines: int) -> tuple[int, int]:
    start = position
    for _ in range(context_lines):
        previous = mm.rfind(b"\n", 0, max(0, start - 1))
        if previous < 0:
            start = 0
            break
        start = previous
    if start > 0 and mm[start:start + 1] == b"\n":
        start += 1
    end = position
    for _ in range(context_lines + 1):
        next_nl = mm.find(b"\n", end)
        if next_nl < 0:
            end = len(mm)
            break
        end = next_nl + 1
    return start, end


def _text_references(
    mm: mmap.mmap,
    term: str,
    *,
    case_sensitive: bool,
    context_lines: int,
    max_hits: int,
) -> list[tuple[int, str]]:
    needle = term.encode("utf-8")
    if not needle or max_hits <= 0:
        return []
    positions: list[int] = []
    if case_sensitive:
        cursor = 0
        while len(positions) < max_hits:
            pos = mm.find(needle, cursor)
            if pos < 0:
                break
            positions.append(pos)
            cursor = pos + max(1, len(needle))
    else:
        pattern = re.compile(re.escape(needle), re.I)
        for match in pattern.finditer(mm):
            positions.append(match.start())
            if len(positions) >= max_hits:
                break
    out: list[tuple[int, str]] = []
    for pos in positions:
        start, end = _line_bounds(mm, pos, context_lines)
        line_no = mm[:pos].count(b"\n") + 1
        out.append((line_no, _decode(mm[start:end]).rstrip()))
    return out


def extract_decompile_report(source: Path, action: dict, *, max_file_bytes: int) -> str:
    st = source.stat()
    if st.st_size > max_file_bytes:
        raise DecompileCompatError(
            f"decompile source exceeds max_decompile_file_bytes: {source.name} ({st.st_size}>{max_file_bytes})"
        )
    if st.st_size == 0:
        raise DecompileCompatError("decompile source is empty")
    if source.is_symlink() or not source.is_file():
        raise DecompileCompatError("decompile source must be a regular non-symlink file")

    with source.open("rb") as fh, mmap.mmap(fh.fileno(), 0, access=mmap.ACCESS_READ) as mm:
        with tempfile.TemporaryDirectory(prefix="ptv-decompile-index-") as td:
            db_path = Path(td) / "functions.sqlite3"
            with sqlite3.connect(db_path) as conn:
                count = _index_mmap(conn, mm)
                direct = _select_direct(conn, action)
                expanded = _expand_neighbors(
                    conn,
                    direct,
                    int(action.get("neighbors_before", 0)),
                    int(action.get("neighbors_after", 0)),
                )

            lines = [
                "# Historical decompile extraction",
                "",
                f"Source: `{source.name}`",
                f"Source bytes: {st.st_size}",
                f"Indexed functions: {count}",
                f"Direct matches: {len(direct)}",
                f"Extracted functions including neighbors: {len(expanded)}",
                "Index storage: temporary/read-only compatibility index (not written into project source tree)",
                "",
            ]
            direct_ids = {r.row_id for r in direct}
            max_block_bytes = 8 * 1024 * 1024
            for rec in expanded:
                raw = mm[rec.start_offset:rec.end_offset]
                truncated = len(raw) > max_block_bytes
                if truncated:
                    raw = raw[:max_block_bytes]
                lines += [
                    f"## {rec.address_hex} {rec.symbol or '<unknown-symbol>'}",
                    f"Direct match: {'YES' if rec.row_id in direct_ids else 'neighbor'}",
                    f"Signature: `{rec.signature.replace(chr(10), ' ')[:800]}`",
                    "```text",
                    _decode(raw).rstrip(),
                ]
                if truncated:
                    lines.append(f"[TRUNCATED function block at {max_block_bytes} bytes]")
                lines += ["```", ""]

            if bool(action.get("include_references", False)):
                term = action.get("reference_term")
                if not term:
                    term = action.get("name") or action.get("symbol")
                if term:
                    refs = _text_references(
                        mm,
                        str(term),
                        case_sensitive=bool(action.get("case_sensitive", True)),
                        context_lines=int(action.get("reference_context_lines", 8)),
                        max_hits=int(action.get("max_reference_hits", 80)),
                    )
                    lines += [f"# Text references for `{term}`", "", f"Reference hits: {len(refs)}", ""]
                    for line_no, text in refs:
                        lines += [f"## line {line_no}", "```text", text, "```", ""]
            if not direct:
                lines += [
                    "NO DIRECT DECOMPILE MATCHES",
                    "This is a query result, not proof that the semantic symbol is absent from other files.",
                    "",
                ]
            return "\n".join(lines) + "\n"
