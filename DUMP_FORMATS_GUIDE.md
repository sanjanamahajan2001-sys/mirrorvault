# MirrorVault Dump Formats Guide

Complete guide to supported backup formats, dump commands, and restore compatibility.

## Table of Contents

1. [Overview](#overview)
2. [MySQL](#mysql)
3. [PostgreSQL](#postgresql)
4. [MongoDB](#mongodb)
5. [Redis](#redis)
6. [SQLite](#sqlite)
7. [MSSQL](#mssql)
8. [Compression Support](#compression-support)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

---

## Overview

MirrorVault supports **standard database dump formats** created by official database tools. The restore functionality uses native database restore commands that are compatible with dumps created by their corresponding dump tools.

### General Principles

- ✅ Works with standard dump formats (SQL, directory structures, binary files)
- ✅ Supports compression (`.gz` format)
- ✅ Handles single and multi-database dumps
- ✅ Automatically extracts target database from multi-database dumps
- ✅ Uses official database tools for maximum compatibility

---

## MySQL

### Supported Formats

- ✅ **SQL dumps** (`.sql`) - Created with `mysqldump`
- ✅ **Compressed SQL dumps** (`.sql.gz`, `.sql.bz2`, `.sql.zip`) - Created with `mysqldump` and compression
- ✅ **Single database dumps** - `mysqldump database_name`
- ✅ **Multi-database dumps** - `mysqldump --all-databases` or `mysqldump --databases db1 db2`
- ✅ **Custom mysqldump flags** - Works with any `mysqldump` flags

### Backup Commands

#### Without Password (Root Access)
```bash
# Single database
sudo mysqldump -u root DATABASE_NAME > /path/to/backup.sql

# Multiple databases
sudo mysqldump -u root --databases DB1 DB2 DB3 > /path/to/backup.sql

# All databases
sudo mysqldump -u root --all-databases > /path/to/backup.sql

# With compression
sudo mysqldump -u root DATABASE_NAME | gzip > /path/to/backup.sql.gz
```

#### With Password
```bash
# Single database
sudo mysqldump -u root -pPASSWORD DATABASE_NAME > /path/to/backup.sql

# With compression
sudo mysqldump -u root -pPASSWORD DATABASE_NAME | gzip > /path/to/backup.sql.gz
```

#### With Custom Flags
```bash
# Include routines and triggers
sudo mysqldump -u root --routines --triggers DATABASE_NAME > /path/to/backup.sql

# No data (schema only)
sudo mysqldump -u root --no-data DATABASE_NAME > /path/to/backup.sql

# Specific tables
sudo mysqldump -u root DATABASE_NAME table1 table2 > /path/to/backup.sql
```

### Restore Process

MirrorVault uses the `mysql` command to restore SQL dumps:
- Automatically extracts target database from multi-database dumps
- Handles gzip/bzip2/zip compression automatically
- Drops all existing tables before restore (complete replacement)
- Creates database if it doesn't exist

### Example Workflow

```bash
# 1. Create backup
sudo mysqldump -u root app_db > /home/user/app_db_backup.sql

# 2. Verify backup
head -20 /home/user/app_db_backup.sql
ls -lh /home/user/app_db_backup.sql

# 3. Restore using MirrorVault
mirrorvault restore
# Select MySQL → Select app_db → Enter path → Confirm
```

---

## PostgreSQL

### Supported Formats

- ✅ **SQL dumps** (`.sql`) - Created with `pg_dump`
- ✅ **Compressed SQL dumps** (`.sql.gz`, `.sql.bz2`, `.sql.zip`) - Created with `pg_dump` and compression
- ✅ **Single database dumps** - `pg_dump database_name`
- ✅ **Cluster dumps** - `pg_dumpall` (extracts target database)
- ✅ **Plain text format** - `pg_dump -F p` (explicit plain text)
- ✅ **Custom format archives** - `pg_dump -F c`
- ✅ **Directory format** - `pg_dump -F d`

### Backup Commands

#### Without Password (Postgres User)
```bash
# Single database (plain text format)
sudo -u postgres pg_dump -F p DATABASE_NAME > /path/to/backup.sql

# Single database (default format, also plain text)
sudo -u postgres pg_dump DATABASE_NAME > /path/to/backup.sql

# With compression
sudo -u postgres pg_dump -F p DATABASE_NAME | gzip > /path/to/backup.sql.gz

# All databases (cluster dump)
sudo -u postgres pg_dumpall > /path/to/all_dbs.sql
```

#### With Password
```bash
# Single database
sudo -u postgres PGPASSWORD=YOUR_PASSWORD pg_dump -F p DATABASE_NAME > /path/to/backup.sql

# With compression
sudo -u postgres PGPASSWORD=YOUR_PASSWORD pg_dump -F p DATABASE_NAME | gzip > /path/to/backup.sql.gz
```

#### With Custom Options
```bash
# Schema only (no data)
sudo -u postgres pg_dump -F p --schema-only DATABASE_NAME > /path/to/backup.sql

# Data only (no schema)
sudo -u postgres pg_dump -F p --data-only DATABASE_NAME > /path/to/backup.sql

# Specific tables
sudo -u postgres pg_dump -F p -t table1 -t table2 DATABASE_NAME > /path/to/backup.sql
```

### Restore Process

MirrorVault uses the `psql` command to restore SQL dumps and `pg_restore` for custom/directory formats:
- Automatically extracts target database from cluster dumps (`pg_dumpall`)
- Handles gzip/bzip2/zip compression automatically
- Terminates active connections before restore
- Drops all existing tables before restore (complete replacement)
- Creates database if it doesn't exist

### Example Workflow

```bash
# 1. Create backup
sudo -u postgres pg_dump -F p abc > /home/user/abc_backup.sql

# 2. Verify backup
head -20 /home/user/abc_backup.sql
ls -lh /home/user/abc_backup.sql

# 3. Restore using MirrorVault
mirrorvault restore
# Select PostgreSQL → Select abc → Enter path → Confirm
```

---

## MongoDB

### Supported Formats

- ✅ **Directory dumps** - Created with `mongodump` (standard format)
- ✅ **Single database dumps** - `mongodump --db database_name`
- ✅ **Multi-database dumps** - `mongodump` (all databases)
- ✅ **Archive format** - `mongodump --archive` (single file)
- ✅ **Compressed archive** - `mongodump --archive --gzip` (`.gz` file)
- ✅ **Archive in tar/zip** - Extracted and restored automatically

### Backup Commands

#### Directory Dump (Recommended)
```bash
# Single database
mongodump --db DATABASE_NAME --out /path/to/dump

# All databases
mongodump --out /path/to/dump

# With authentication
mongodump --db DATABASE_NAME --username USER --password PASSWORD --out /path/to/dump
```

#### Archive Format (Single File)
```bash
# Single database archive
mongodump --db DATABASE_NAME --archive=/path/to/backup.archive

# Compressed archive
mongodump --db DATABASE_NAME --archive=/path/to/backup.archive.gz --gzip

# All databases archive
mongodump --archive=/path/to/backup.archive
```

### Restore Process

MirrorVault uses the `mongorestore` command to restore dumps:
- Automatically detects directory vs archive format
- Handles multi-database dumps by finding target database subdirectory
- Handles gzip compression automatically
- Drops existing database before restore (complete replacement)

### Example Workflow

```bash
# 1. Create backup (directory format)
mongodump --db demo_db --out /home/user/mongodb_backup

# 2. Verify backup
ls -lh /home/user/mongodb_backup/demo_db/

# 3. Restore using MirrorVault
mirrorvault restore
# Select MongoDB → Select demo_db → Enter path → Confirm
```

---

## Redis

### Supported Formats

- ✅ **RDB files** (`.rdb`) - Standard Redis dump format
- ✅ **Compressed RDB** (`.rdb.gz`, `.rdb.bz2`, `.rdb.zip`) - Compressed RDB files
- ✅ **AOF** (`appendonly.aof`, `.aof`) - Append-only file backups

### Backup Commands

#### Standard RDB Dump
```bash
# Trigger save and copy RDB file
redis-cli SAVE
cp /var/lib/redis/dump.rdb /path/to/backup.rdb

# Or use BGSAVE for non-blocking
redis-cli BGSAVE
# Wait for completion, then copy
cp /var/lib/redis/dump.rdb /path/to/backup.rdb
```

#### Compressed RDB
```bash
# Create compressed backup
redis-cli SAVE
gzip /var/lib/redis/dump.rdb
cp /var/lib/redis/dump.rdb.gz /path/to/backup.rdb.gz
```

### Restore Process

MirrorVault restores RDB and AOF files by:
- Copying `.rdb` or `.aof` file to Redis data directory
- Handling gzip/bzip2/zip compression automatically
- Stopping Redis before restore, starting after
- Replacing existing dump file

### Example Workflow

```bash
# 1. Create backup
redis-cli SAVE
cp /var/lib/redis/dump.rdb /home/user/redis_backup.rdb

# 2. Verify backup
ls -lh /home/user/redis_backup.rdb

# 3. Restore using MirrorVault
mirrorvault restore
# Select Redis → Select dump.rdb → Enter path → Confirm
```

---

## SQLite

### Supported Formats

- ✅ **Database files** (`.db`, `.sqlite`) - Direct copy
- ✅ **SQL dumps** (`.sql`) - Created with `sqlite3 .dump`
- ✅ **Compressed dumps** (`.sql.gz`, `.sql.bz2`, `.sql.zip`, `.db.gz`, `.db.bz2`, `.db.zip`) - Compressed versions

### Backup Commands

#### Direct Database Copy
```bash
# Simple file copy
cp /path/to/database.db /path/to/backup.db

# With timestamp
cp /path/to/database.db /path/to/database_$(date +%Y-%m-%d).db
```

#### SQL Dump
```bash
# Create SQL dump
sqlite3 /path/to/database.db .dump > /path/to/backup.sql

# With compression
sqlite3 /path/to/database.db .dump | gzip > /path/to/backup.sql.gz
```

#### Compressed Database File
```bash
# Compress database file directly
gzip -c /path/to/database.db > /path/to/backup.db.gz
```

### Restore Process

MirrorVault restores SQLite databases by:
- For `.db` files: Direct file copy
- For `.sql` files: Uses `sqlite3` to import SQL dump
- Handles gzip/bzip2/zip compression automatically
- Replaces existing database file

### Example Workflow

```bash
# 1. Create backup (SQL dump)
sqlite3 /home/user/demo_sqlite.db .dump > /home/user/demo_sqlite_backup.sql

# 2. Verify backup
head -20 /home/user/demo_sqlite_backup.sql
ls -lh /home/user/demo_sqlite_backup.sql

# 3. Restore using MirrorVault
mirrorvault restore
# Select SQLite → Select database path → Enter dump path → Confirm
```

---

## MSSQL

### Supported Formats

- ✅ **SQL Server backups** (`.bak`) - Created with `BACKUP DATABASE`
- ✅ **Compressed backups** (`.bak.gz`, `.bak.bz2`, `.bak.zip`) - Extracted automatically

### Backup Commands

#### With SQL Authentication
```bash
sqlcmd -S localhost -U sa -P PASSWORD -Q "BACKUP DATABASE [DB] TO DISK = N'/path/to/backup.bak' WITH INIT, COPY_ONLY;"
```

#### With Windows Authentication
```bash
sqlcmd -S localhost -E -Q "BACKUP DATABASE [DB] TO DISK = N'/path/to/backup.bak' WITH INIT, COPY_ONLY;"
```

### Restore Process

MirrorVault uses `sqlcmd` with `RESTORE DATABASE`:
- Restores from `.bak` files only
- Uses `WITH REPLACE` to restore over existing DB

### Example Workflow

```bash
# 1. Create backup
sqlcmd -S localhost -U sa -P PASSWORD -Q "BACKUP DATABASE [app_db] TO DISK = N'/home/user/app_db.bak' WITH INIT, COPY_ONLY;"

# 2. Verify backup
ls -lh /home/user/app_db.bak

# 3. Restore using MirrorVault
mirrorvault restore
# Select MSSQL → Select app_db → Enter path → Confirm
```

---

## Compression Support

### Supported Compression Formats

- ✅ **Gzip** (`.gz`) - Fully supported for all engines
- ✅ **Bzip2** (`.bz2`) - Supported for restore and optional backup compression
- ✅ **Zip** (`.zip`) - Supported for restore and optional backup compression
- ✅ **Tar** (`.tar.gz`, `.tar.bz2`) - Supported for directory-based dumps
- ❌ **XZ** - Not supported

### Compression Best Practices

1. **Use Gzip**: Most compatible and widely supported
2. **Compress Large Dumps**: Significantly reduces storage space
3. **Verify Compressed Files**: Always verify compressed dumps are valid
4. **Test Restores**: Test compressed dump restores before relying on them

### Compression Examples

```bash
# MySQL compressed
sudo mysqldump -u root DATABASE | gzip > backup.sql.gz

# PostgreSQL compressed
sudo -u postgres pg_dump -F p DATABASE | gzip > backup.sql.gz

# SQLite compressed
sqlite3 database.db .dump | gzip > backup.sql.gz

# Verify compressed file
file backup.sql.gz
zcat backup.sql.gz | head -20
```

### Optional Built-in Backup Compression

MirrorVault can optionally compress backups after creation:

```
MV_BACKUP_COMPRESSION=gz   # gzip
MV_BACKUP_COMPRESSION=bz2  # bzip2 (requires bzip2)
MV_BACKUP_COMPRESSION=zip  # zip
MV_BACKUP_KEEP_SOURCE=true # keep original uncompressed output
```

### Strict Validation (Optional)

Set `MV_STRICT_VALIDATE=true` to enable deeper validation checks where supported:
- MongoDB: `mongorestore --dryRun` (if supported by your version)
- Redis: `redis-check-rdb` (if installed)
- MSSQL: `RESTORE VERIFYONLY`

---

## Multi-Database Dumps

### MySQL Multi-Database

**Creating:**
```bash
# Multiple specific databases
sudo mysqldump -u root --databases db1 db2 db3 > backup.sql

# All databases
sudo mysqldump -u root --all-databases > backup.sql
```

**Restoring:**
- MirrorVault automatically extracts the target database
- Only the selected database is restored
- Other databases in the dump are ignored

### PostgreSQL Multi-Database

**Creating:**
```bash
# All databases (cluster dump)
sudo -u postgres pg_dumpall > backup.sql
```

**Restoring:**
- MirrorVault automatically extracts the target database
- Only the selected database is restored
- Other databases in the dump are ignored

### MongoDB Multi-Database

**Creating:**
```bash
# All databases
mongodump --out /path/to/dump
```

**Restoring:**
- MirrorVault finds the target database subdirectory
- Only the selected database is restored
- Other databases in the dump are ignored

---

## Engine-Specific Backup Options

MirrorVault supports optional format selection for some engines:

- **PostgreSQL**: `MV_POSTGRES_BACKUP_FORMAT=plain|custom|directory`
- **MongoDB**: `MV_MONGO_BACKUP_FORMAT=directory|archive|archive.gz`
- **SQLite**: `MV_SQLITE_BACKUP_MODE=dump|backup` (backup uses SQLite `.backup`)
- **Redis**: `MV_REDIS_BACKUP_MODE=rdb|aof`
- **MSSQL**: `MV_MSSQL_BACKUP_COMPRESSION=true` (adds `WITH COMPRESSION` to backups)

---

## Limitations

### Not Currently Supported

- ❌ MySQL binary logs
- ❌ MongoDB oplog dumps
- ❌ Incremental backups
- ❌ Point-in-time recovery

### Why Standard Formats Work

The restore logic uses **standard database restore commands** that are designed to work with dumps created by their corresponding dump tools. As long as:

1. The dump is in a standard format (SQL, directory structure, etc.)
2. The dump was created with official database tools
3. The dump format matches what the restore command expects

Then the restore should work, regardless of:
- Which flags were used to create the dump
- Whether it's compressed or not
- Whether it contains one or multiple databases
- The specific backup command used

---

## Best Practices

### Creating Backups

1. **Use Standard Commands**: Stick to official tools (`mysqldump`, `pg_dump`, `mongodump`, etc.)
2. **Single Database Dumps**: When possible, create single-database dumps for easier restore
3. **Use Compression**: `.gz` compression is fully supported and recommended
4. **Test Your Dumps**: Verify dumps can be restored before relying on them
5. **Document Your Process**: Keep track of which commands/flags you use
6. **Regular Backups**: Schedule regular backups for critical databases
7. **Verify File Sizes**: Ensure backup files are not empty

### Verifying Backups

```bash
# Check file size (should not be 0)
ls -lh /path/to/backup.sql

# Check file content (SQL dumps)
head -20 /path/to/backup.sql

# Check compressed files
file /path/to/backup.sql.gz
zcat /path/to/backup.sql.gz | head -20

# Count lines (SQL dumps)
wc -l /path/to/backup.sql
```

### Testing Restores

```bash
# Test MySQL restore manually
sudo mysql -u root DATABASE_NAME < /path/to/backup.sql

# Test PostgreSQL restore manually
sudo -u postgres psql -d DATABASE_NAME < /path/to/backup.sql

# Test SQLite restore manually
sqlite3 test.db < /path/to/backup.sql
```

---

## Troubleshooting

### Dump File Issues

**Problem**: Dump file is empty (0 bytes)
```bash
# Check file size
ls -lh /path/to/backup.sql

# Verify dump command worked
# Re-run dump command and check for errors
```

**Problem**: Dump file is corrupted
```bash
# Check file type
file /path/to/backup.sql

# For compressed files
file /path/to/backup.sql.gz
zcat /path/to/backup.sql.gz | head -20

# Try manual restore to verify
```

**Problem**: Dump format not recognized
- Ensure you're using the correct dump command for the engine
- MySQL dumps must be created with `mysqldump`
- PostgreSQL dumps must be created with `pg_dump` or `pg_dumpall`
- Check file extension matches format

### Restore Issues

**Problem**: Restore fails - format incompatible
- Verify dump format matches database engine
- MySQL dumps work with MySQL only
- PostgreSQL dumps work with PostgreSQL only
- Check dump file header to verify format

**Problem**: Restore shows 0 tables after completion
- Check if dump file contains the target database
- For multi-database dumps, ensure target DB exists in dump
- Verify dump file is not corrupted
- Check restore logs in `/var/log/mirrorvault/`

**Problem**: Restore fails - database not found in dump
- For multi-database dumps, verify target database name
- Check dump file content: `grep -i "CREATE DATABASE" backup.sql`
- Ensure database name matches exactly (case-sensitive for some engines)

### Compression Issues

**Problem**: Compressed file not recognized
```bash
# Verify compression format
file /path/to/backup.sql.gz

# Test decompression
zcat /path/to/backup.sql.gz | head -20

# Re-compress if needed
gzip -c /path/to/backup.sql > /path/to/backup.sql.gz
```

---

## Format Compatibility Matrix

| Engine | SQL Dump | Directory | Archive | Binary | Compressed |
|--------|----------|-----------|---------|--------|------------|
| MySQL | ✅ | ❌ | ❌ | ❌ | ✅ (.gz/.bz2/.zip) |
| PostgreSQL | ✅ | ✅ (pg_dump -F d) | ✅ (pg_dump -F c) | ❌ | ✅ (.gz/.bz2/.zip) |
| MongoDB | ❌ | ✅ | ✅ | ❌ | ✅ (.gz/.bz2/.zip/.tar.*) |
| Redis | ❌ | ❌ | ❌ | ✅ (.rdb/.aof) | ✅ (.gz/.bz2/.zip) |
| SQLite | ✅ | ❌ | ❌ | ✅ (.db) | ✅ (.gz/.bz2/.zip) |
| MSSQL | ❌ | ❌ | ❌ | ✅ (.bak) | ✅ (.gz/.bz2/.zip) |

---

## Command Quick Reference

### MySQL
```bash
# Backup
sudo mysqldump -u root DATABASE > backup.sql
sudo mysqldump -u root DATABASE | gzip > backup.sql.gz

# Restore (manual)
sudo mysql -u root DATABASE < backup.sql
```

### PostgreSQL
```bash
# Backup
sudo -u postgres pg_dump -F p DATABASE > backup.sql
sudo -u postgres pg_dump -F p DATABASE | gzip > backup.sql.gz

# Restore (manual)
sudo -u postgres psql -d DATABASE < backup.sql
```

### MongoDB
```bash
# Backup
mongodump --db DATABASE --out /path/to/dump
mongodump --db DATABASE --archive=backup.archive.gz --gzip

# Restore (manual)
mongorestore --db DATABASE /path/to/dump/DATABASE
mongorestore --archive=backup.archive.gz --gzip
```

### Redis
```bash
# Backup
redis-cli SAVE
cp /var/lib/redis/dump.rdb backup.rdb

# Restore (manual)
cp backup.rdb /var/lib/redis/dump.rdb
sudo systemctl restart redis
```

### SQLite
```bash
# Backup
sqlite3 database.db .dump > backup.sql
cp database.db backup.db

# Restore (manual)
sqlite3 database.db < backup.sql
cp backup.db database.db
```

### MSSQL
```bash
# Backup
sqlcmd -S localhost -U sa -P PASSWORD -Q "BACKUP DATABASE [DB] TO DISK = N'/path/to/backup.bak' WITH INIT, COPY_ONLY;"

# Restore (manual)
sqlcmd -S localhost -U sa -P PASSWORD -Q "RESTORE DATABASE [DB] FROM DISK = N'/path/to/backup.bak' WITH REPLACE;"
```

---

For user commands and functionality, see [USER_GUIDE.md](USER_GUIDE.md).  
For developer information, see [DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md).
