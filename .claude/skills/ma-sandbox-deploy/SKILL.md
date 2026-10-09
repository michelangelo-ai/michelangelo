---
name: ma-sandbox-deploy
description: Build a Go service binary, package it into a Docker image, import into k3d, and deploy via helm sync. Use when iterating on apiserver (or another Go service) and wanting to test changes in a running sandbox without waiting for CI.
user-invocable: true
allowed-tools:
  - Bash(bash .claude/skills/ma-sandbox-scripts/deploy-local.sh *)
  - Bash(bash .claude/skills/ma-sandbox-scripts/start.sh)
  - Bash(bash .claude/skills/ma-sandbox-scripts/python-env.sh *)
  - Bash(bash .claude/skills/ma-sandbox-scripts/check-backend.sh)
  - Bash(kubectl get *)
  - Bash(kubectl logs *)
  - Bash(kubectl rollout status *)
  - Bash(docker image inspect *)
  - Edit(.claude/skills/ma-sandbox-scripts/**)
---

# Sandbox Local Image Dev Loop

For iterating on Go services (`apiserver`, `controllermgr`, `worker`) inside a running sandbox. Requires an existing sandbox cluster; if none exists, run `ma sandbox create` first (see `/ma-sandbox-setup`).

## How to run commands in this skill

Auto mode honors this skill's `allowed-tools` only **after the skill has been invoked**, so invoke it before calling any script; a script call made earlier goes to the classifier like any other command and can be blocked.

Then call each script in the exact literal form `allowed-tools` lists, from the repo root: `bash .claude/skills/ma-sandbox-scripts/<name>.sh <args>`. Any change to the command text falls outside the pattern and loses the pre-approval, so don't add a `$(git rev-parse ...)` path prefix, wrap the call in `VAR=$(...)`, or chain it with `&&`, `;`, `cd`, `source` or `rm -rf`. The scripts find the repo, the Python venv and their `/tmp` build directories themselves, so none of that is needed.

## Deploy

1. If the cluster is stopped (kubectl can't reach it), resume it. Don't reset it:

   ```bash
   bash .claude/skills/ma-sandbox-scripts/start.sh
   ```

2. Build and deploy one service. This is the whole loop, for the first deploy and every iteration after it:

   ```bash
   bash .claude/skills/ma-sandbox-scripts/deploy-local.sh apiserver
   bash .claude/skills/ma-sandbox-scripts/deploy-local.sh controllermgr
   bash .claude/skills/ma-sandbox-scripts/deploy-local.sh worker --repo /path/to/other/checkout
   ```

   `--repo` builds from another checkout (a worktree, for example) and treats it as read-only. Without it the script builds the repo it lives in.

   The build takes a few minutes; use a long Bash timeout (up to 600000 ms) or `run_in_background`, then read the output.

> **`ma sandbox sync`, which the script runs, drops and recreates the sandbox MySQL `michelangelo` database.** Data that exists only in metadata storage (for example PipelineRuns already pruned from etcd) is lost. Confirm the user accepts that before deploying to a sandbox that holds data they care about.

The script exits 0 only when all three verify signals pass, 1 on any failure, 2 on bad usage.

### What the script does

It builds a static `linux/arm64` binary (`CGO_ENABLED=0`, because the Distroless base has no libc) and the Docker image under `/tmp/ma-sandbox-deploy/<service>/`, labels the image with the git branch, short SHA and dirty flag, imports it into the `michelangelo-sandbox` k3d cluster, runs `ma sandbox sync`, restarts the deployment and waits for the rollout. Nothing is written to the working directory, and `go build` is never run in the repo root, where it would drop a stray binary.

### Image overrides

The chart takes the image as a map: `images.<service>.repository` and `images.<service>.tag`, for example `--set images.apiserver.repository=michelangelo-apiserver --set images.apiserver.tag=local`. The older `--set images.apiserver=michelangelo-apiserver:local` form puts a string where the chart expects a map and doesn't select the local image.

`ma sandbox sync` runs helm with `--reuse-values`, but an override has to be passed again on every call or helm falls back to the default `ghcr.io` image. The script handles this: it passes the override for the service being deployed and keeps any other service that is already running a `michelangelo-<service>:*` image. Only reach for manual `--set` flags if you run `ma sandbox sync` yourself, and then pass every override each time.

## Verify what's deployed

The script prints these three signals and checks them. The `:local` tag carries no identity of its own, so check all three when you verify by hand:

1. **It's the local build, not the released image.** The pod image should be `michelangelo-apiserver:local`, not `ghcr.io/michelangelo-ai/apiserver:main`.

   ```bash
   kubectl get pod -l app.kubernetes.io/component=apiserver -o jsonpath='{.items[0].spec.containers[0].image}'
   ```

2. **Which branch and commit it was built from.** The labels are baked into the image, so they survive `k3d image import`.

   ```bash
   docker image inspect michelangelo-apiserver:local --format '{{json .Config.Labels}}'
   ```

3. **It's the latest build, not a stale pod.** A pod that started before the image was built missed the rollout.

   ```bash
   docker image inspect michelangelo-apiserver:local --format 'built: {{.Created}}'
   kubectl get pod -l app.kubernetes.io/component=apiserver -o jsonpath='started: {.items[0].status.startTime}'
   ```

## When the deploy fails

Report the failing step and its output. Don't re-run `deploy-local.sh` or `ma sandbox sync` repeatedly, and don't patch cluster objects by hand to get past it.

- **Helm "Apply failed with N conflict" naming `kubectl-set` or `kubectl-client-side-apply`.** Helm applies server-side, and someone changed chart-owned objects with `kubectl set image` or `kubectl apply`, which gave those fields to a different field manager. Look at `helm status michelangelo` and `kubectl get <object> -o yaml --show-managed-fields`. Deploy images with `deploy-local.sh` instead of `kubectl set image`, so the chart keeps ownership.
- **`helm status michelangelo` reports `failed`.** A `sync` against a release that isn't `deployed` can uninstall and reinstall the whole release rather than upgrade it. Confirm with the user before re-running it.
- **Anything else wrong with the cluster.** `/ma-sandbox-debug` for diagnosis; `/ma-sandbox-reset` only as a last resort.
