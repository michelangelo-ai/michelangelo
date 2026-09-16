---
name: ma-sandbox-deploy
description: Build a Go service binary, package it into a Docker image, import into k3d, and deploy via helm sync. Use when iterating on apiserver (or another Go service) and wanting to test changes in a running sandbox without waiting for CI.
user-invocable: true
---

# Sandbox Local Image Dev Loop

For iterating on Go services (primarily `apiserver` and `controllermgr`) inside a running sandbox. Requires a sandbox already running — if not, run `ma sandbox create` first (see `/ma-sandbox-setup`).

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
```

Run this once at the start of your shell session, or prefix paths below with the result.

## Full build + deploy sequence

The steps below use `apiserver` as the example. See [Adapting for controllermgr](#adapting-for-controllermgr) for the controllermgr-specific substitutions.

```bash
# 1. Build statically-linked linux/arm64 binary (CGO_ENABLED=0 required — Distroless base has no libc)
cd "$REPO_ROOT/go"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/apiserver-local ./cmd/apiserver/

# 2. Copy config (only needed once, or when config files change)
cp -r "$REPO_ROOT/go/cmd/apiserver/config" /tmp/apiserver-config

# 3. Prepare a clean build context and build Docker image
mkdir -p /tmp/apiserver-build
cp /tmp/apiserver-local /tmp/apiserver-build/apiserver-local
cp -r /tmp/apiserver-config /tmp/apiserver-build/apiserver-config
docker build \
  -f "$REPO_ROOT/docker/service.Dockerfile" \
  --build-arg BINARY_PATH=apiserver-local \
  --build-arg CONFIG_PATH=apiserver-config \
  --platform linux/arm64 \
  --label git.branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)" \
  --label git.sha="$(git -C "$REPO_ROOT" rev-parse --short HEAD)" \
  --label git.dirty="$(git -C "$REPO_ROOT" diff --quiet || echo true)" \
  -t michelangelo-apiserver:local \
  /tmp/apiserver-build/

# 4. Import image into k3d cluster
k3d image import michelangelo-apiserver:local -c michelangelo-sandbox

# 5. Sync sandbox with local image override
# IMPORTANT: pass --set images.apiserver=... on EVERY sync call — helm's --reuse-values
# silently reverts to the default ghcr.io image if you omit it
source "$REPO_ROOT/python/.venv/bin/activate"
ma sandbox sync \
  --set images.apiserver=michelangelo-apiserver:local \
  --set images.pullPolicy=IfNotPresent \
  --set ui.enabled=true \
  --set envoy.enabled=true \
  --set controllermgr.enabled=true \
  --set worker.enabled=true
```

## Hot-swap (subsequent iterations)

Check whether config files changed since the last sync:

```bash
git diff --name-only HEAD | grep 'cmd/apiserver/config/'
```

**IF** that command returns no output — config is unchanged. Skip steps 2–3 (config copy) and run only the binary rebuild + import:

```bash
# Rebuild binary, re-import, rollout restart
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/apiserver-local "$REPO_ROOT/go/cmd/apiserver/"
cp /tmp/apiserver-local /tmp/apiserver-build/apiserver-local
docker build \
  -f "$REPO_ROOT/docker/service.Dockerfile" \
  --build-arg BINARY_PATH=apiserver-local \
  --build-arg CONFIG_PATH=apiserver-config \
  --platform linux/arm64 \
  --label git.branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)" \
  --label git.sha="$(git -C "$REPO_ROOT" rev-parse --short HEAD)" \
  --label git.dirty="$(git -C "$REPO_ROOT" diff --quiet || echo true)" \
  -t michelangelo-apiserver:local \
  /tmp/apiserver-build/
k3d image import michelangelo-apiserver:local -c michelangelo-sandbox
kubectl rollout restart deployment/michelangelo-apiserver
kubectl rollout status deployment/michelangelo-apiserver --timeout=60s
```

## Adapting for controllermgr

Substitute these values in every step above:

| apiserver | controllermgr |
|-----------|--------------|
| `./cmd/apiserver/` | `./cmd/controllermgr/` |
| `/tmp/apiserver-local` | `/tmp/controllermgr-local` |
| `/tmp/apiserver-config` | `/tmp/controllermgr-config` |
| `/tmp/apiserver-build/` | `/tmp/controllermgr-build/` |
| `michelangelo-apiserver:local` | `michelangelo-controllermgr:local` |
| `--set images.apiserver=...` | `--set images.controllermgr=...` |
| `deployment/michelangelo-apiserver` | `deployment/michelangelo-controllermgr` |
| `app.kubernetes.io/component=apiserver` | `app.kubernetes.io/component=controllermgr` |

Check whether `cmd/controllermgr/` has a `config/` directory — if not, omit steps 2 and the `CONFIG_PATH` build arg.

## Verify what's deployed

The `:local` tag carries no inherent identity — verify across three signals.

**1. It's your local build, not the released image:**

```bash
kubectl get pod -l app.kubernetes.io/component=apiserver \
  -o jsonpath='{.items[0].spec.containers[0].image}'
```

Should return `michelangelo-apiserver:local`, not `ghcr.io/michelangelo-ai/apiserver:main`.

**2. Which branch/commit it was built from** (requires the `--label` flags above):

```bash
docker image inspect michelangelo-apiserver:local --format '{{json .Config.Labels}}'
# {"git.branch":"craig.marker/fix-...","git.sha":"d1046e10","git.dirty":"true"}
```

The labels are baked into the image, so they survive `k3d image import` — inspecting the
local image tells you what the imported (and running) image is.

**3. It's your *latest* build, not a stale pod** — compare image build time to pod start
time. If the pod started *before* your last build, you forgot to `rollout restart`:

```bash
docker image inspect michelangelo-apiserver:local --format 'built: {{.Created}}'
kubectl get pod -l app.kubernetes.io/component=apiserver \
  -o jsonpath='started: {.items[0].status.startTime}{"\n"}'
```
