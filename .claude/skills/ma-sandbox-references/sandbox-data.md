# Sandbox Data Reference

How to inspect, seed, reset, and create data in a running Michelangelo sandbox.

## Constraints

- **kubectl is the right inspection tool** — curl, yab, and grpcurl do not work against the YARPC/gRPC apiserver without a compiled protobuf FileDescriptorSet.
- **UI interactions or `ma` CLI are the right creation tools** — no direct API calls.

---

## Demo data

Seed with:

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
source "$REPO_ROOT/python/.venv/bin/activate"
ma sandbox demo pipeline       # training pipelines, trigger runs, eval pipeline
ma sandbox demo inference      # inference server (only needed for deploy-phase testing)
```

Both commands run `kubectl apply` on the YAML files under:
- `python/michelangelo/cli/sandbox/demo/pipeline/`
- `python/michelangelo/cli/sandbox/demo/inference/`

Read those files directly to see exactly what entities get created, their names, and their specs. Both commands are **idempotent** — safe to re-run.

Trigger runs are not created via standalone YAML — `ma sandbox demo pipeline` seeds a pipeline with a trigger attached (e.g. `training-pipeline-with-trigger.yaml`), and a trigger run is then created from that pipeline through the UI.

**State after seeding**: entity state is set by the controller manager after apply — inspect with `kubectl get` to see actual current states rather than assuming.

---

## Inspecting state

```bash
# All resources in the project
kubectl get pipelines,pipelineruns,triggerruns -n ma-dev-test

# Detailed status of a specific resource
kubectl describe triggerrun <name> -n ma-dev-test

# Watch state changes live
kubectl get triggerruns -n ma-dev-test -w
```

---

## Resetting demo data to a known state

When an entity is in the wrong state for a test:

### Option A — Delete the specific resource and re-apply

```bash
kubectl delete triggerrun <name> -n ma-dev-test
ma sandbox demo pipeline
```

The reconciler will recreate it in its initial state.

### Option B — Create a new entity via the UI

Navigate to the relevant list page, use the "Create" or "Run" action to create a fresh entity. Faster than a full re-seed and exercises more of the stack — prefer this when the goal is to test a state transition.

---

## Ports

| Service | URL |
|---------|-----|
| Michelangelo UI (full sandbox) | http://localhost:8090 |
| API Server | http://localhost:15566 |
| Vite dev server (frontend only, mocks) | http://localhost:5173 |
| Cadence Web | http://localhost:8088 |
| Temporal Web | http://localhost:8080 |
| MinIO Console | http://localhost:9090 |
| Grafana | http://localhost:3000 |
| Ray Dashboard | http://localhost:8265 |

Full port list: `docs/getting-started/ma-sandbox-ports-and-endpoints.md`
