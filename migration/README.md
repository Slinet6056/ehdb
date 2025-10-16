# E-Hentai Database Migration Guide

This guide describes how to migrate E-Hentai database from MySQL/MariaDB to PostgreSQL.

## Prerequisites

- MySQL/MariaDB
- PostgreSQL
- [pgloader](https://github.com/dimitri/pgloader)
- Database dump file from [URenko/e-hentai-db releases](https://github.com/URenko/e-hentai-db/releases)

## Migration Steps

### 1. Download and Extract Database Dump

Download `nightly.sql.zstd` from the [releases page](https://github.com/URenko/e-hentai-db/releases) and extract it:

```bash
zstd -d nightly.sql.zstd
```

### 2. Create Databases

Create databases in both MySQL and PostgreSQL:

```bash
# MySQL/MariaDB
mysql -u user -p -e "CREATE DATABASE e_hentai_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# PostgreSQL
psql -U user -c "CREATE DATABASE ehentai_db;"
```

### 3. Import Data to MySQL

Import the extracted SQL file into MySQL:

```bash
mysql -u user -p e_hentai_db < nightly.sql
```

### 4. Run Pre-migration Script

Clean up data before migration:

```bash
mysql -u user -p e_hentai_db < pre_migration.sql
```

### 5. Configure and Run pgloader

Edit `pgloader.conf` to update database connection strings:

```
FROM mysql://user:password@localhost:3306/e_hentai_db
INTO postgresql://user:password@localhost:5432/ehentai_db
```

Run the migration:

```bash
pgloader pgloader.conf
```

### 6. Run Post-migration Script

Complete the migration by running the post-migration script in PostgreSQL:

```bash
psql -U user -d ehentai_db -f post_migration.sql
```

## Notes

- The default MySQL database name is `e_hentai_db`
- The default PostgreSQL database name is `ehentai_db`
- If you use different database names, update them in `pgloader.conf`, `pre_migration.sql`, and `post_migration.sql`

## Acknowledgments

This migration is based on the work of:

- [ccloli/e-hentai-db](https://github.com/ccloli/e-hentai-db) - MySQL-based database and synchronization scripts
- [URenko/e-hentai-db](https://github.com/URenko/e-hentai-db) - Live crawling and regular database dumps
