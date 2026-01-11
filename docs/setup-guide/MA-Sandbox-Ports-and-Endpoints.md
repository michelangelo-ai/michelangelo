## MA Sandbox Ports and Endpoints

### Default (Cadence) mode

The sandbox maps NodePorts from the k3d cluster to localhost for easy access.

| Service | Host port | NodePort | In-cluster service | Container port | Purpose |
|---|---:|---:|---|---:|---|
| MySQL | 3306 | 30001 | `mysql` | 3306 | Storage for Cadence/Temporal |
| MinIO (Console) | 9090 | 30008 | `minio` | 9090 | Web UI for MinIO |
| MinIO (S3 API) | 9091 | 30007 | `minio` | 9091 | S3-compatible API endpoint |
| Michelangelo API Server | 14566 | 30009 | `ma-apiserver` | 14566 | gRPC API (YARPC) |
| Envoy (gRPC-web proxy) | 8081 | 30010 | `envoy` | 8081 | gRPC-web → gRPC proxy for UI/clients |
| Cadence Frontend (gRPC) | 7833 | 30002 | `cadence` | 7833 | Cadence SDK/clients |
| Cadence Frontend (TChannel) | 7933 | 30003 | `cadence` | 7933 | Cadence internal comms |
| Cadence Web | 8088 | 30004 | `cadence-web` | 8088 | Web UI for Cadence |

Quick links:

- MinIO Console: `http://localhost:9090`
- Michelangelo API (Envoy gRPC-web): `http://localhost:8081`
- Cadence Web UI: `http://localhost:8088`

Notes:

- Envoy is applied unless `--exclude ui` is used. If excluded, the 8081 mapping may be unused.
- The API server is reachable inside the cluster at `michelangelo-apiserver:14566` and externally via `localhost:14566`.

### Temporal mode

When `--workflow temporal` is used, Cadence services are not exposed. Instead, the script automatically port-forwards Temporal services:

| Service | Host port | Access | In-cluster service | Purpose |
|---|---:|---|---|---|
| Temporal Web | 8080 | `kubectl port-forward svc/temporaltest-web 8080:8080` | `temporaltest-web` | Web UI |
| Temporal Frontend (gRPC) | 7233 | `kubectl port-forward svc/temporaltest-frontend 7233:7233` | `temporaltest-frontend` | Temporal SDK/clients |

Quick links:

- Temporal Web UI: `http://localhost:8080`

All other sandbox ports (MySQL, MinIO, API Server, Envoy) remain the same as in the Cadence table.

### Optional Ray jobs cluster

If you pass `--create-jobs-cluster`, a dedicated k3d cluster for Ray jobs is created with the following host mappings:

| Service | Host port | Purpose |
|---|---:|---|
| Ray Client | 10001 | Ray client (ray job submission / client API) |
| Ray Dashboard | 8265 | Ray Dashboard UI |

Quick links:

- Ray Dashboard: `http://localhost:8265`

### Internal (in-cluster) ports reference

These ports are primarily for intra-cluster communication but are listed for reference:

- `michelangelo-controllermgr`
  - Controller Manager webhook/manager: 9443
  - Metrics: 8080
  - Health probe: 8081
- `michelangelo-apiserver` (service `ma-apiserver`): 14566
- `minio`: 9090 (console), 9091 (S3 API)
- `mysql`: 3306
- `cadence`: 7833 (gRPC), 7933 (TChannel)
- `cadence-web`: 8088
- `envoy`: 8081
