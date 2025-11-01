#!/usr/bin/env bash

set -eou pipefail

# Compile Chronon definitions for Amazon Books Qwen pipeline
echo "Compiling Chronon definitions for Amazon Books Qwen..."

# Create output directory
mkdir -p .compile

# Compile staging queries
echo "Compiling staging queries..."
mkdir -p .compile/staging_queries/amazon_books
python -m chronon.compile \
    --input-path data/staging_queries/amazon_books \
    --output-path .compile/staging_queries/amazon_books

# Compile group_bys
echo "Compiling group_bys..."
mkdir -p .compile/group_bys/amazon_books
python -m chronon.compile \
    --input-path data/group_bys/amazon_books \
    --output-path .compile/group_bys/amazon_books

# Compile joins
echo "Compiling joins..."
mkdir -p .compile/joins/amazon_books
python -m chronon.compile \
    --input-path data/joins/amazon_books \
    --output-path .compile/joins/amazon_books

echo "✅ Chronon compilation completed!"
echo "   Compiled definitions available in .compile/ directory"