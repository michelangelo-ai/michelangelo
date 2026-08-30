---
name: ma-sandbox-debug
description: Tail logs, inspect pods, and diagnose unhealthy services in a running Michelangelo sandbox. Use when a service is crashlooping, returning errors, or not responding as expected. Triggers on "check logs", "why is X failing", "debug sandbox", "kubectl logs".
user-invocable: true
---

# Sandbox Debug

Diagnosing service failures in a running sandbox. Start with health overview, then drill into specific services.

## Step 1: Health overview

```bash
ma sandbox health        # high-level check: cluster, pods, API, envoy, UI
kubectl get pods -A      # full pod inventory with status
```

**IF** `kubectl get pods -A` output contains any pod in `CrashLoopBackOff`, `Error`, or `ImagePullBackOff` state (excluding expected noise listed below): **proceed to Step 3** for each affected pod.

**IF** a pod is stuck `Pending` beyond 3 minutes: **proceed to Step 3** for that pod.

**IF** all pods show `Running` or `Completed`: report the cluster as healthy and stop — do not continue to Step 2 unless the user reports a specific symptom.

## Step 2: Tail logs by service

```bash
# apiserver — handles all API requests (Go)
kubectl logs -f deployment/michelangelo-apiserver

# worker — handles async tasks
kubectl logs -f deployment/michelangelo-worker

# controller manager
kubectl logs -f deployment/michelangelo-controllermgr

# envoy — ingress proxy (proxy errors appear here, not in apiserver)
kubectl logs -f deployment/michelangelo-envoy

# UI — nginx serving the frontend bundle
kubectl logs -f deployment/michelangelo-ui
```

Useful flags: `--tail=100` to limit output; `--previous` to see logs from the last crashed container instance.

## Need more verbose logs?

Go services default to `info` level. Enable debug logging for deeper investigation:

```bash
bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/set_log_level.sh controllermgr debug
```

Revert when done: replace `debug` with `info`. Pair with `$(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/capture_service_logs.sh` to filter the resulting output down to signal.

## Step 3: Inspect a crashing pod

```bash
# Get the pod name
kubectl get pods -l app.kubernetes.io/component=apiserver

# Events and last exit reason
kubectl describe pod <pod-name>

# Logs from the previous crashed instance
kubectl logs <pod-name> --previous
```

The **Events** section and **Last State** (exit code, reason) in `describe` output are the most diagnostic fields.

## Common failure patterns

| Symptom | Where to look | Likely cause |
|---------|--------------|--------------|
| UI loads, API returns 503 | envoy logs | Envoy can't reach apiserver |
| UI loads but shows no data | `kubectl get projects,pipelines,pipelineruns -A` | No demo data seeded — run `ma sandbox demo pipeline`. If resources exist but UI is empty, check envoy logs |
| API returns 500 | apiserver logs | Go panic or DB connection error |
| Pod in `CrashLoopBackOff` | `describe pod`, `--previous` logs | OOM, missing config, bad binary |
| Pod stuck `Pending` | `describe pod` Events | No node resources or PVC mount failure |
| `ImagePullBackOff` | `describe pod` Events | Image not imported — run `k3d image import` |
| Feature broken after deploy | apiserver logs | Business logic bug in the deployed binary |
| `create` failed partway / "cluster already exists" | — | Run `ma sandbox sync` to finish — don't delete+recreate |

## Filter logs for errors

```bash
kubectl logs deployment/michelangelo-apiserver | grep -i "error\|panic\|fatal"
kubectl logs deployment/michelangelo-worker --tail=200
```

## Force-restart a stuck service

When a service isn't crashing but isn't picking up new config or code:

```bash
kubectl rollout restart deployment/michelangelo-apiserver
kubectl rollout status deployment/michelangelo-apiserver --timeout=60s
```

## Expected noise (not actual errors)

- `cadence-schema-init`, `ingester-schema-init`, `sandbox-bucket-setup` — reach `Completed` and stay there; this is correct
- `ray-history-server` in `ImagePullBackOff` — non-blocking; Ray jobs still work without it

## Still stuck?

If the cluster state is unrecoverable, do a full reset: `/ma-sandbox-reset`.
