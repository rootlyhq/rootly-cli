# PostgreSQL

We run **PostgreSQL 16**. All guidance below assumes PG 16 features and behavior.

## String Column Types: Use `text`

**Always use `t.text` for string columns in migrations, never `t.string`.**

```ruby
# Good
t.text "name", null: false

# Bad
t.string "name", null: false
```

**Rationale:**
- In PostgreSQL, `text` and `varchar`/`string` have no performance difference — they are stored identically
- Simpler mental model — one type for all string data
- No varchar length limits to worry about
- Aligns with PostgreSQL best practices

**Reference:** [PostgreSQL documentation on character types](https://www.postgresql.org/docs/current/datatype-character.html)

## Adding Columns with Defaults Is Safe

Since PG 11, `ADD COLUMN ... DEFAULT` with a **non-volatile** default no longer rewrites the table — the default is stored in the catalog and applied on read. On PG 16, this is a safe, fast metadata-only operation for any table size.

```ruby
# Safe on PG 16 — no table rewrite (non-volatile default)
add_column :users, :admin, :boolean, default: false
```

There is no need for the two-step `add_column` + `change_column_default` pattern that was necessary on PG 10 and earlier.

**Caution:** Volatile defaults (e.g., `gen_random_uuid()`, `now()`) still trigger a table rewrite. See [Safe Database Migrations](../rails/migrations.md#2-adding-a-column-with-a-default-value) for the two-step pattern to handle these safely.

## Use `jsonb`, Not `json`

**Always use `jsonb` for JSON columns.** The `json` type stores raw text and must be reparsed on every access. `jsonb` stores a decomposed binary format that supports indexing and is faster to query.

```ruby
# Good
t.jsonb "metadata", default: {}

# Bad
t.json "metadata"
```

**Indexing JSONB columns:**

```ruby
# GIN index for containment queries (@>, ?, ?|, ?&)
add_index :events, :metadata, using: :gin

# GIN with jsonb_path_ops for @> only (smaller, faster)
add_index :events, :metadata, using: :gin, opclass: :jsonb_path_ops

# Expression index for a single key you query often
add_index :events, "(metadata->>'type')"
```

Avoid deeply nested JSONB structures — prefer flat keys and lean on relational columns for data you filter or join on frequently.

## Index Type Selection

Choose the right index type for the access pattern:

| Index Type | Use When | Example |
|------------|----------|---------|
| **B-tree** (default) | Equality and range queries on scalar columns | `add_index :users, :email` |
| **GIN** | JSONB containment, array `ANY`/`@>`, full-text search `tsvector` | `add_index :events, :tags, using: :gin` |
| **GiST** | Range types, geometric data, exclusion constraints | `add_index :reservations, :during, using: :gist` |
| **BRIN** | Very large tables with naturally ordered data (timestamps, auto-increment IDs) | `add_index :audit_logs, :created_at, using: :brin` |
| **Hash** | Equality-only lookups (rare — B-tree is usually better) | Rarely needed |

### Partial Indexes

Use partial indexes to reduce index size and improve write performance when you only query a subset of rows:

```ruby
# Only index active users — smaller and faster than a full index
add_index :users, :email, where: "discarded_at IS NULL"

# Only index unprocessed jobs
add_index :jobs, :scheduled_at, where: "status = 'pending'"
```

### Covering Indexes (INCLUDE)

Use `INCLUDE` to add non-key columns to an index for index-only scans without affecting sort order or uniqueness:

```ruby
# Index-only scan: PG can answer the query from the index alone
add_index :orders, :user_id, include: [:status, :created_at]
```

## Avoid ENUM Types

**Prefer check constraints or a reference/lookup table over PostgreSQL `ENUM` types.** Enums cannot have values removed, reordering requires recreating the type, and adding values in a transaction requires special handling.

```ruby
# Good — check constraint, easy to modify
add_check_constraint :incidents, "severity IN ('sev0', 'sev1', 'sev2', 'sev3')", name: "severity_check"

# Good — integer column with Rails enum
t.integer "status", default: 0, null: false

# Bad — PG enum type
execute "CREATE TYPE severity AS ENUM ('sev0', 'sev1', 'sev2', 'sev3')"
```

## UUID Primary Keys

If a table uses UUID primary keys, use `gen_random_uuid()` (built into PG 13+) rather than the `pgcrypto` or `uuid-ossp` extensions:

```ruby
# No extension needed — gen_random_uuid() is built into PG 13+
create_table :events, id: :uuid, default: -> { "gen_random_uuid()" } do |t|
  # ...
end
```

**When to prefer UUIDs over bigint:**
- Public-facing IDs (avoids exposing row counts / enumeration)
- Data that may be generated across distributed systems
- Tables where sequential IDs leak business information

**When to prefer bigint (the default):**
- Internal tables with no external exposure
- Tables that benefit from smaller, ordered keys (less index bloat, better cache locality)

## Connection Pooling

Use [PgBouncer](https://www.pgbouncer.org/) in front of PostgreSQL. Rails' built-in connection pool manages connections per-process, but with multiple Puma workers, Sidekiq processes, and console sessions, total connections add up fast.

**Guidelines:**
- Set `pool` in `database.yml` to match the thread count per process (Puma threads or Sidekiq concurrency)
- Use PgBouncer in **transaction mode** for web traffic — connections are returned to the pool after each transaction
- Avoid `SET` session-level commands (e.g., `SET statement_timeout`) in transaction mode — they leak across checkouts. Use `SET LOCAL` inside a transaction instead
- Monitor active connections with: `SELECT count(*) FROM pg_stat_activity WHERE state = 'active';`

## Query Optimization

### Use `EXPLAIN (ANALYZE, BUFFERS)` for Investigation

When debugging slow queries, always use `EXPLAIN (ANALYZE, BUFFERS)` rather than plain `EXPLAIN` — it shows actual row counts, buffer hits, and I/O:

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT * FROM incidents WHERE organization_id = 42 AND status = 'open';
```

In Rails:

```ruby
Incident.where(organization_id: 42, status: :open).explain(:analyze)
```

### Statement Timeout

Set a statement timeout to prevent runaway queries from holding connections. Since we use PgBouncer in transaction mode, **always use `SET LOCAL` inside a transaction** — session-level `SET` leaks across PgBouncer checkouts:

```ruby
# Good — scoped to this transaction only, safe with PgBouncer
ActiveRecord::Base.transaction do
  ActiveRecord::Base.connection.execute("SET LOCAL statement_timeout = '30s'")
  # query
end

# Bad — session-level SET leaks in PgBouncer transaction mode
ActiveRecord::Base.connection.execute("SET statement_timeout = '30s'")
```

For long-running tasks (migrations, backfills), increase the timeout within the transaction:

```ruby
ActiveRecord::Base.transaction do
  ActiveRecord::Base.connection.execute("SET LOCAL statement_timeout = '5min'")
  # long operation
end
```

## Advisory Locks

Use [PostgreSQL advisory locks](https://www.postgresql.org/docs/16/explicit-locking.html#ADVISORY-LOCKS) for application-level coordination when you need a distributed mutex without a separate service:

```ruby
# Session-level lock (released at disconnect or explicit unlock)
ActiveRecord::Base.connection.execute("SELECT pg_advisory_lock(42)")

# Transaction-level lock (released at end of transaction — preferred)
ActiveRecord::Base.transaction do
  ActiveRecord::Base.connection.execute("SELECT pg_advisory_xact_lock(42)")
  # critical section
end
```

Prefer transaction-level locks (`pg_advisory_xact_lock`) — they cannot leak if the application crashes.

## Additional Resources

- [PostgreSQL 16 Release Notes](https://www.postgresql.org/docs/16/release-16.html)
- [PostgreSQL Documentation: Character Types](https://www.postgresql.org/docs/16/datatype-character.html)
- [PostgreSQL Documentation: JSON Types](https://www.postgresql.org/docs/16/datatype-json.html)
- [PostgreSQL Documentation: Index Types](https://www.postgresql.org/docs/16/indexes-types.html)
- [Use The Index, Luke](https://use-the-index-luke.com/) — SQL indexing and tuning guide
