package tools

// This file is to force "go mod tidy" to download Go modules that we need but are not imported
// in the Go code.
import (
	_ "k8s.io/api/core/v1"
)
