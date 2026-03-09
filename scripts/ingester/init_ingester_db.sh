#!/bin/bash
# Michelangelo Ingester Database Initialization Script
# This script initializes the MySQL database schema for the ingester
# Safe to run multiple times (idempotent)
# Works for both sandbox and production environments

set -e

# Configuration from environment variables with defaults
MYSQL_HOST="${MYSQL_HOST:-localhost}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-root}"
MYSQL_DATABASE="${MYSQL_DATABASE:-michelangelo}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Michelangelo Ingester DB Initialization${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Configuration:"
echo "  Host: $MYSQL_HOST"
echo "  Port: $MYSQL_PORT"
echo "  User: $MYSQL_USER"
echo "  Database: $MYSQL_DATABASE"
echo ""

# Get the script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SCHEMA_FILE="$SCRIPT_DIR/ingester_schema.sql"

if [ ! -f "$SCHEMA_FILE" ]; then
    echo -e "${RED}ERROR: Schema file not found: $SCHEMA_FILE${NC}"
    exit 1
fi

# Test MySQL connection
echo -e "${YELLOW}Testing MySQL connection...${NC}"
if ! mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SELECT 1" &>/dev/null; then
    echo -e "${RED}ERROR: Cannot connect to MySQL${NC}"
    echo "Please check:"
    echo "  - MySQL is running"
    echo "  - Credentials are correct"
    echo "  - Host and port are accessible"
    exit 1
fi
echo -e "${GREEN}✓ MySQL connection successful${NC}"
echo ""

# Create database if it doesn't exist
echo -e "${YELLOW}Creating database '$MYSQL_DATABASE' if not exists...${NC}"
mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "CREATE DATABASE IF NOT EXISTS $MYSQL_DATABASE;"
echo -e "${GREEN}✓ Database ready${NC}"
echo ""

# Execute schema
echo -e "${YELLOW}Initializing ingester schema...${NC}"
mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" < "$SCHEMA_FILE"
echo -e "${GREEN}✓ Schema initialized${NC}"
echo ""

# Verify tables were created
echo -e "${YELLOW}Verifying tables...${NC}"
TABLE_COUNT=$(mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -sN -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$MYSQL_DATABASE';")
echo -e "${GREEN}✓ Found $TABLE_COUNT tables${NC}"
echo ""

# List tables
echo "Tables created:"
mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" -e "SHOW TABLES;" | grep -v "Tables_in" | sed 's/^/  - /'
echo ""

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Ingester database initialization complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Next steps:"
echo "  1. Start the controllermgr with ingester enabled"
echo "  2. Create CRD objects (Models, Pipelines, etc.)"
echo "  3. Verify they sync to MySQL"
echo ""
echo "To verify sync:"
echo "  mysql -h $MYSQL_HOST -P $MYSQL_PORT -u $MYSQL_USER -p$MYSQL_PASSWORD $MYSQL_DATABASE -e \"SELECT * FROM model;\""
