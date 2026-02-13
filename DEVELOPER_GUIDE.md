# MirrorVault Developer Guide

Complete guide for developers and engineers working on MirrorVault.

## Table of Contents

1. [Project Overview](#project-overview)
2. [Prerequisites](#prerequisites)
3. [Project Structure](#project-structure)
4. [Building the Project](#building-the-project)
5. [Architecture](#architecture)
6. [Key Components](#key-components)
7. [Development Workflow](#development-workflow)
8. [Code Organization](#code-organization)
9. [Testing](#testing)
10. [Contributing](#contributing)

---

## Project Overview

MirrorVault is a secure database backup agent written in Go that provides:
- Interactive TUI for backup and restore operations
- Support for multiple database engines (MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL)
- Scheduled automatic backups via systemd timers or cron
- Safe restore operations with automatic rollback
- Backup cleanup and retention management

### Technology Stack

- **Language**: Go 1.24+
- **TUI Framework**: Bubble Tea (charmbracelet/bubbletea)
- **Styling**: Lipgloss (charmbracelet/lipgloss)
- **System Integration**: systemd or cron (for scheduled backups)

---

## Prerequisites

### Required Software

- **Go**: Version 1.24 or later
- **Git**: For version control
- **Linux System**: With systemd or cron (for scheduled backups)
- **Database Engines**: MySQL, PostgreSQL, MongoDB, Redis, SQLite, MSSQL (for testing)

### Development Tools

- **Code Editor**: VS Code, GoLand, or any Go-compatible editor
- **Terminal**: For running and testing
- **Sudo Access**: Required for backup/restore operations

### Verify Prerequisites

```bash
# Check Go version
go version

# Check Git
git --version

# Check systemd (Linux)
systemctl --version
```

---

## Project Structure

```
mirrorvault/
├── cmd/
│   └── mirrorvault/
│       └── main.go                 # Application entry point
├── internal/
│   ├── analyse/                    # Database detection and scanning
│   │   ├── detect/                 # Engine-specific detection
│   │   │   ├── common.go
│   │   │   ├── mysql.go
│   │   │   ├── postgres.go
│   │   │   ├── mongodb.go
│   │   │   ├── redis.go
│   │   │   ├── sqlite.go
│   │   │   └── mssql.go
│   │   └── scan.go                 # Main scanning logic
│   ├── backup/                      # Backup functionality
│   │   ├── credentials/             # Authentication handling
│   │   │   ├── context.go
│   │   │   └── prompt.go
│   │   ├── execute/                 # Backup execution
│   │   │   ├── executor.go         # Main backup orchestrator
│   │   │   ├── common.go            # Shared backup utilities
│   │   │   ├── mysql.go
│   │   │   ├── postgres.go
│   │   │   ├── mongodb.go
│   │   │   ├── redis.go
│   │   │   ├── sqlite.go
│   │   │   └── mssql.go
│   │   └── plan/                    # Backup planning
│   │       ├── builder.go
│   │       └── plan.go
│   ├── restore/                     # Restore functionality
│   │   ├── analyze/                 # Database analysis
│   │   │   └── analyzer.go
│   │   ├── execute/                 # Restore execution
│   │   │   ├── executor.go         # Main restore orchestrator
│   │   │   ├── backup.go           # Pre-restore backup
│   │   │   ├── restore.go          # Restore coordination
│   │   │   ├── mysql.go
│   │   │   ├── postgres.go
│   │   │   ├── mongodb.go
│   │   │   ├── redis.go
│   │   │   └── sqlite.go
│   │   ├── history/                 # Restore history
│   │   │   └── parser.go
│   │   ├── log/                     # Restore logging
│   │   │   └── logger.go
│   │   ├── plan/                    # Restore planning
│   │   │   ├── builder.go
│   │   │   └── plan.go
│   │   ├── validate/                # Dump validation
│   │   │   ├── validator.go
│   │   │   └── dump_analyzer.go
│   │   └── backup_finder.go         # Finding latest backups
│   ├── schedule/                    # Scheduled backups
│   │   ├── schedule.go              # Schedule management
│   │   └── cleanup.go               # Backup cleanup
│   ├── output/                      # Output handling
│   │   ├── tui/                     # Terminal UI
│   │   │   ├── model.go             # TUI model/state
│   │   │   ├── tui.go               # TUI main logic
│   │   │   ├── run.go               # TUI runner
│   │   │   ├── styles.go            # UI styling
│   │   │   ├── render_helpers.go   # Rendering utilities
│   │   │   ├── scan_view.go        # Scan view
│   │   │   ├── select_*.go          # Selection views
│   │   │   ├── view_*.go            # Various views
│   │   │   └── *_bridge.go         # Bridge components
│   │   ├── plain.go                 # Plain text output
│   │   └── prompt.go                # User prompts
│   ├── version/                     # Version information
│   │   └── version.go
│   └── logrotate/                   # Log rotation installer
│       └── install.go
├── pkg/
│   └── model/                        # Shared models
│       └── database.go
├── go.mod                           # Go module definition
├── go.sum                           # Go module checksums
├── README.md                        # Project readme
├── USER_GUIDE.md                    # User documentation
├── DUMP_FORMATS_GUIDE.md            # Dump formats documentation
└── DEVELOPER_GUIDE.md               # This file
```

---

## Building the Project

### Basic Build

```bash
# Build the binary
go build -o mirrorvault cmd/mirrorvault/main.go

# Build with optimizations
go build -ldflags="-s -w" -o mirrorvault cmd/mirrorvault/main.go
```

### Build with Version Information

```bash
# Set version variables at build time
go build -ldflags "\
  -X 'mirrorvault/internal/version.Version=1.0.0' \
  -X 'mirrorvault/internal/version.Commit=$(git rev-parse --short HEAD)' \
  -X 'mirrorvault/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o mirrorvault cmd/mirrorvault/main.go
```

### Cross-Platform Build

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o mirrorvault-linux-amd64 cmd/mirrorvault/main.go

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o mirrorvault-windows-amd64.exe cmd/mirrorvault/main.go

# Build for macOS
GOOS=darwin GOARCH=amd64 go build -o mirrorvault-darwin-amd64 cmd/mirrorvault/main.go
```

### Install System-Wide

```bash
# Build and install
go build -o mirrorvault cmd/mirrorvault/main.go
sudo cp mirrorvault /usr/local/bin/mirrorvault
sudo chmod +x /usr/local/bin/mirrorvault
```

### Development Build Script

Create a `build.sh` script:

```bash
#!/bin/bash
VERSION=${1:-dev}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build -ldflags "\
  -X 'mirrorvault/internal/version.Version=$VERSION' \
  -X 'mirrorvault/internal/version.Commit=$COMMIT' \
  -X 'mirrorvault/internal/version.BuildTime=$BUILD_TIME'" \
  -o mirrorvault cmd/mirrorvault/main.go

echo "Built mirrorvault version $VERSION (commit: $COMMIT)"
```

---

## Architecture

### High-Level Architecture

```
┌─────────────────┐
│   User Input    │
│   (TUI/CLI)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Command Router │
│   (main.go)     │
└────────┬────────┘
         │
    ┌────┴────┬──────────┬──────────┐
    │         │          │          │
    ▼         ▼          ▼          ▼
┌──────┐ ┌────────┐ ┌────────┐ ┌────────┐
│ Scan │ │ Backup │ │Restore │ │Schedule│
└──┬───┘ └───┬────┘ └───┬────┘ └───┬────┘
   │         │          │          │
   ▼         ▼          ▼          ▼
┌─────────────────────────────────────┐
│     Database Engine Detectors       │
│  (MySQL, PostgreSQL, MongoDB, etc.) │
└─────────────────────────────────────┘
```

### Component Interaction

1. **Scan Phase**: Detects available database engines and databases
2. **Plan Phase**: Builds backup/restore plan based on user selection
3. **Execute Phase**: Performs actual backup/restore operations
4. **Schedule Phase**: Creates systemd timers for automatic backups

### Data Flow

```
User Selection → Plan Builder → Executor → Database Engine
                                      ↓
                                 Backup Files
                                      ↓
                                 File System
```

---

## Key Components

### 1. Database Detection (`internal/analyse/`)

**Purpose**: Scans system for available database engines and databases

**Key Files**:
- `scan.go`: Main scanning orchestrator
- `detect/*.go`: Engine-specific detection logic

**How it works**:
- Checks if database commands exist (`mysql`, `psql`, `mongodump`, etc.)
- Executes version commands to detect engines
- Lists databases using engine-specific commands
- Determines authentication requirements

### 2. Backup System (`internal/backup/`)

**Purpose**: Handles backup creation and execution

**Key Components**:
- **Plan Builder**: Creates backup plan from user selection
- **Executor**: Orchestrates backup execution
- **Engine Executors**: Engine-specific backup logic

**Backup Flow**:
1. User selects databases
2. Plan builder creates backup plan
3. Executor runs engine-specific backup commands
4. Backup files saved to `/var/backups/mirrorvault/`

### 3. Restore System (`internal/restore/`)

**Purpose**: Handles database restoration from backup files

**Key Components**:
- **Validator**: Validates dump file format and compatibility
- **Analyzer**: Analyzes database state before/after restore
- **Executor**: Orchestrates restore with rollback support
- **History**: Tracks restore operations

**Restore Flow**:
1. Validate dump file format
2. Create pre-restore backup
3. Drop existing tables/data
4. Restore from dump
5. Analyze results
6. Rollback on failure

### 4. TUI System (`internal/output/tui/`)

**Purpose**: Provides interactive terminal user interface

**Key Components**:
- **Model**: State management (Bubble Tea model)
- **Views**: UI rendering functions
- **Update Functions**: Handle user input and state changes
- **Styles**: UI styling with Lipgloss

**TUI Architecture**:
- Uses Bubble Tea for state management
- View functions render UI based on state
- Update functions handle messages and state transitions
- Supports multiple views (scan, backup, restore, schedule)

### 5. Scheduling System (`internal/schedule/`)

**Purpose**: Manages scheduled automatic backups

**Key Components**:
- **Schedule Manager**: CRUD operations for schedules
- **Scheduler Backends**: systemd timers or cron
- **Cleanup**: Automatic backup cleanup (14-day retention)

**Scheduling Flow**:
1. User creates schedule via TUI
2. System creates systemd timer and service units
3. Systemd triggers backups at scheduled times
4. Cleanup service removes old backups

---

## Development Workflow

### Setting Up Development Environment

```bash
# Clone repository
git clone <repository-url>
cd mirrorvault

# Install dependencies
go mod download

# Verify setup
go build -o mirrorvault cmd/mirrorvault/main.go
./mirrorvault --version
```

### Making Changes

1. **Create Feature Branch**
```bash
git checkout -b feature/your-feature-name
```

2. **Make Changes**
- Follow Go coding standards
- Add comments for public functions
- Update tests if applicable

3. **Test Changes**
```bash
# Build and test
go build -o mirrorvault cmd/mirrorvault/main.go
./mirrorvault scan  # Test your changes
```

4. **Commit Changes**
```bash
git add .
git commit -m "Description of changes"
```

### Code Style Guidelines

- **Formatting**: Use `gofmt` or `go fmt`
- **Linting**: Follow Go best practices
- **Naming**: Use descriptive names, follow Go conventions
- **Comments**: Document public functions and complex logic
- **Error Handling**: Always handle errors explicitly

### Running Locally

```bash
# Build
go build -o mirrorvault cmd/mirrorvault/main.go

# Run scan
./mirrorvault scan

# Run backup (interactive)
./mirrorvault backup

# Run restore (interactive)
./mirrorvault restore
```

---

## Code Organization

### Package Structure

#### `cmd/mirrorvault/`
- **main.go**: Application entry point, command routing

#### `internal/analyse/`
- Database detection and scanning
- Engine-specific detection logic

#### `internal/backup/`
- **plan/**: Backup planning logic
- **execute/**: Backup execution
- **credentials/**: Authentication handling

#### `internal/restore/`
- **plan/**: Restore planning
- **execute/**: Restore execution
- **validate/**: Dump validation
- **analyze/**: Database analysis
- **history/**: Restore history tracking
- **log/**: Restore logging

#### `internal/schedule/`
- Schedule management
- Systemd integration
- Cleanup operations

#### `internal/output/tui/`
- TUI implementation using Bubble Tea
- View rendering
- State management

#### `pkg/model/`
- Shared data models
- Database type definitions

### Adding a New Database Engine

1. **Add Detection** (`internal/analyse/detect/`)
   - Create `engine.go` with detection logic
   - Implement `DetectEngine()` function

2. **Add Backup** (`internal/backup/execute/`)
   - Create `engine.go` with backup logic
   - Implement backup execution

3. **Add Restore** (`internal/restore/execute/`)
   - Create `engine.go` with restore logic
   - Implement restore execution

4. **Update Models** (`pkg/model/`)
   - Add engine to database types if needed

5. **Update TUI** (`internal/output/tui/`)
   - Add engine-specific UI handling if needed

---

## Testing

### Manual Testing

```bash
# Test scan
./mirrorvault scan

# Test backup
./mirrorvault backup

# Test restore
./mirrorvault restore

# Test scheduling
./mirrorvault schedule-daily
```

### Testing Specific Features

```bash
# Test with specific database
./mirrorvault backup
# Select MySQL → Select specific database

# Test restore with specific dump
./mirrorvault restore
# Select engine → Select database → Enter dump path

# Test scheduled backup
./mirrorvault schedule-daily
# Create schedule → Check systemd timer
systemctl status mirrorvault-<timer-name>
```

### Integration Testing

Test the full workflow:

```bash
# 1. Scan
./mirrorvault scan

# 2. Backup
./mirrorvault backup
# Select databases and confirm

# 3. Verify backup
ls -lh /var/backups/mirrorvault/*/

# 4. Restore
./mirrorvault restore
# Select same databases and restore

# 5. Verify restore
# Check database tables and data
```

---

## Contributing

### Development Guidelines

1. **Code Quality**
   - Follow Go best practices
   - Write clear, readable code
   - Add comments for complex logic
   - Handle errors properly

2. **Testing**
   - Test your changes thoroughly
   - Test with multiple database engines
   - Verify edge cases

3. **Documentation**
   - Update relevant documentation
   - Add comments for public APIs
   - Update user guide if adding features

4. **Commit Messages**
   - Use clear, descriptive messages
   - Reference issues if applicable
   - Keep commits focused and atomic

### Pull Request Process

1. Fork the repository
2. Create feature branch
3. Make changes and test
4. Update documentation
5. Submit pull request with description

### Code Review Checklist

- [ ] Code follows Go conventions
- [ ] Error handling is proper
- [ ] Tests pass
- [ ] Documentation updated
- [ ] No breaking changes (or documented)
- [ ] Backward compatible

---

## Build Commands Reference

### Development Build
```bash
go build -o mirrorvault cmd/mirrorvault/main.go
```

### Production Build
```bash
go build -ldflags="-s -w" -o mirrorvault cmd/mirrorvault/main.go
```

### Build with Version
```bash
go build -ldflags "\
  -X 'mirrorvault/internal/version.Version=1.0.0' \
  -X 'mirrorvault/internal/version.Commit=$(git rev-parse --short HEAD)' \
  -X 'mirrorvault/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
  -o mirrorvault cmd/mirrorvault/main.go
```

### Install
```bash
sudo cp mirrorvault /usr/local/bin/mirrorvault
sudo chmod +x /usr/local/bin/mirrorvault
```

### Clean Build
```bash
go clean
go mod tidy
go build -o mirrorvault cmd/mirrorvault/main.go
```

---

## Key Design Decisions

### Why Bubble Tea?

- Provides clean state management
- Handles terminal rendering efficiently
- Supports complex interactive UIs
- Well-maintained and documented

### Why systemd Timers?

- Native Linux integration
- Reliable scheduling
- Automatic restart on failure
- Standard system service

### Why Separate Backup/Restore Directories?

- Clear separation of manual vs scheduled backups
- Easier management and cleanup
- Better organization

### Why Pre-restore Backups?

- Safety mechanism
- Automatic rollback on failure
- User can manually restore if needed

---

## Common Development Tasks

### Adding a New Command

1. Add case in `cmd/mirrorvault/main.go`
2. Create handler function
3. Update help text
4. Test the command

### Modifying TUI Views

1. Locate view function in `internal/output/tui/view_*.go`
2. Modify rendering logic
3. Update state if needed
4. Test in interactive mode

### Adding Engine Support

1. Add detection in `internal/analyse/detect/`
2. Add backup in `internal/backup/execute/`
3. Add restore in `internal/restore/execute/`
4. Update documentation

### Debugging

```bash
# Enable verbose logging (if implemented)
MIRRORVAULT_DEBUG=1 ./mirrorvault backup

# Check systemd logs
sudo journalctl -u mirrorvault-<service> -f

# Check restore logs
tail -f /var/log/mirrorvault/restore_*.log
```

---

## Dependencies

### Core Dependencies

- **bubbletea**: Terminal UI framework
- **lipgloss**: Terminal styling
- **golang.org/x/term**: Terminal utilities

### Managing Dependencies

```bash
# Add dependency
go get <package>

# Update dependencies
go get -u ./...

# Clean up
go mod tidy
```

---

## File Locations (Runtime)

### Configuration
- Schedules: `/var/lib/mirrorvault/schedules.json`
- Systemd Units: `/etc/systemd/system/mirrorvault-*.timer`
 - Schedule secrets: `/var/lib/mirrorvault/secrets/`
 - Cron jobs (if systemd not available): `crontab -l`

### Data
- Backups: `/var/backups/mirrorvault/`
- Scheduled Backups: `/var/backups/mirrorvault/daily-backups/`
- Pre-restore Backups: `/var/backups/mirrorvault/restore-backups/`

### Logs
- Restore Logs: `/var/log/mirrorvault/restore_*.log`
 - Logrotate config: `/etc/logrotate.d/mirrorvault` (install via `mirrorvault install-logrotate`)

---

## Version Information

Version information is set at build time via linker flags:

```go
// internal/version/version.go
var (
    Version   = "dev"
    Commit    = "unknown"
    BuildTime = "unknown"
)
```

Set at build:
```bash
go build -ldflags "-X 'mirrorvault/internal/version.Version=1.0.0'"
```

---

## Troubleshooting Development Issues

### Build Fails

```bash
# Clean and rebuild
go clean
go mod tidy
go build -o mirrorvault cmd/mirrorvault/main.go
```

### Dependencies Issues

```bash
# Update all dependencies
go get -u ./...
go mod tidy
```

### TUI Not Rendering

- Ensure terminal supports ANSI colors
- Check terminal size (minimum 80x24)
- Verify Bubble Tea compatibility

### Systemd Issues

```bash
# Check systemd unit syntax
systemd-analyze verify /etc/systemd/system/mirrorvault-*.timer

# Reload systemd
sudo systemctl daemon-reload
```

---

For user documentation, see [USER_GUIDE.md](USER_GUIDE.md).  
For dump formats information, see [DUMP_FORMATS_GUIDE.md](DUMP_FORMATS_GUIDE.md).
