#!/bin/sh
set -e

# Vite applications only support build time environment variable injection, which
# would require 1:1 mapping of docker image to Michelangelo Studio use case. As a
# workaround, this script replaces __MICHELANGELO prefixed environment variables
# at runtime.
echo "Starting environment variable substitution..."

# Check for required environment variables
if [ -z "$__MICHELANGELO_API_BASE_URL" ]; then
    echo "❌ FATAL: __MICHELANGELO_API_BASE_URL environment variable is required but not set"
    echo "Please set it in your Kubernetes deployment:"
    echo "  env:"
    echo "  - name: __MICHELANGELO_API_BASE_URL"
    echo "    value: \"http://your-api-endpoint:8081\""
    exit 1
fi

# Replace all __MICHELANGELO_ prefixed environment variables
for i in $(env | grep __MICHELANGELO_)
do
    key=$(echo $i | cut -d '=' -f 1)
    value=$(echo $i | cut -d '=' -f 2-)
    echo "Replacing ${key}_PLACEHOLDER with: $value"

    # Replace in JS and CSS only (more targeted than all files)
    find /usr/share/nginx/html -type f \( -name '*.js' -o -name '*.css' \) -exec sed -i "s|${key}_PLACEHOLDER|${value}|g" '{}' +
done

echo "Environment variable substitution completed"
