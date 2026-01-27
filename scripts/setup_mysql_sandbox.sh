#!/bin/bash

# MySQL Setup Script for Michelangelo Ingester (Sandbox)
# This script sets up MySQL database with all CRD tables for the ingester controller

set -e

# Configuration
DB_HOST="${MYSQL_HOST:-localhost}"
DB_PORT="${MYSQL_PORT:-3306}"
DB_NAME="${MYSQL_DATABASE:-michelangelo}"
DB_USER="${MYSQL_USER:-root}"
DB_PASSWORD="${MYSQL_PASSWORD:-}"

echo "=================================================="
echo "Michelangelo MySQL Sandbox Setup"
echo "=================================================="
echo "Database Host: $DB_HOST"
echo "Database Port: $DB_PORT"
echo "Database Name: $DB_NAME"
echo "Database User: $DB_USER"
echo "=================================================="

# Function to execute SQL
execute_sql() {
    local sql="$1"
    if [ -z "$DB_PASSWORD" ]; then
        mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -e "$sql"
    else
        mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASSWORD" -e "$sql"
    fi
}

# Function to execute SQL file
execute_sql_file() {
    local file="$1"
    if [ -z "$DB_PASSWORD" ]; then
        mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" < "$file"
    else
        mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASSWORD" < "$file"
    fi
}

# Create database if it doesn't exist
echo "Creating database '$DB_NAME' if it doesn't exist..."
execute_sql "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

echo "Database '$DB_NAME' is ready."
echo ""
echo "Creating CRD tables..."

# Create schema file path
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_FILE="$SCRIPT_DIR/mysql_schema.sql"

# Check if schema file exists
if [ ! -f "$SCHEMA_FILE" ]; then
    echo "Error: Schema file not found at $SCHEMA_FILE"
    echo "Please run: ./scripts/generate_mysql_schema.sh first"
    exit 1
fi

# Execute schema file
if [ -z "$DB_PASSWORD" ]; then
    mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" "$DB_NAME" < "$SCHEMA_FILE"
else
    mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASSWORD" "$DB_NAME" < "$SCHEMA_FILE"
fi

echo ""
echo "=================================================="
echo "MySQL setup completed successfully!"
echo "=================================================="
echo ""
echo "Database: $DB_NAME"
echo ""
echo "To verify the setup:"
echo "  mysql -h $DB_HOST -P $DB_PORT -u $DB_USER $DB_NAME -e 'SHOW TABLES;'"
echo ""
echo "To connect to the database:"
echo "  mysql -h $DB_HOST -P $DB_PORT -u $DB_USER $DB_NAME"
echo ""
