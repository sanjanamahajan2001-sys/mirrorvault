# MirrorVault

**Secure Database Backup Agent**

MirrorVault is a comprehensive database backup and restore solution that supports multiple database engines with an intuitive terminal user interface (TUI) and automated scheduling capabilities.

## Features

- 🔍 **Database Discovery**: Automatically scans and detects available database engines
- 💾 **Multi-Engine Support**: MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL
- 📅 **Scheduled Backups**: Automatic daily backups via systemd timers or cron
- 🔄 **Safe Restore**: Pre-restore backups and automatic rollback on failure
- 🎨 **Interactive TUI**: Beautiful terminal interface for all operations
- ☁️ **Google Drive Backups**: Optional Drive uploads for manual backups
- 📊 **Restore History**: Track all restore operations with detailed logs
- 🧹 **Automatic Cleanup**: 14-day backup retention with automatic cleanup
- 🔐 **Secure**: Handles authentication securely for protected databases

## Quick Start

### Installation

```bash
# Build from source
go build -o mirrorvault cmd/mirrorvault/main.go

# Install system-wide
sudo cp mirrorvault /usr/local/bin/mirrorvault
sudo chmod +x /usr/local/bin/mirrorvault
```

### Basic Usage

```bash
# Scan for databases
mirrorvault scan

# Create backup
mirrorvault backup

# Restore database
mirrorvault restore

# Schedule daily backup
mirrorvault schedule-daily
```

## Documentation

- **[USER_GUIDE.md](USER_GUIDE.md)** - Complete user guide with all commands and functionality
- **[DUMP_FORMATS_GUIDE.md](DUMP_FORMATS_GUIDE.md)** - Supported dump formats and backup/restore commands
- **[DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md)** - Developer documentation, code structure, and build instructions

## Supported Database Engines

| Engine | Backup Format | Restore Format | Authentication |
|--------|--------------|----------------|----------------|
| MySQL | SQL (`.sql`) | SQL dumps | Optional |
| PostgreSQL | SQL (`.sql`) | SQL dumps | Required |
| MongoDB | Directory/Archive | Directory/Archive | Optional |
| Redis | RDB (`.rdb`) | RDB files | Not required |
| SQLite | DB/SQL | DB files/SQL dumps | Not required |
| MSSQL | Backup (`.bak`) | Backup files | Required |

## Requirements

- Linux system with systemd or cron
- Go 1.24+ (for building from source)
- Database engines installed (MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL)
- Sudo/root access for backup and restore operations

## Project Structure

```
mirrorvault/
├── cmd/mirrorvault/     # Application entry point
├── internal/            # Internal packages
│   ├── analyse/        # Database detection
│   ├── backup/         # Backup functionality
│   ├── restore/         # Restore functionality
│   ├── schedule/       # Scheduled backups
│   └── output/tui/      # Terminal UI
├── pkg/model/           # Shared models
└── docs/               # Documentation
```

## License

[Add your license here]

## Contributing

See [DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md) for development setup and contribution guidelines.

## Support

For issues, questions, or contributions, please refer to the documentation:
- User questions: [USER_GUIDE.md](USER_GUIDE.md)
- Format questions: [DUMP_FORMATS_GUIDE.md](DUMP_FORMATS_GUIDE.md)
- Development questions: [DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md)
