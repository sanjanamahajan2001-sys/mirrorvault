# Database Dump Commands

## MySQL Dump Commands

### Without Password:
```bash
sudo mysqldump -u root DATABASE_NAME > /home/sanjana/mysql_backup.sql
```

### With Password:
```bash
sudo mysqldump -u root -pPASSWORD DATABASE_NAME > /home/sanjana/mysql_backup.sql
```

### Example for app_db:
```bash
sudo mysqldump -u root app_db > /home/sanjana/app_db_backup.sql
```

## PostgreSQL Dump Commands

### Without Password:
```bash
sudo -u postgres pg_dump -F p DATABASE_NAME > /home/sanjana/postgres_backup.sql
```

### With Password:
```bash
sudo -u postgres PGPASSWORD=YOUR_PASSWORD pg_dump -F p DATABASE_NAME > /home/sanjana/postgres_backup.sql
```

### Example for abc database:
```bash
sudo -u postgres pg_dump -F p abc > /home/sanjana/abc_backup.sql
```

## Verify Dump Files

### Check file size:
```bash
ls -lh /home/sanjana/*.sql
```

### Check if file is empty:
```bash
wc -l /home/sanjana/your_backup.sql
```

### View first few lines (to verify it has content):
```bash
head -20 /home/sanjana/your_backup.sql
```

### Check file type and content:
```bash
file /home/sanjana/your_backup.sql
head -5 /home/sanjana/your_backup.sql
```

### For gzipped files:
```bash
# Check if it's actually gzipped
file /home/sanjana/your_backup.sql.gz

# View first few lines of gzipped file
zcat /home/sanjana/your_backup.sql.gz | head -20
```

## Test Restore (Manual)

### MySQL Restore:
```bash
sudo mysql -u root DATABASE_NAME < /home/sanjana/mysql_backup.sql
```

### PostgreSQL Restore:
```bash
sudo -u postgres psql -d DATABASE_NAME < /home/sanjana/postgres_backup.sql
```

## Important Notes

1. **Match Engine and Dump**: Make sure you're using a MySQL dump for MySQL restore and PostgreSQL dump for PostgreSQL restore. They are NOT compatible!

2. **Single Database vs All Databases**: 
   - `mysqldump DATABASE_NAME` - Creates dump for ONE database (use this for restore)
   - `mysqldump --all-databases` - Creates dump for ALL databases (needs extraction for single DB restore)
   - `pg_dump DATABASE_NAME` - Creates dump for ONE database (use this for restore)
   - `pg_dumpall` - Creates dump for ALL databases (needs extraction for single DB restore)

3. **If you used --all-databases**: The restore will try to extract the target database automatically, but if the dump file doesn't contain your target database, the restore will result in 0 tables.

2. **File Permissions**: Ensure the dump file is readable:
   ```bash
   ls -l /home/sanjana/your_backup.sql
   ```

3. **Check File Content**: A valid SQL dump should start with SQL comments or CREATE statements:
   ```bash
   head -10 /home/sanjana/your_backup.sql
   ```

4. **For Testing**: Create a simple test dump:
   ```bash
   # MySQL
   sudo mysqldump -u root app_db > /home/sanjana/test_mysql.sql
   
   # PostgreSQL  
   sudo -u postgres pg_dump -F p abc > /home/sanjana/test_postgres.sql
   ```
