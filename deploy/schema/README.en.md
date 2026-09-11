> 🌐 **English** | [中文](README.md)

# schema/ — Baseline database-creation SQL (MySQL / PostgreSQL)

Full database-creation scripts that correspond exactly to the backend GORM models
(`backend/internal/model/model.go`'s `AllModels()`, 16 tables). **Exported from a real
environment**: after letting the app's `AutoMigrate` create tables on MySQL 8.4 /
PostgreSQL 17.10, the scripts were exported via `mysqldump --no-data` /
`pg_dump --schema-only`, so they are column-for-column identical to the auto-created output.

| File | Applies to |
|------|------------|
| `schema.mysql.sql` | MySQL 8.x (utf8mb4 / utf8mb4_unicode_ci) |
| `schema.postgres.sql` | PostgreSQL 12+ (the psql 17 `\restrict` guard lines have been removed, any client can re-apply) |

## Usage

```bash
# MySQL
mysql -uroot -p -e "CREATE DATABASE gateway_mysql CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mysql -uroot -p gateway_mysql < schema.mysql.sql

# PostgreSQL
psql -U postgres -c "CREATE DATABASE gateway_pg;"
psql -U postgres -d gateway_pg -f schema.postgres.sql

# Then start the service normally (DB_TYPE=mysql|postgres + DB_DSN)
```

## Relationship to AutoMigrate

- The app still runs `AutoMigrate` on startup (REQ-009): for a database pre-created with these scripts it is an **idempotent no-op**.
- Verified: load this directory's scripts into a fresh database → start the service → a full 20-step E2E passes 20/20 (on both databases); re-dumping then matches this directory's files **line for line** (`deploy/verify_schema.sh`, both MySQL and PG report ROUNDTRIP_IDENTICAL).
- To regenerate after a model change: on a test machine run `bash deploy/make_schema.sh` → overwrite this directory → `bash deploy/verify_schema.sh`.
- SQLite needs no script (the app auto-creates the database and tables).
