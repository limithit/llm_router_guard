#!/bin/bash
# Environment for gateway E2E tests (sourced by callers)
export DB_MYSQL_DSN='root:comeback@tcp(127.0.0.1:3306)/gateway_mysql?charset=utf8mb4&parseTime=True&loc=Local'
export DB_PG_DSN='host=127.0.0.1 port=5432 user=postgres password=comeback dbname=gateway_pg sslmode=disable TimeZone=Asia/Shanghai'
export MYSQL_PWD=comeback
export PGPASSWORD=comeback
