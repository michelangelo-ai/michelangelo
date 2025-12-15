# Michelangelo Kubernetes Deployment

This directory contains Kubernetes configurations for deploying Michelangelo using Kustomize with the **overlay pattern**.

## 🏗️ Architecture Overview

Michelangelo consists of the following core components:

- **API Server** - Core gRPC API service for pipeline management
- **Controller Manager** - Kubernetes controller for managing Michelangelo resources
- **Worker** - Workflow execution workers (Temporal/Cadence)
- **UI** - Web interface with Envoy proxy

## 📁 Clear Configuration Structure

```
deploy/k8s/
├── README.md                    # This file
├── base/                       # Base Kubernetes manifests (shared across environments)
│   ├── kustomization.yaml      # Base kustomization
│   ├── namespace.yaml          # Namespace definition
│   ├── apiserver/              # API server base configs
│   ├── controllermgr/          # Controller manager base configs
│   ├── worker/                 # Worker base configs
│   └── michelangelo-ui/        # UI base configs
└── overlays/                   # Environment-specific overlays
    ├── dev/                    # 🔧 Development environment
    ├── staging/                # 🚀 Staging environment
    ├── prod/                   # 🏭 Production environment
    └── sandbox/                # 🧪 Sandbox environment (k3d-based)
```

## 🎯 Why This Structure?

- **`deploy/k8s/`**: All Kubernetes-related configs (clear intent)
- **`base/`**: Shared configuration (deployments, services, RBAC)
- **`overlays/`**: Environment-specific customizations only
- **No naming confusion**: Each folder has a clear, distinct purpose

## 🚀 Quick Start

### Prerequisites

Before deploying Michelangelo, ensure you have:

- **Kubernetes cluster** (>= 1.24)
- **kubectl** installed and configured
- **External Temporal/Cadence** workflow engine
- **Object storage** (S3-compatible)
- **Database** (MySQL/PostgreSQL) for Temporal/Cadence

### Deploy to Any Environment

```bash
# Clone the repository
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo

# Choose your target environment:

# 🔧 Development (single replicas, 'main' tags)
kubectl apply -k deploy/k8s/overlays/dev

# 🚀 Staging (moderate replicas, 'v1.0.0-rc' tags)
kubectl apply -k deploy/k8s/overlays/staging

# 🧪 Sandbox (k3d + full stack, 'main' tags)
kubectl apply -k deploy/k8s/overlays/sandbox

# 🏭 Production (high replicas, stable 'v1.0.0' tags)
kubectl apply -k deploy/k8s/overlays/prod
```

## ⚙️ How Overlays Work

### **Base Configuration (Shared)**
- Common deployments, services, RBAC configurations
- No environment-specific settings
- Used by ALL environments as foundation

### **Environment Overlays (Differences Only)**

| Setting | Dev | Staging | Sandbox | Production |
|---------|-----|---------|---------|------------|
| **Replicas** | 1 each | 2 each | 1 each | 3-5 each |
| **Image Tag** | `main` | `v1.0.0-rc` | `main` | `v1.0.0` |
| **Name Prefix** | `dev-` | `staging-` | `sandbox-` | `prod-` |
| **Environment Label** | `development` | `staging` | `sandbox` | `production` |
| **Infrastructure** | External | External | Includes DBs/Storage | External |

### **Example: Dev vs Prod Deployment Names**

```bash
# Development deployment names:
dev-michelangelo-apiserver
dev-michelangelo-worker

# Production deployment names:
prod-michelangelo-apiserver
prod-michelangelo-worker
```

## 🧪 Sandbox Overlay

The **sandbox** overlay is special - it includes a complete self-contained Michelangelo stack:

- **Full Infrastructure**: MySQL, MinIO, Temporal all included
- **No External Dependencies**: Everything runs in your cluster
- **Development Ready**: Uses `main` image tags
- **k3d Compatible**: Works with local k3d clusters

### Quick Sandbox Setup

```bash
# If using with ma sandbox command:
ma sandbox create --workflow temporal

# Or deploy manually to any cluster:
kubectl apply -k deploy/k8s/overlays/sandbox
```

**Sandbox includes:**
- Michelangelo API Server, Controller Manager, Worker, UI
- Temporal workflow engine with MySQL backend
- MinIO object storage
- Envoy proxy for gRPC-Web
- All necessary ConfigMaps and secrets

Need a different environment? Easy:

```bash
# Create new environment overlay
mkdir -p deploy/k8s/overlays/testing

# Create kustomization.yaml
cat > deploy/k8s/overlays/testing/kustomization.yaml << EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

namePrefix: test-

replicas:
  - name: michelangelo-apiserver
    count: 1
  - name: michelangelo-worker
    count: 2

images:
  - name: ghcr.io/michelangelo-ai/apiserver
    newTag: feature-branch

commonLabels:
  environment: testing
EOF

# Deploy it
kubectl apply -k deploy/k8s/overlays/testing
```

## 🔍 Verification

Check your deployment:

```bash
# See all pods (adjust environment label)
kubectl get pods -l environment=production

# Check specific deployment
kubectl get deployment prod-michelangelo-apiserver

# Test API health
kubectl port-forward svc/prod-michelangelo-apiserver 14566:14566
curl http://localhost:14566/health
```

## 📋 Required Setup

### Create Secrets First

```bash
# Database credentials
kubectl create secret generic michelangelo-database \
  --from-literal=host=your-db-host \
  --from-literal=database=michelangelo \
  --from-literal=username=michelangelo \
  --from-literal=password=your-password

# Storage credentials
kubectl create secret generic michelangelo-storage \
  --from-literal=endpoint=s3.amazonaws.com \
  --from-literal=region=us-west-2 \
  --from-literal=bucket=your-bucket \
  --from-literal=access_key_id=your-access-key \
  --from-literal=secret_access_key=your-secret-key

# Temporal/Cadence connection
kubectl create secret generic michelangelo-temporal \
  --from-literal=host=temporal.yourdomain.com:7233 \
  --from-literal=domain=michelangelo
```

## 🆘 Support

For deployment issues:
- Review [known issues](https://github.com/michelangelo-ai/michelangelo/issues)
- Join [community discussions](https://github.com/michelangelo-ai/michelangelo/discussions)