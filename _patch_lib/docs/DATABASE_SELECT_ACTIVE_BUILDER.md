# Database SELECT active builder — Python Patch Tool v6.19.0

`database_select` is a read-only COLLECT action for gathering database evidence. It is intentionally **not** a raw SQL runner.

## Non-negotiable safety contract

1. Request ZIPs never contain executable SQL text. There is no `query`, `raw_sql`, `sql`, `expression_sql`, or similar escape hatch.
2. AI describes a SELECT as a structured AST. Patch Tool validates the AST and constructs the SQL itself.
3. The generated statement class is always `SELECT`. The compiler rejects statement separators and SQL comment syntax in generated SQL.
4. SQLite databases are opened with URI `mode=ro` and an SQLite authorizer denies non-read operations.
5. MySQL authentication is only by `mysql_config_editor` login path. Password fields are rejected from local profiles.
6. Remote MySQL uses a bounded SSH local tunnel. SSH credentials/options remain in the operator's normal SSH configuration.
7. A MySQL account used by a profile SHOULD have SELECT-only grants. This is the independent server-side safety boundary even if the client has a bug.
8. The local profile file is never read from the request ZIP and is a hard evidence exclusion: COLLECT/search/FAIL_HANDOFF must never package or content-search it, even when a request explicitly names the file.
9. Timeout/row/byte limits are fail-partial: keep rows already collected, mark the action and COLLECT `INCOMPLETE`, and still publish the result ZIP.
10. Hard connection/auth/schema/execution errors remain FAIL; they are not misreported as partial success.

## Local profile file

Default lookup order:

```text
<project>/tools/db_profiles.local.json
<project>/.python_patch_tool/db_profiles.local.json
```

An operator may explicitly override the path with `PTV_DB_PROFILES_FILE`.

Start from `tools/db_profiles.example.json`. The local file is operator-owned and MUST NOT be put inside a COLLECT request ZIP.

### SQLite

```json
{
  "version": 1,
  "profiles": {
    "game_sqlite": {
      "engine": "sqlite",
      "path": "data/game.sqlite"
    }
  }
}
```

Relative SQLite paths are resolved from the project root. The target must be a real regular non-symlink file.

### MySQL local

```json
{
  "version": 1,
  "profiles": {
    "m3_mysql_local": {
      "engine": "mysql",
      "transport": "local",
      "host": "127.0.0.1",
      "port": 3306,
      "database": "m3",
      "auth": {
        "type": "login_path",
        "login_path": "m3_readonly"
      }
    }
  }
}
```

`transport=local` accepts loopback only. Remote servers must use `ssh_tunnel`.

Create a login path interactively, for example:

```bash
mysql_config_editor set --login-path=m3_readonly --host=127.0.0.1 --user=<readonly-user> --password
```

Patch Tool never receives the password.

### MySQL through SSH

```json
{
  "version": 1,
  "profiles": {
    "m3_mysql_remote": {
      "engine": "mysql",
      "transport": "ssh_tunnel",
      "database": "m3",
      "auth": {
        "type": "login_path",
        "login_path": "m3_readonly"
      },
      "ssh": {
        "target": "m3-local1",
        "remote_host": "127.0.0.1",
        "remote_port": 3306,
        "connect_timeout_sec": 10
      }
    }
  }
}
```

`ssh.target` should normally be an alias from `~/.ssh/config`. Patch Tool uses `BatchMode=yes`, `ExitOnForwardFailure=yes`, a temporary loopback port, and tears the tunnel down after the query.

## `database_select` request shape

```json
{
  "type": "database_select",
  "id": "active_users",
  "profile": "m3_mysql_remote",

  "select": [
    {"expr": {"column": "u.id"}, "alias": "user_id"},
    {"expr": {"column": "u.name"}, "alias": "user_name"}
  ],

  "from": {"table": "users", "alias": "u"},

  "joins": [],
  "group_by": [],
  "order_by": [],
  "distinct": false,
  "limit": 1000,
  "offset": 0,

  "format": "csv",
  "max_rows": 100000,
  "max_bytes": 104857600,
  "timeout_sec": 120,
  "chunk_rows": 10000,
  "must_return_rows": false
}
```

`where`, `having`, `joins`, `group_by`, `order_by`, `distinct`, `limit`, and `offset` are optional. If `offset` is present, `limit` is required.

## Expression AST

Every expression object has exactly one expression kind.

### Column

```json
{"column": "u.id"}
{"column": "u.*"}
{"column": "*"}
```

Identifiers are validated and quoted by the selected SQL dialect.

### Bound value

```json
{"value": 123}
{"value": "paid"}
{"value": null}
{"value": true}
```

Values never become SQL syntax. SQLite uses bound parameters. MySQL string values are emitted as UTF-8 hex data wrapped by a tool-generated conversion expression so quote/backslash/sql-mode content cannot alter statement structure.

### Function / aggregate

```json
{
  "function": {
    "name": "COUNT",
    "args": [{"column": "o.id"}],
    "distinct": true
  }
}
```

Function names are allowlisted. Dangerous/arbitrary functions such as `SLEEP`, `BENCHMARK`, `LOAD_FILE`, or user-defined function names are not accepted merely because they form syntactically valid SELECT expressions.

### Arithmetic

```json
{
  "binary": {
    "op": "+",
    "left": {"column": "a.score"},
    "right": {"column": "b.score"}
  }
}
```

Allowed operators: `+ - * / %`.

### CASE

