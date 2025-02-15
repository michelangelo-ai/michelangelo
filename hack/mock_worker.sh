#!/bin/bash

# Navigate to the `go/` directory
echo "Navigating to the go/ directory..."
cd go/ || { echo "go/ directory not found. Please ensure you are in the correct location."; exit 1; }

# Set up environment
echo "Setting up environment and installing gomock and mockgen..."

# Install gomock
echo "Installing gomock library..."
go get github.com/golang/mock/gomock || { echo "Failed to install gomock"; exit 1; }

# Install mockgen in ../bin/ directory
echo "Installing mockgen tool to ../bin/..."
GO_BIN_DIR="../bin"
go install github.com/golang/mock/mockgen@latest || { echo "Failed to install mockgen"; exit 1; }

# Verify installation
MOCKGEN_EXEC="$GO_BIN_DIR/mockgen"
if [[ ! -x "$MOCKGEN_EXEC" ]]; then
  echo "mockgen was not successfully installed in $GO_BIN_DIR. Please check your setup." >&2
  exit 1
fi

echo "Verifying mockgen installation..."
"$MOCKGEN_EXEC" --version || { echo "Failed to verify mockgen installation"; exit 1; }

# Generate mocks for the Worker interface
echo "Generating mocks for the Worker interface..."
"$MOCKGEN_EXEC" -destination=worker/mock_worker.go -package=worker go.uber.org/cadence/worker Worker || { echo "Failed to generate Worker mock"; exit 1; }

echo "Mocks successfully generated in the go/ directory."
