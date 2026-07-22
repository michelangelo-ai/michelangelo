---
sidebar_position: 6
---

# Sandbox Ports and Endpoints

This is a quick reference for the URLs and ports your local Michelangelo AI sandbox exposes on `localhost`. After `ma sandbox create` finishes, use this page to find where to point your browser, your CLI, or your SDK client.

If you haven't set up the sandbox yet, start with [Sandbox Setup](./sandbox-setup.md) first.

## Quick links

The most common URLs you'll open after the sandbox is up:

| Service | URL | Default credentials |
|---|---|---|
| MA Studio UI | http://localhost:8090 | none |
| MinIO Console | http://localhost:9090 | `minioadmin` / `minioadmin` |
| Cadence Web (default) | http://localhost:8088 | none |
| Temporal Web (with `--workflow temporal`) | http://localhost:8080 | none |
| Grafana | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9092 | none |
| MLflow Tracking (with `--include-experimental mlflow`) | http://localhost:5001 | none |
| Ray Dashboard (with `--create-compute-cluster`) | http://localhost:8265 | none |

The MySQL root password is `root` (database: `temporal`). Connect with `mysql -h 127.0.0.1 -P 3306 -u root -proot`.

> **Heads up:** these credentials are intentionally trivial because the sandbox is meant for local development on your machine only. Do not reuse them anywhere else.

## All exposed ports

Every port below is mapped from the k3d cluster to your host so you can reach it directly from `localhost`.

### Default sandbox (Cadence workflow engine)

These mappings are created automatically by `ma sandbox create`:

| Service | Host port | NodePort | In-cluster service | Container port | What it's for |
|---|---:|---:|---|---:|---|
| MA Studio UI | 8090 | 30011 | `michelangelo-ui` | 8090 | Web UI — your main entry point |
| Michelangelo AI API Server | 15566 | 30009 | `michelangelo-apiserver` | 15566 | gRPC API (YARPC) for SDK and CLI clients |
| Envoy (gRPC-web proxy) | 8081 | 30010 | `michelangelo-envoy` | 8081 | gRPC-web → gRPC bridge used by the UI |
| Cadence Web | 8088 | 30004 | `michelangelo-cadence-web` | 8088 | Inspect, retry, and debug Cadence workflow runs |
| Cadence Frontend (gRPC) | 7833 | 30002 | `michelangelo-cadence` | 7833 | Cadence SDK and client connections |
| Cadence Frontend (TChannel) | 7933 | 30003 | `michelangelo-cadence` | 7933 | Cadence internal RPC |
| MySQL | 3306 | 30001 | `mysql` | 3306 | Metadata storage for the workflow engine |
| MinIO (S3 API) | 9091 | 30007 | `minio` | 9091 | S3-compatible object storage endpoint |
| MinIO (Console) | 9090 | 30008 | `minio` | 9090 | Web UI for browsing buckets |
| Grafana | 3000 | 30012 | `grafana` | 3000 | Dashboards (skipped with `--exclude grafana`) |
| Prometheus | 9092 | 30015 | `prometheus` | 9090 | Metrics (skipped with `--exclude prometheus`) |
| MLflow Tracking | 5001 | 30013 | `mlflow` | 5000 | Experiment tracking (only deployed with `--include-experimental mlflow`) |

### Temporal workflow engine

When you pass `--workflow temporal`, the sandbox swaps the Cadence-specific mappings for Temporal ones at cluster-create time. The Cadence Web port is not exposed; everything else above (UI, API Server, MinIO, MySQL, Grafana, Prometheus, MLflow) stays the same.

| Service | Host port | NodePort | In-cluster service | What it's for |
|---|---:|---:|---|---|
| Temporal Web | 8080 | 30005 | `michelangelo-temporal-web` | Inspect, retry, and debug Temporal workflow runs |

> **Note:** the Temporal frontend gRPC service (`michelangelo-temporal-frontend:7233`) is reachable inside the cluster but is **not** auto-exposed on the host. If you need direct host access for SDK testing, port-forward it manually:
>
> ```bash
> kubectl port-forward svc/michelangelo-temporal-frontend 7233:7233
> ```

### Optional Ray compute cluster

If you ran `ma sandbox create --create-compute-cluster`, a second k3d cluster is created for Ray jobs with these mappings:

| Service | Host port | What it's for |
|---|---:|---|
| Ray Dashboard | 8265 | Browse Ray jobs, actors, and resources |
| Ray Client | 10001 | Submit jobs from a Ray client |

Without `--create-compute-cluster`, the main sandbox cluster itself acts as the Ray target — you can still run Ray workloads, just not in an isolated cluster.

## Connecting from outside the cluster

A few common patterns once the sandbox is up:

```bash
# Open the Michelangelo UI
open http://localhost:8090

# Talk to the API server with grpcurl
grpcurl -plaintext localhost:15566 list

# Connect to MySQL
mysql -h 127.0.0.1 -P 3306 -u root -proot temporal

# Use MinIO with the AWS CLI (S3-compatible)
AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
  aws --endpoint-url http://localhost:9091 --region us-east-1 s3 ls
```

## Troubleshooting port conflicts

If `ma sandbox create` fails because a host port is already bound, find what's holding it and free it up:

```bash
# Find the process using a port (e.g., 9090)
lsof -i :9090

# Stop it if it's safe to do so
kill <PID>
```

Then retry:

```bash
# Non-destructive — preserves existing sandbox state
ma sandbox stop && ma sandbox start

# Destructive — wipes the cluster and starts fresh
ma sandbox delete && ma sandbox create
```

For other sandbox issues (image pulls, crashing pods, Temporal namespace errors), see the [Troubleshooting section in Sandbox Setup](./sandbox-setup.md#troubleshooting).

## Advanced: in-cluster ports

You usually don't need these — they're for developers debugging Kubernetes networking, writing controllers, or running probes from inside the cluster.

| Component | Port | Purpose |
|---|---:|---|
| `michelangelo-controllermgr` | 8091 | Controller manager metrics |
| `michelangelo-controllermgr` | 8081 | Controller manager health probe |
| `michelangelo-apiserver` | 15566 | gRPC API |
| `michelangelo-envoy` | 8081 | gRPC-web proxy |
| `michelangelo-cadence` | 7833 | Cadence frontend (gRPC) |
| `michelangelo-cadence` | 7933 | Cadence frontend (TChannel) |
| `michelangelo-cadence-web` | 8088 | Cadence Web UI |
| `michelangelo-temporal-frontend` | 7233 | Temporal frontend (gRPC) |
| `michelangelo-temporal-web` | 8080 | Temporal Web UI |
| `minio` | 9090 / 9091 | Console / S3 API |
| `mysql` | 3306 | MySQL |
