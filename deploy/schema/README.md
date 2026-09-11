> 🌐 [English](README.en.md) | **中文**

# schema/ — 基线建库 SQL（MySQL / PostgreSQL）

与后端 GORM 模型（`backend/internal/model/model.go` 的 `AllModels()`，共 16 表）完全对应的
完整建库脚本。**由真实环境导出**：让应用 `AutoMigrate` 在 MySQL 8.4 / PostgreSQL 17.10 上建表后，
分别用 `mysqldump --no-data` / `pg_dump --schema-only` 导出，因此与自动建表产物逐列一致。

| 文件 | 适用 |
|------|------|
| `schema.mysql.sql` | MySQL 8.x（utf8mb4 / utf8mb4_unicode_ci） |
| `schema.postgres.sql` | PostgreSQL 12+（已去除 psql 17 的 `\restrict` 保护行，任意客户端可回灌） |

## 使用方式

```bash
# MySQL
mysql -uroot -p -e "CREATE DATABASE gateway_mysql CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
mysql -uroot -p gateway_mysql < schema.mysql.sql

# PostgreSQL
psql -U postgres -c "CREATE DATABASE gateway_pg;"
psql -U postgres -d gateway_pg -f schema.postgres.sql

# 然后正常启动服务（DB_TYPE=mysql|postgres + DB_DSN）
```

## 与 AutoMigrate 的关系

- 应用启动时仍会执行 `AutoMigrate`（REQ-009）：对按本脚本预建的库是**幂等 no-op**；
- 已验证：把本目录脚本灌入全新库 → 启动服务 → 完整 20 步 E2E 20/20（双库均过）；
  再重新 dump 与本目录文件 **逐行一致**（`deploy/verify_schema.sh`，MySQL/PG 双 ROUNDTRIP_IDENTICAL）。
- 升级模型后重新生成：测试机 `bash deploy/make_schema.sh` → 覆盖本目录 → `bash deploy/verify_schema.sh`。
- SQLite 无需脚本（应用自动建库建表）。
