#!/bin/bash
# Script to verify MySQL backup integrity and test restore functionality

echo "=== MirrorVault Backup Verification Script ==="
echo ""

BACKUP_DIR="/var/backups/mirrorvault/mysql"
TEST_DB_PREFIX="test_restore_"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to verify backup file
verify_backup_file() {
    local backup_file=$1
    local db_name=$2
    
    echo -n "Checking $backup_file... "
    
    # Check if file exists and is readable
    if [ ! -f "$backup_file" ]; then
        echo -e "${RED}FAILED - File not found${NC}"
        return 1
    fi
    
    # Check if file is not empty
    if [ ! -s "$backup_file" ]; then
        echo -e "${RED}FAILED - File is empty${NC}"
        return 1
    fi
    
    # Check if it's a valid MariaDB/MySQL dump (check for dump header)
    if head -n 5 "$backup_file" | grep -qiE "(MariaDB dump|MySQL dump|mysqldump)" 2>/dev/null; then
        # Valid dump file - check if it has tables (empty databases are OK)
        if grep -qiE "^CREATE TABLE|^DROP TABLE IF EXISTS" "$backup_file" 2>/dev/null; then
            echo -e "${GREEN}OK (with tables)${NC}"
        else
            echo -e "${GREEN}OK (empty database)${NC}"
        fi
        return 0
    fi
    
    # Fallback: check for SQL patterns
    if ! grep -qE "(CREATE|INSERT|DROP|LOCK|UNLOCK|/\*|-- MySQL|mysqldump|MariaDB)" "$backup_file" 2>/dev/null; then
        # Try case-insensitive
        if ! grep -qiE "(create|insert|drop|lock|unlock|mariadb|mysql)" "$backup_file" 2>/dev/null; then
            echo -e "${YELLOW}WARNING - File may not contain valid SQL${NC}"
            return 2
        fi
    fi
    
    echo -e "${GREEN}OK${NC}"
    return 0
    
    echo -e "${GREEN}OK (basic check)${NC}"
    return 0
}

# Function to test restore
test_restore() {
    local backup_file=$1
    local db_name=$2
    local test_db="${TEST_DB_PREFIX}${db_name}_$$"
    
    echo -n "Testing restore of $db_name... "
    
    # Create test database
    if ! sudo mysql -u root -e "CREATE DATABASE IF NOT EXISTS $test_db;" 2>/dev/null; then
        echo -e "${RED}FAILED - Cannot create test database${NC}"
        return 1
    fi
    
    # Try to restore
    if sudo mysql -u root "$test_db" < "$backup_file" 2>/dev/null; then
        # Check if tables were created
        table_count=$(sudo mysql -u root -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$test_db';" 2>/dev/null)
        
        if [ "$table_count" -gt 0 ]; then
            echo -e "${GREEN}OK (restored $table_count tables)${NC}"
            # Clean up test database
            sudo mysql -u root -e "DROP DATABASE IF EXISTS $test_db;" 2>/dev/null
            return 0
        else
            echo -e "${YELLOW}WARNING - Restored but no tables found${NC}"
            sudo mysql -u root -e "DROP DATABASE IF EXISTS $test_db;" 2>/dev/null
            return 2
        fi
    else
        echo -e "${RED}FAILED - Restore failed${NC}"
        sudo mysql -u root -e "DROP DATABASE IF EXISTS $test_db;" 2>/dev/null
        return 1
    fi
}

# Main verification
echo "1. Verifying backup file integrity..."
echo "----------------------------------------"

failed_files=()
warning_files=()

for backup_file in "$BACKUP_DIR"/*.sql; do
    if [ -f "$backup_file" ]; then
        db_name=$(basename "$backup_file" | sed 's/_2026-.*\.sql$//')
        verify_backup_file "$backup_file" "$db_name"
        result=$?
        if [ $result -eq 1 ]; then
            failed_files+=("$backup_file")
        elif [ $result -eq 2 ]; then
            warning_files+=("$backup_file")
        fi
    fi
done

echo ""
echo "2. Testing restore functionality (sample of 3 databases)..."
echo "----------------------------------------"

# Test restore on a few databases (to avoid taking too long)
test_count=0
for backup_file in "$BACKUP_DIR"/*.sql; do
    if [ -f "$backup_file" ] && [ $test_count -lt 3 ]; then
        db_name=$(basename "$backup_file" | sed 's/_2026-.*\.sql$//')
        # Skip very large files for quick test
        file_size=$(stat -f%z "$backup_file" 2>/dev/null || stat -c%s "$backup_file" 2>/dev/null)
        if [ "$file_size" -lt 100000000 ]; then  # Only test files < 100MB
            test_restore "$backup_file" "$db_name"
            test_count=$((test_count + 1))
        fi
    fi
done

echo ""
echo "3. Summary"
echo "----------------------------------------"
echo "Total backup files: $(ls -1 "$BACKUP_DIR"/*.sql 2>/dev/null | wc -l)"
echo "Failed verifications: ${#failed_files[@]}"
echo "Warnings: ${#warning_files[@]}"

if [ ${#failed_files[@]} -gt 0 ]; then
    echo -e "${RED}Failed files:${NC}"
    for file in "${failed_files[@]}"; do
        echo "  - $file"
    done
fi

if [ ${#warning_files[@]} -gt 0 ]; then
    echo -e "${YELLOW}Warning files:${NC}"
    for file in "${warning_files[@]}"; do
        echo "  - $file"
    done
fi

echo ""
echo "=== Verification Complete ==="
