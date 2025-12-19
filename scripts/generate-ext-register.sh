#!/bin/bash
# Generate ext validation registration code from ext protos
#
# Usage:
#   ./scripts/generate-ext-register.sh
#
# This script scans proto/api/v2_ext/*_ext.proto files and generates
# register_generated.go with automatic type mappings.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "Generating ext validation registration code..."

go run tools/gen-ext-register/main.go \
    -proto_dir=proto/api/v2_ext \
    -output=proto/api/v2_ext/register_generated.go \
    -base_package=github.com/michelangelo-ai/michelangelo/proto/api/v2 \
    -ext_package=github.com/michelangelo-ai/michelangelo/proto/api/v2_ext \
    -api_package=github.com/michelangelo-ai/michelangelo/go/api

echo "Done! Generated proto/api/v2_ext/register_generated.go"
echo ""
echo "To add a new ext proto:"
echo "  1. Create proto/api/v2_ext/<name>_ext.proto with *Ext messages"
echo "  2. Run this script to regenerate registration"
echo "  3. Import v2_ext package in your application"