```json
{
  "case": {
    "when": [
      {
        "if": {
          "compare": {
            "left": {"column": "u.level"},
            "op": ">=",
            "right": {"value": 100}
          }
        },
        "then": {"value": "high"}
      }
    ],
    "else": {"value": "normal"}
  }
}
```

### Scalar subquery

```json
{
  "subquery": {
    "select": [
      {"expr": {"function": {"name": "MAX", "args": [{"column": "x.score"}]}}}
    ],
    "from": {"table": "scores", "alias": "x"}
  }
}
```

### CAST

```json
{"cast": {"expr": {"column": "u.level"}, "as": "INTEGER"}}
```

Cast target types are allowlisted.

## WHERE / HAVING condition AST

Conditions are recursive; this is how grouped `AND` / `OR` logic is represented without raw parentheses text.

### AND / OR groups

```json
{
  "and": [
    {
      "compare": {
        "left": {"column": "u.level"},
        "op": ">=",
        "right": {"value": 100}
      }
    },
    {
      "or": [
        {
          "compare": {
            "left": {"column": "u.region"},
            "op": "=",
            "right": {"value": "vn"}
          }
        },
        {
          "compare": {
            "left": {"column": "u.vip"},
            "op": ">",
            "right": {"value": 0}
          }
        }
      ]
    }
  ]
}
```

The builder emits the equivalent of:

```sql
WHERE (u.level >= ? AND (u.region = ? OR u.vip > ?))
```

### NOT

```json
{"not": {"compare": {"left": {"column": "u.status"}, "op": "=", "right": {"value": "banned"}}}}
```

### Comparisons

Supported operators:

```text
= != <> < <= > >=
LIKE NOT LIKE
IN NOT IN
BETWEEN NOT BETWEEN
IS NULL IS NOT NULL
```

`IN` accepts a non-empty expression array or a subquery.

### EXISTS / NOT EXISTS

```json
{
  "exists": {
    "not": false,
    "query": {
      "select": [{"expr": {"column": "o.id"}}],
      "from": {"table": "orders", "alias": "o"},
      "where": {
        "compare": {
          "left": {"column": "o.user_id"},
          "op": "=",
          "right": {"column": "u.id"}
        }
      }
    }
  }
}
```

## JOIN

```json
{
  "joins": [
    {
      "type": "LEFT",
      "source": {"table": "orders", "alias": "o"},
      "on": {
        "and": [
          {
            "compare": {
              "left": {"column": "o.user_id"},
              "op": "=",
              "right": {"column": "u.id"}
            }
          },
          {
            "compare": {
              "left": {"column": "o.status"},
              "op": "=",
              "right": {"value": "paid"}
            }
          }
        ]
      }
    }
  ]
}
```

Supported join types: `INNER`, `LEFT`, `RIGHT`, `CROSS`. `CROSS` has no `on`; all others require it.

A JOIN source may also be a subquery:

```json
{
  "source": {
    "subquery": {
      "select": [{"expr": {"column": "x.user_id"}}],
      "from": {"table": "orders", "alias": "x"}
    },
    "alias": "q"
  }
}
```

## GROUP BY + HAVING

```json
{
  "group_by": [
    {"column": "u.id"},
    {"column": "u.name"}
  ],
  "having": {
    "compare": {
      "left": {"function": {"name": "COUNT", "args": [{"column": "o.id"}]}},
      "op": ">",
      "right": {"value": 2}
    }
  }
}
```

`having` is accepted only when `group_by` is present.

## Window functions

Window-capable allowlisted functions can use an `over` object:

```json
{
  "expr": {
    "function": {
      "name": "ROW_NUMBER",
      "args": [],
      "over": {
        "partition_by": [{"column": "u.team_id"}],
        "order_by": [
          {"expr": {"column": "u.score"}, "direction": "DESC"}
        ],
        "frame": {
          "unit": "ROWS",
          "start": "UNBOUNDED PRECEDING",
          "end": "CURRENT ROW"
        }
      }
    }
  },
  "alias": "rank_no"
}
```

## Output and partial-result semantics

Output format is `csv` or `jsonl`. Data is streamed to bounded chunks under:

```text
database_queries/<action>/result/result_0001.csv
```

or:

```text
database_queries/<action>/result/result_0001.jsonl
```

The same action directory includes:

```text
META.json
BUILT_QUERY.sql
BOUND_VALUES_META.json
ACTIVE_QUERY_STRUCTURE.json
```

`BUILT_QUERY.sql` is a display template with `?` placeholders rather than secret/request values. `ACTIVE_QUERY_STRUCTURE.json` replaces bound values by their types.

If the database is exhausted before any output limit:

```text
Execution status: COMPLETED
Coverage status: VERIFIED
COLLECT PASS
```

If `max_rows`, `max_bytes`, timeout, or outer COLLECT package quotas stop output early:

```text
Execution status: PARTIAL
Coverage status: PARTIAL
COLLECT INCOMPLETE
```

Rows/chunks already written remain in the result ZIP.

## What is intentionally not supported

The database lane does not support:

- INSERT / UPDATE / DELETE / REPLACE;
- CREATE / ALTER / DROP / TRUNCATE;
- CALL / stored procedure execution;
- raw SQL text;
- arbitrary function names;
- multi-statement SQL;
- MySQL direct remote TCP profiles;
- password fields in request/profile JSON;
- `SHOW`, `DESCRIBE`, or `PRAGMA` actions.

For schema evidence, build a SELECT against `information_schema` (MySQL) or `sqlite_master` (SQLite) through the same active builder.
