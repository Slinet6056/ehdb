# E-Hentai Database Migration Guide

This guide describes how to migrate E-Hentai database from SQLite to PostgreSQL.

## Prerequisites

- SQLite 3
- PostgreSQL
- [pgloader](https://github.com/dimitri/pgloader)
- Database dump file from [URenko/e-hentai-db releases](https://github.com/URenko/e-hentai-db/releases)

## Migration Steps

### 1. Download and Extract Database Dump

Download `e-hentai.db.zst` from the [releases page](https://github.com/URenko/e-hentai-db/releases) and extract it:

```bash
zstd -d e-hentai.db.zst -o e-hentai.db
```

### 2. Create Databases

Create the PostgreSQL database:

```bash
# PostgreSQL
psql -U user -c "CREATE DATABASE ehentai_db;"
```

### 3. Run Pre-migration Script

Clean up the extracted SQLite database before migration:

```bash
sqlite3 e-hentai.db < pre_migration.sql
```

### 4. Configure and Run pgloader

Edit `pgloader.conf` to update the SQLite file path and PostgreSQL connection string:

```
FROM sqlite:///path/to/e-hentai.db
INTO postgresql://user:password@localhost:5432/ehentai_db
```

Run the migration:

```bash
pgloader pgloader.conf
```

### 5. Run Post-migration Script

Complete the migration by running the post-migration script in PostgreSQL:

```bash
psql -U user -d ehentai_db -f post_migration.sql
```

## Notes

- The default PostgreSQL database name is `ehentai_db`
- If you use different database names or file paths, update them in `pgloader.conf` and the commands above

## Acknowledgments

This migration is based on the work of:

- [ccloli/e-hentai-db](https://github.com/ccloli/e-hentai-db) - MySQL-based database and synchronization scripts
- [URenko/e-hentai-db](https://github.com/URenko/e-hentai-db) - Live crawling and regular database dumps
