# Multi-Version CRD Support Setup Guide

This guide explains how to add multi-version support for Custom Resource Definitions (CRDs) in the Michelangelo API Server. This is useful for testing API versioning, backward compatibility, and CRD conversion webhooks.

## Overview

Multi-version CRD support allows you to:
- Serve multiple API versions (e.g., v2 and v2alpha1) simultaneously
- Test conversion between versions using conversion webhooks
- Validate backward compatibility of API changes

**Note:** Multi-version support is for **local testing only** and should not be committed to the main branch.

## Prerequisites

1. A test API version defined in `proto/test/api/` (e.g., `proto/test/api/v2alpha1/`)
2. TLS certificates generated for the webhook server

## Step 1: Creating Test API Versions

When creating a test version with differences from the hub version (e.g., field name changes), you need to implement conversion logic. Reference `go/cmd/kubeproto/protoc-gen-kubeconversion/README.md` for detailed instructions on:

- How to handle field differences between versions
- Implementing custom conversion logic
- Using the kubeconversion protoc plugin

The conversion code generator will create `ConvertTo` and `ConvertFrom` methods to handle transformations between your test version and the hub version.

## Step 2: Update go/cmd/apiserver/main.go

Add the import:

```go
import (
    // ... existing imports ...
    v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
    v2alpha1pb "github.com/michelangelo-ai/michelangelo/proto/test/api/v2alpha1"  // Add this
)
```

Update the `getScheme()` function to register the test version:

```go
func getScheme() (*runtime.Scheme, error) {
    s := scheme.Scheme
    if err := v2pb.AddToScheme(s); err != nil {
        return nil, err
    }
    // Add this:
    if err := v2alpha1pb.AddToScheme(s); err != nil {
        return nil, err
    }
    return s, nil
}
```

Update the `crd.SyncCRDs()` call in the `opts()` function:

```go
crd.SyncCRDs(v2pb.GroupVersion.Group,
    []string{},
    v2pb.YamlSchemas,
    v2alpha1pb.YamlSchemas),  // Add this
```

**Note:** After updating `main.go`, run `./tools/gazelle` to automatically update `BUILD.bazel` dependencies.

## Step 3: Configure CRD Version Settings

Ensure your `config/base.yaml` has the multi-version configuration:

```yaml
apiserver:
  crdSync:
    enableCRDUpdate: true
    crdVersions:
      projects.michelangelo.api:
        versions: [v2, v2alpha1]
        storageVersion: v2  # v2 is the hub (storage) version
```

## Step 4: Generate Certificates and Configure Webhook

### Generate TLS Certificates

```bash
# Generate certificates for the webhook server
./tools/test/generate-certs.sh
```

This creates certificates in `tools/test/certs/`.

### Update Webhook Configuration

Update `config/base.yaml` to point to the certificate directory:

```yaml
webhook:
  host: "0.0.0.0"
  port: 9443
  certDir: "<absolute-path-to-repo>/tools/test/certs"  # Update this path
  url: "https://host.docker.internal:9443"  # For k3d
```

**Note:** Use the absolute path to avoid "file not found" errors.

## Step 5: Start the API Server

The webhook server will be started automatically via dependency injection when you start the API server.

```bash
bazel run //go/cmd/apiserver:apiserver
```

## Verification

### Check CRD Registration

```bash
# Verify both versions are registered
kubectl get crd projects.michelangelo.api -o jsonpath='{.spec.versions[*].name}'
# Expected output: v2 v2alpha1

# Verify conversion strategy
kubectl get crd projects.michelangelo.api -o jsonpath='{.spec.conversion.strategy}'
# Expected output: Webhook
```

### Test Multi-Version Support

```bash
# Create a Project using v2alpha1
kubectl apply -f proto/test/api/v2alpha1/test-project-v2alpha1.yaml

# Retrieve as v2 (hub version)
kubectl get project.v2.michelangelo.api my-project -o yaml

# Retrieve as v2alpha1 (spoke version)
kubectl get project.v2alpha1.michelangelo.api my-project -o yaml

# Verify field conversions work (e.g., description ↔ projectDescription)
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ User Request (v2alpha1)                                 │
└─────────────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────────────┐
│ kube-apiserver                                          │
│ - Receives v2alpha1 Project                             │
│ - Needs to convert to v2 (storage version)              │
└─────────────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────────────┐
│ Conversion Webhook                                      │
│ URL: https://host.docker.internal:9443/convert          │
│ - Converts: v2alpha1.ProjectSpec → v2.ProjectSpec       │
│ - Handles field renaming (projectDescription → description) │
└─────────────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────────────┐
│ etcd (Storage)                                          │
│ - Stores as v2 (hub version)                            │
└─────────────────────────────────────────────────────────┘
```
