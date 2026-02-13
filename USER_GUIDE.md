# MirrorVault User Guide

Complete guide to using MirrorVault - Secure Database Backup Agent.

## Table of Contents

1. [Installation](#installation)
2. [Basic Commands](#basic-commands)
3. [Backup Operations](#backup-operations)
4. [Restore Operations](#restore-operations)
5. [Scheduled Backups](#scheduled-backups)
6. [Cleanup Operations](#cleanup-operations)
7. [Troubleshooting](#troubleshooting)

---

## Installation

### Prerequisites

- Linux system with systemd or cron
- Go 1.21 or later (for building from source)
- Database engines installed (MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL)
- Sudo/root access for backup and restore operations

### Build from Source

```bash
# Clone the repository
git clone <repository-url>
cd mirrorvault

# Build the binary
go build -o mirrorvault cmd/mirrorvault/main.go

# Install system-wide (requires sudo)
sudo cp mirrorvault /usr/local/bin/mirrorvault
sudo chmod +x /usr/local/bin/mirrorvault
```

### Verify Installation

```bash
mirrorvault --version
```

---

## Basic Commands

### Scan Databases

Scan your system to discover all available databases:

```bash
mirrorvault scan
```

**Interactive Mode:**
- Navigate with ↑/↓ arrow keys
- Press Enter to proceed to backup selection
- Press Ctrl+C to exit

**Output:**
- Lists all detected database engines
- Shows running status for each engine
- Displays available databases per engine
- Indicates if authentication is required

---

## Backup Operations

### Interactive Backup

Start an interactive backup session:

```bash
mirrorvault backup
```

**Workflow:**
1. **Select Engine**: Choose database engine (MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL)
2. **Select Databases**: Select one or more databases to backup
3. **Confirm**: Review backup plan and confirm
4. **Execute**: Monitor backup progress

**Features:**
- Real-time progress indicators
- Automatic backup file naming with timestamps
- Backup location: `/var/backups/mirrorvault/<engine>/`
- Supports authentication for protected databases
- Validates backup output before reporting success

### Backup Locations

- **Manual Backups**: `/var/backups/mirrorvault/<engine>/`
- **Scheduled Backups**: `/var/backups/mirrorvault/daily-backups/<engine>/`

### Google Drive Backups (Manual Only)

MirrorVault can upload manual backups to Google Drive **in addition to local storage**. Scheduled backups remain local-only.

#### One-Time Setup (Server-First)
1. **Create Google OAuth credentials** (admin step):
   - Go to Google Cloud Console → APIs & Services.
   - Enable **Google Drive API**.
   - Create **OAuth consent screen** (External).
   - Create **OAuth Client ID (TV and Limited Input)** for **Device flow** (recommended for servers).
   - Optional: **OAuth Client ID (Desktop)** for browser redirect (desktop only).
2. **Connect Drive in the TUI**:
   - Run `mirrorvault backup`.
   - On the **Google Drive Backup Setup** screen, press **C** to connect.
   - Choose **Device flow (recommended for servers)**.
   - Open the displayed URL, enter the device code, and approve access.
   - You’ll see **Connected securely** when complete.
3. **Choose a folder**:
   - Pick an existing folder **or** create a new one.
   - Account free space and folder size are shown at the top.

#### Connection Persistence
- Connection is saved on disk and reused automatically.
- Stored locations:
  - `/var/lib/mirrorvault/drive_config.json`
  - `~/.config/mirrorvault/drive_config.json`
- You only need to reconnect if you **disconnect** or **switch accounts**.

#### Drive Setup Controls (TUI)
- **C**: Connect / reconnect
- **D**: Disable Drive uploads (keeps connection)
- **E**: Enable Drive uploads
- **X**: Disconnect (clears tokens; requires reconnection)
- **F**: Choose folder

#### Backup Flow with Drive Enabled
- Backup runs locally first.
- MirrorVault checks Drive free space (requires **≥ 2×** backup size).
- If space is sufficient, the backup uploads to Drive.
- If space is insufficient, upload is skipped and local backup is preserved.
- **No retention policy** is applied to Drive backups.

**Note:** Device flow is limited to files/folders created by MirrorVault (`drive.file`). Browser redirect uses full Drive access (`drive`) and is intended for desktop use only.

### Restore from Google Drive
1. Run `mirrorvault restore`.
2. Select engine → database.
3. Choose **Google Drive** as source.
4. Pick a backup file from Drive.
5. MirrorVault downloads it and proceeds with restore.

### Backup File Naming

Format: `<database_name>_YYYY-MM-DD.sql` (or appropriate extension)

Examples:
- `app_db_2026-01-12.sql`
- `users_db_2026-01-12.sql`
- `dump.rdb_2026-01-12.rdb`

---

### Non-Interactive Backup (CLI)

Use CLI flags to run backups without the TUI:

```bash
# Single database
mirrorvault backup --engine MySQL --db app_db --password-file /path/to/password

# All databases for an engine
mirrorvault backup --engine PostgreSQL --all-dbs --password-file /path/to/password
```

## Restore Operations

### Interactive Restore

Restore a database from a backup file:

```bash
mirrorvault restore
```

**Workflow:**
1. **Select Engine**: Choose the database engine
2. **Select Database**: Choose target database to restore
3. **Enter Dump Path**: Provide full path to backup file
   - Press F1 to automatically use the latest backup
4. **Confirm**: Review restore plan and current database stats
5. **Execute**: Monitor restore progress

**Safety Features:**
- **Pre-restore Backup**: Automatically creates backup before restore
- **Validation**: Validates dump file format and compatibility
- **Rollback**: Automatically rolls back on failure
- **Progress Tracking**: Real-time progress with detailed logs

### Non-Interactive Restore (CLI)

Use CLI flags to run restore without the TUI:

```bash
mirrorvault restore --engine MySQL --db app_db --dump-path /path/to/app_db.sql --password-file /path/to/password
```

### Restore History

View all previous restore operations:

```bash
mirrorvault restore-history
```

**Features:**
- Lists all restore operations with timestamps
- Shows success/failure status
- Displays restore details (engine, database, dump path)
- Shows error messages for failed restores
- Scrollable interface (↑/k up, ↓/j down)

**Navigation:**
- Scroll: ↑/k (up), ↓/j (down)
- Exit: Ctrl+C or Enter

---

## Scheduled Backups

### Create Scheduled Backup

Schedule automatic daily backups:

```bash
mirrorvault schedule-daily
```

**Workflow:**
1. **Select Engine**: Choose database engine
2. **Select Databases**: Select databases to backup daily
3. **Set Time**: Enter backup time in HH:MM format (24-hour)
4. **Confirm**: Review schedule and confirm
5. **Authentication**: Provide password if required

**Features:**
- Daily automatic backups at specified time
- Scheduler backend: systemd (preferred) or cron
- Password stored securely in `/var/lib/mirrorvault/secrets/`
- Automatic cleanup of old backups (14 days retention)

### List Scheduled Backups

View all active schedules:

```bash
mirrorvault list-schedules
```

**Interactive Mode:**
- Navigate with ↑/↓ arrow keys
- Press E to edit schedule time
- Press D to delete schedule
- Press Enter or Ctrl+C to exit

### Edit Schedule Time

In the schedule list view:
1. Navigate to the schedule you want to edit
2. Press E
3. Enter new time in HH:MM format
4. Confirm changes

### Delete Schedule

**Delete Single Schedule:**
```bash
mirrorvault delete-schedule <timer-name>
```

**Delete All Schedules:**
```bash
mirrorvault delete-schedule --all
```

Or use the interactive list:
1. Navigate to the schedule
2. Press D
3. Confirm deletion

### Fix Existing Timers

If scheduled backups aren't running on systemd, fix timer dependencies:

```bash
sudo mirrorvault fix-timers
```

This command:
- Removes incorrect dependencies from timer units
- Reloads systemd
- Restarts all timers

### Verify Timer Status

Check if timers are active:

```bash
systemctl list-timers --all | grep mirrorvault
```

Check specific timer:

```bash
systemctl status mirrorvault-<timer-name>
```

View backup logs:

```bash
sudo journalctl -u mirrorvault-<service-name> -n 50
```

---

## Cleanup Operations

### Automatic Cleanup

Cleanup runs automatically daily at 01:00 UTC via systemd timer.

**What it does:**
- Removes backups older than 14 days
- Cleans both manual and scheduled backup directories
- Maintains disk space automatically

### Manual Cleanup

Run cleanup manually:

```bash
sudo mirrorvault cleanup
```

**Cleanup Rules:**
- Retention period: 14 days
- Applies to: `/var/backups/mirrorvault/` and subdirectories
- Preserves backups newer than 14 days

---

## Command Reference

### All Available Commands

```bash
mirrorvault scan                    # Scan for databases
mirrorvault backup                  # Interactive backup
mirrorvault restore                 # Interactive restore
mirrorvault restore-history         # View restore history
mirrorvault schedule-daily          # Schedule daily backups
mirrorvault list-schedules          # List all schedules
mirrorvault delete-schedule <name>  # Delete a schedule
mirrorvault delete-schedule --all   # Delete all schedules
mirrorvault cleanup                 # Manual cleanup
mirrorvault install-logrotate       # Install logrotate config
mirrorvault --version               # Show version
```

---

## Keyboard Shortcuts

### General Navigation
- **↑/k**: Move up
- **↓/j**: Move down
- **Enter**: Select/Confirm
- **Esc**: Go back
- **Ctrl+C**: Exit

### Restore History
- **↑/k**: Scroll up
- **↓/j**: Scroll down
- **Enter**: Exit
- **Ctrl+C**: Exit

### Schedule List
- **↑/↓**: Navigate
- **E**: Edit schedule time
- **D**: Delete schedule
- **Enter/Ctrl+C**: Exit

### Restore Dump Path
- **F1**: Auto-fill latest backup path
- **Enter**: Confirm
- **Esc**: Go back

---

## File Locations

### Backup Files
- Manual: `/var/backups/mirrorvault/<engine>/`
- Scheduled: `/var/backups/mirrorvault/daily-backups/<engine>/`

### Configuration
- Schedules: `/var/lib/mirrorvault/schedules.json`
- Schedule secrets: `/var/lib/mirrorvault/secrets/`
- Systemd Units: `/etc/systemd/system/mirrorvault-*.timer`
- Systemd Services: `/etc/systemd/system/mirrorvault-*.service`
 - Cron jobs (if systemd not available): `crontab -l`

### Logs
- Restore Logs: `/var/log/mirrorvault/restore_*.log`
- Systemd Logs: `journalctl -u mirrorvault-<service-name>`
 - Log rotation config: `/etc/logrotate.d/mirrorvault` (install via `mirrorvault install-logrotate`)

### Pre-restore Backups
- Location: `/var/backups/mirrorvault/restore-backups/<engine>/`

---

## Configuration (Environment Variables)

### Database Connection Overrides
Use these to override default host/user/port settings:

```
MV_MYSQL_HOST, MV_MYSQL_PORT, MV_MYSQL_USER
MV_POSTGRES_HOST, MV_POSTGRES_PORT, MV_POSTGRES_USER
MV_MONGODB_HOST, MV_MONGODB_PORT, MV_MONGODB_USER, MV_MONGODB_AUTHDB
MV_REDIS_HOST, MV_REDIS_PORT
MV_MSSQL_HOST, MV_MSSQL_PORT, MV_MSSQL_USER
```

### SQLite Scan Control
```
MV_SQLITE_SCAN_ROOTS=/home:/data
MV_SQLITE_MAX_DEPTH=4
```

### Strict Backup Validation
```
MV_STRICT_VALIDATE=true
```
When enabled, MirrorVault runs extra validation steps (where supported), such as
`mongorestore --dryRun`, `redis-check-rdb`, and `RESTORE VERIFYONLY` for MSSQL.

### Google Drive (Optional)
```
MV_GDRIVE_CLIENT_ID
MV_GDRIVE_CLIENT_SECRET
MV_GDRIVE_REDIRECT_HOST
```
- Use these to provide OAuth client credentials via environment variables.
- For servers, **Device flow** is recommended and only requires the **TV and Limited Input** client.
- `MV_GDRIVE_REDIRECT_HOST` is only used for browser redirect flow (desktop use).

### Backup Format Options
```
MV_BACKUP_COMPRESSION=gz|bz2|zip
MV_BACKUP_KEEP_SOURCE=true
MV_POSTGRES_BACKUP_FORMAT=plain|custom|directory
MV_MONGO_BACKUP_FORMAT=directory|archive|archive.gz
MV_SQLITE_BACKUP_MODE=dump|backup
MV_REDIS_BACKUP_MODE=rdb|aof
MV_MSSQL_BACKUP_COMPRESSION=true
```

---

## Supported Database Engines

### MySQL
- **Authentication**: Optional (root user)
- **Backup Format**: SQL dump (`.sql`)
- **Compression**: Supported (`.sql.gz`, `.sql.bz2`, `.sql.zip`)

### PostgreSQL
- **Authentication**: Required (postgres user)
- **Backup Format**: SQL dump (`.sql`), custom (`.dump`), directory
- **Compression**: Supported (`.sql.gz`, `.sql.bz2`, `.sql.zip`)

### MongoDB
- **Authentication**: Optional
- **Backup Format**: Directory dump or archive
- **Compression**: Supported (`.gz`, `.bz2`, `.zip`, `.tar.gz`)

### Redis
- **Authentication**: Not required
- **Backup Format**: RDB file (`.rdb`) or AOF (`.aof`)
- **Compression**: Supported (`.rdb.gz`, `.rdb.bz2`, `.rdb.zip`)

### SQLite
- **Authentication**: Not required
- **Backup Format**: Database file (`.db`) or SQL dump (`.sql`)
- **Compression**: Supported (`.db.gz`, `.db.bz2`, `.db.zip`, `.sql.gz`, `.sql.bz2`, `.sql.zip`)

### MSSQL
- **Authentication**: Required (SQL Server credentials)
- **Backup Format**: SQL Server backup file (`.bak`)
- **Compression**: Supported (`.bak.gz`, `.bak.bz2`, `.bak.zip`)

---

## Troubleshooting

### Backup Issues

**Problem**: Backup fails with permission error
```bash
# Check file permissions
ls -l /var/backups/mirrorvault/

# Ensure directory exists and is writable
sudo mkdir -p /var/backups/mirrorvault
sudo chmod 755 /var/backups/mirrorvault
```

**Problem**: Authentication fails
- Ensure you're using the correct password
- For MySQL: Check if root user requires password
- For PostgreSQL: Password is required

**Problem**: Database not found
- Verify database exists: `mirrorvault scan`
- Check database name spelling
- Ensure database engine is running

### Restore Issues

**Problem**: Restore fails - dump file not found
- Verify full path to dump file
- Check file permissions: `ls -l /path/to/dump.sql`
- Use F1 in restore interface to auto-fill latest backup

**Problem**: Restore fails - format incompatible
- Ensure dump format matches database engine
- MySQL dumps work with MySQL only
- PostgreSQL dumps work with PostgreSQL only
- See DUMP_FORMATS_GUIDE.md for supported formats

**Problem**: Restore shows 0 tables after completion
- Check if dump file contains the target database
- For multi-database dumps, ensure target DB exists in dump
- Verify dump file is not corrupted

### Scheduled Backup Issues

**Problem**: Scheduled backups not running
```bash
# Check timer status
systemctl list-timers --all | grep mirrorvault

# Fix timer dependencies
sudo mirrorvault fix-timers

# Check service logs
sudo journalctl -u mirrorvault-<service-name> -n 50
```

If using cron (no systemd), verify the crontab:
```bash
sudo crontab -l
```

**Problem**: Timer shows as inactive
```bash
# Check timer status
systemctl status mirrorvault-<timer-name>

# Restart timer
sudo systemctl restart mirrorvault-<timer-name>

# Enable timer
sudo systemctl enable mirrorvault-<timer-name>
```

**Problem**: Backup runs but creates empty files
- Check database engine is running
- Verify database exists
- Check authentication credentials
- Review service logs for errors

### Google Drive Issues

**Problem**: `invalid_client` or `Invalid client type`
- Device flow requires **OAuth Client ID: TV and Limited Input**
- Browser flow requires **OAuth Client ID: Desktop (loopback)**
- Re-enter the correct Client ID/Secret in the TUI and reconnect

**Problem**: Drive shows “Not connected” after a previous successful connect
- Ensure the config file is readable:
  - `/var/lib/mirrorvault/drive_config.json`
  - `~/.config/mirrorvault/drive_config.json`
- Run MirrorVault as the same user that created the connection, or copy the config to the correct path

### General Issues

**Problem**: Command not found
```bash
# Verify installation
which mirrorvault

# Rebuild and install
go build -o mirrorvault cmd/mirrorvault/main.go
sudo cp mirrorvault /usr/local/bin/mirrorvault
```

**Problem**: Permission denied
- Most operations require sudo/root access
- Ensure you have sudo privileges
- Check file/directory permissions

---

## Best Practices

1. **Regular Backups**: Schedule daily backups for critical databases
2. **Test Restores**: Periodically test restore operations
3. **Monitor Logs**: Check logs regularly for issues
4. **Verify Backups**: Ensure backup files are created and not empty
5. **Cleanup**: Let automatic cleanup manage old backups (14-day retention)
6. **Documentation**: Keep track of backup schedules and locations
7. **Security**: Use strong passwords for authenticated databases
8. **Backup Verification**: Verify backup file sizes and timestamps
9. **Strict Validation (Optional)**: Set `MV_STRICT_VALIDATE=true` for deeper checks

---

## Getting Help

### View Logs
```bash
# Restore logs
ls -lh /var/log/mirrorvault/

# Systemd service logs
sudo journalctl -u mirrorvault-<service-name> -n 100

# Systemd timer logs
sudo journalctl -u mirrorvault-<timer-name> -n 100
```

### Check System Status
```bash
# All timers
systemctl list-timers --all | grep mirrorvault

# Specific timer
systemctl status mirrorvault-<timer-name>

# Service status
systemctl status mirrorvault-<service-name>
```

### Verify Backups
```bash
# List backup files
ls -lh /var/backups/mirrorvault/*/

# Check backup sizes
du -sh /var/backups/mirrorvault/*/

# Verify latest backups
find /var/backups/mirrorvault -type f -mtime -1 -ls
```

---

## Examples

### Complete Backup Workflow

```bash
# 1. Scan for databases
mirrorvault scan

# 2. Create backup
mirrorvault backup
# Select engine → Select databases → Confirm

# 3. Verify backup was created
ls -lh /var/backups/mirrorvault/mysql/
```

### Complete Restore Workflow

```bash
# 1. View restore history
mirrorvault restore-history

# 2. Restore database
mirrorvault restore
# Select engine → Select database → Enter dump path (or F1) → Confirm

# 3. Verify restore
# Check database tables and data
```

### Schedule Daily Backup

```bash
# 1. Create schedule
mirrorvault schedule-daily
# Select engine → Select databases → Enter time (e.g., 02:00) → Confirm

# 2. Verify schedule
mirrorvault list-schedules

# 3. Check timer status
systemctl list-timers --all | grep mirrorvault
```

---

For detailed information about dump formats and backup/restore commands, see [DUMP_FORMATS_GUIDE.md](DUMP_FORMATS_GUIDE.md).
