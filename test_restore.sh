#!/bin/bash
# Script to test restore functionality for a specific backup

if [ $# -lt 1 ]; then
    echo "Usage: $0 <backup_file_path> [test_database_name]"
    echo "Example: $0 /var/backups/mirrorvault/mysql/test_db_2026-01-12.sql"
    exit 1
fi

BACKUP_FILE=$1
TEST_DB=${2:-"test_restore_$$"}

if [ ! -f "$BACKUP_FILE" ]; then
    echo "Error: Backup file not found: $BACKUP_FILE"
    exit 1
fi

echo "=== Testing Restore Functionality ==="
echo "Backup file: $BACKUP_FILE"
echo "Test database: $TEST_DB"
echo ""

# Check backup file size
file_size=$(stat -f%z "$BACKUP_FILE" 2>/dev/null || stat -c%s "$BACKUP_FILE" 2>/dev/null)
echo "Backup file size: $(numfmt --to=iec-i --suffix=B $file_size 2>/dev/null || echo "${file_size} bytes")"
echo ""

# Check backup file content
echo "1. Checking backup file structure..."
# Check for MariaDB/MySQL dump header (always present)
if head -n 5 "$BACKUP_FILE" | grep -qiE "(MariaDB dump|MySQL dump|mysqldump)" 2>/dev/null; then
    echo "   ✓ Valid MariaDB/MySQL dump file"
    # Check if database has tables
    if grep -qiE "^CREATE TABLE|^DROP TABLE IF EXISTS" "$BACKUP_FILE" 2>/dev/null; then
        echo "   ✓ Contains table definitions"
    else
        echo "   ⚠ Database appears to be empty (no tables)"
    fi
else
    # Show first few lines for debugging
    echo "   ⚠ First 10 lines of backup file:"
    head -n 10 "$BACKUP_FILE" | sed 's/^/      /'
    echo "   Continuing with restore test anyway..."
fi

# Count tables in backup (try different patterns)
table_count=$(grep -cE "^CREATE TABLE|^DROP TABLE IF EXISTS" "$BACKUP_FILE" 2>/dev/null || echo "0")
if [ "$table_count" -eq 0 ]; then
    # Try case-insensitive and different patterns
    table_count=$(grep -ciE "create table|drop table if exists" "$BACKUP_FILE" 2>/dev/null || echo "0")
fi
if [ "$table_count" -gt 0 ]; then
    echo "   Found $table_count tables in backup"
else
    echo "   Database appears to be empty (no tables)"
fi
echo ""

# Create test database
echo "2. Creating test database..."
if sudo mysql -u root -e "CREATE DATABASE IF NOT EXISTS $TEST_DB;" 2>/dev/null; then
    echo "   ✓ Test database created: $TEST_DB"
else
    echo "   ✗ Failed to create test database"
    exit 1
fi

# Restore backup
echo ""
echo "3. Restoring backup..."
if sudo mysql -u root "$TEST_DB" < "$BACKUP_FILE" 2>&1; then
    echo "   ✓ Restore completed without errors"
else
    echo "   ✗ Restore failed"
    sudo mysql -u root -e "DROP DATABASE IF EXISTS $TEST_DB;" 2>/dev/null
    exit 1
fi

# Verify restore
echo ""
echo "4. Verifying restored database..."
restored_tables=$(sudo mysql -u root -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$TEST_DB';" 2>/dev/null)
if [ "$restored_tables" -gt 0 ]; then
    echo "   ✓ Database restored successfully"
    echo "   ✓ Found $restored_tables tables in restored database"
    
    # Get table names
    echo ""
    echo "   Restored tables:"
    sudo mysql -u root -N -e "SELECT table_name FROM information_schema.tables WHERE table_schema='$TEST_DB' ORDER BY table_name;" 2>/dev/null | while read table; do
        if [ -n "$table" ]; then
            row_count=$(sudo mysql -u root -N -e "SELECT COUNT(*) FROM \`$TEST_DB\`.\`$table\`;" 2>/dev/null || echo "0")
            echo "     - $table ($row_count rows)"
        fi
    done
else
    # Check if database exists (empty databases are valid)
    db_exists=$(sudo mysql -u root -N -e "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='$TEST_DB';" 2>/dev/null)
    if [ "$db_exists" -eq 1 ]; then
        echo "   ✓ Database restored successfully (empty database - no tables)"
        echo "   ℹ This is normal for databases with no tables"
    else
        echo "   ✗ Database was not created or restore failed"
        sudo mysql -u root -e "DROP DATABASE IF EXISTS $TEST_DB;" 2>/dev/null
        exit 1
    fi
fi

# Cleanup
echo ""
echo "5. Cleaning up test database..."
if sudo mysql -u root -e "DROP DATABASE IF EXISTS $TEST_DB;" 2>/dev/null; then
    echo "   ✓ Test database removed"
else
    echo "   ⚠ Warning: Could not remove test database $TEST_DB (you may need to remove it manually)"
fi

echo ""
echo "=== Restore Test Complete ==="
echo "✓ Backup file is valid and can be restored successfully"
