# Supported Backup Formats for Restore

This document describes all the backup formats and commands that the restore functionality supports.

## General Principles

The restore logic is designed to work with **standard database dump formats** created by official database tools. It uses the native restore commands (`mysql`, `psql`, `mongorestore`, `sqlite3`, `redis-cli`) which are compatible with dumps created by their corresponding dump tools.

## MySQL

### Supported Formats
- ✅ **SQL dumps** (`.sql`) - Created with `mysqldump`
- ✅ **Compressed SQL dumps** (`.sql.gz`, `.sql.bz2`) - Created with `mysqldump | gzip`
- ✅ **Single database dumps** - `mysqldump database_name`
- ✅ **Multi-database dumps** - `mysqldump --all-databases` or `mysqldump --databases db1 db2`
- ✅ **Custom mysqldump flags** - Works with any `mysqldump` flags (e.g., `--no-data`, `--routines`, `--triggers`)

### How It Works
- Uses `mysql` command to restore SQL dumps
- Automatically extracts target database from multi-database dumps
- Handles gzip compression automatically
- Drops all existing tables before restore (complete replacement)

### Example Commands That Work
```bash
# Standard single database dump
mysqldump -u root mydb > backup.sql

# Compressed dump
mysqldump -u root mydb | gzip > backup.sql.gz

# Multi-database dump (will extract target DB)
mysqldump -u root --all-databases > all_dbs.sql

# With custom flags
mysqldump -u root --routines --triggers mydb > backup.sql
```

## PostgreSQL

### Supported Formats
- ✅ **SQL dumps** (`.sql`) - Created with `pg_dump`
- ✅ **Compressed SQL dumps** (`.sql.gz`, `.sql.bz2`) - Created with `pg_dump | gzip`
- ✅ **Single database dumps** - `pg_dump database_name`
- ✅ **Cluster dumps** - `pg_dumpall` (extracts target database)
- ✅ **Custom format dumps** - `pg_dump -F p` (plain text format)
- ⚠️ **Custom format archives** - `pg_dump -F c` (custom format) - **NOT YET SUPPORTED**

### How It Works
- Uses `psql` command to restore SQL dumps
- Automatically extracts target database from cluster dumps (`pg_dumpall`)
- Handles gzip compression automatically
- Terminates active connections before restore
- Drops all existing tables before restore (complete replacement)

### Example Commands That Work
```bash
# Standard single database dump
pg_dump -U postgres mydb > backup.sql

# Compressed dump
pg_dump -U postgres mydb | gzip > backup.sql.gz

# Cluster dump (will extract target DB)
pg_dumpall -U postgres > all_dbs.sql

# Plain text format
pg_dump -F p -U postgres mydb > backup.sql
```

## MongoDB

### Supported Formats
- ✅ **Directory dumps** - Created with `mongodump` (standard format)
- ✅ **Single database dumps** - `mongodump --db database_name`
- ✅ **Multi-database dumps** - `mongodump` (all databases)
- ✅ **Archive format** - `mongodump --archive` (single file)
- ✅ **Compressed archive** - `mongodump --archive --gzip` (`.gz` file)

### How It Works
- Uses `mongorestore` command to restore dumps
- Automatically detects directory vs archive format
- Handles multi-database dumps by finding target database subdirectory
- Drops existing database before restore (complete replacement)

### Example Commands That Work
```bash
# Standard directory dump (single DB)
mongodump --db mydb --out /path/to/dump

# Standard directory dump (all DBs)
mongodump --out /path/to/dump

# Archive format (single file)
mongodump --db mydb --archive=backup.archive

# Compressed archive
mongodump --db mydb --archive=backup.archive.gz --gzip
```

## SQLite

### Supported Formats
- ✅ **Database files** (`.db`, `.sqlite`) - Direct copy
- ✅ **SQL dumps** (`.sql`) - Created with `sqlite3 .dump`
- ✅ **Compressed dumps** (`.sql.gz`, `.db.gz`) - Compressed versions

### How It Works
- For `.db` files: Direct file copy
- For `.sql` files: Uses `sqlite3` to import SQL dump
- Handles gzip compression automatically

### Example Commands That Work
```bash
# Direct database copy
cp database.db backup.db

# SQL dump
sqlite3 database.db .dump > backup.sql

# Compressed SQL dump
sqlite3 database.db .dump | gzip > backup.sql.gz
```

## Redis

### Supported Formats
- ✅ **RDB files** (`.rdb`) - Standard Redis dump format
- ✅ **Compressed RDB** (`.rdb.gz`) - Compressed RDB files

### How It Works
- Copies `.rdb` file to Redis data directory
- Handles gzip compression automatically
- Stops Redis before restore, starts after

### Example Commands That Work
```bash
# Standard RDB dump
redis-cli SAVE
cp /var/lib/redis/dump.rdb backup.rdb

# Compressed RDB
redis-cli SAVE
gzip /var/lib/redis/dump.rdb
cp /var/lib/redis/dump.rdb.gz backup.rdb.gz
```

## Limitations

### Not Currently Supported
- ❌ PostgreSQL custom format archives (`pg_dump -F c`)
- ❌ PostgreSQL directory format (`pg_dump -F d`)
- ❌ MySQL binary logs
- ❌ MongoDB oplog dumps
- ❌ Redis AOF (Append-Only File) format
- ❌ Compressed formats other than `.gz` (`.bz2`, `.zip` partially supported)

### Why These Work
The restore logic uses **standard database restore commands** that are designed to work with dumps created by their corresponding dump tools. As long as:
1. The dump is in a standard format (SQL, directory structure, etc.)
2. The dump was created with official database tools
3. The dump format matches what the restore command expects

Then the restore should work, regardless of:
- Which flags were used to create the dump
- Whether it's compressed or not
- Whether it contains one or multiple databases
- The specific backup command used

## Best Practices

1. **Use standard dump commands** - Stick to official tools (`mysqldump`, `pg_dump`, `mongodump`, etc.)
2. **Test your dumps** - Verify dumps can be restored before relying on them
3. **Document your backup process** - Keep track of which commands/flags you use
4. **Use compression** - `.gz` compression is fully supported and recommended
5. **Single database dumps** - When possible, create single-database dumps for easier restore

## Troubleshooting

If a restore fails:
1. Check the dump file format matches the database engine
2. Verify the dump file is not corrupted
3. Ensure you have proper permissions
4. Check the restore logs in `/var/log/mirrorvault/`
5. Try restoring manually with the native database command to verify the dump is valid
