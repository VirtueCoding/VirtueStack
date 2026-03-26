---
applyTo: "migrations/**/*.sql"
---

# SQL Migration Instructions

## Naming

Migrations use sequential numbering: `000001_descriptive_name.up.sql` / `000001_descriptive_name.down.sql`. Always create both `.up.sql` and `.down.sql` files. Use `make migrate-create NAME=feature_name` to generate the pair.

## Zero-Downtime Pattern (Expand-Contract)

Follow the expand-contract pattern for all schema changes:

1. **Expand:** Add nullable columns with defaults, create new tables/indexes. Never rename or drop columns.
2. **Migrate:** Dual-write to old and new columns, backfill data.
3. **Contract:** Drop old columns in a separate, later migration.

## Rules

- Every migration must have a rollback script (`.down.sql`).
- Migrations must be idempotent — safe to run multiple times.
- Always add `SET lock_timeout = '5s';` to prevent long table locks.
- Index every column used in `WHERE`, `JOIN`, or `ORDER BY` clauses.
- Never use `CREATE INDEX CONCURRENTLY` inside migrations — golang-migrate wraps them in transactions.
- Use parameterized/positional queries — never string interpolation.

## PostgreSQL Conventions

- Use `UUID` for primary keys (generated with `gen_random_uuid()`).
- Include `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` and `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` on all tables.
- Foreign key names: `{table}_{column}_fk`.
- Index names: `idx_{table}_{columns}`.
- Enable Row Level Security on all customer-facing tables.

## Prohibitions

- No direct column renames — use expand-contract.
- No direct column drops — use expand-contract.
- No table renames.
- No `CREATE INDEX CONCURRENTLY` (incompatible with golang-migrate transactions).
