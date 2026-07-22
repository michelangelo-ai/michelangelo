# Custom Docker Images for Feature Branches and Sandbox Testing

> **Who is this for?** This guide is for **contributors** developing Michelangelo AI's core platform services (API server, controller manager, worker). If you're building ML pipelines using the Michelangelo AI SDK, you don't need custom Docker images — the default sandbox images work out of the box. See the [Sandbox Setup Guide](../getting-started/sandbox-setup.md) for getting started.

This guide explains two ways to build and test custom images:

- **[Option A — Local build](#option-a--local-build-no-github-push-required)**: build on your machine, import directly into k3d. Fast iteration loop — no GitHub push or CI required.
- **[Option B — CI build](#option-b--ci-build-via-github-actions)**: push your branch, let GitHub Actions build multi-arch images and push them to GHCR, then point the sandbox at those images.

---

## Option A — Local build (no GitHub push required)

Use this option when you want a fast feedback loop or are not ready to push your branch.

### Prerequisites

- Docker with BuildKit enabled (Docker Desktop ≥ 4.x)
- A sandbox cluster (create one with `ma sandbox create` — see [Sandbox Setup](../getting-started/sandbox-setup.md))
- The bazel-managed Go 1.24 binary (downloaded automatically on the first `bazel build`):
  ```bash
  # macOS
  find /private/var/tmp/_bazel_$(whoami) -name "go" -path "*/rules_go~~go_sdk~*/bin/go" 2>/dev/null | head -1

  # Linux
  find ~/.cache/bazel -name "go" -path "*/rules_go~~go_sdk~*/bin/go" 2>/dev/null | head -1
  ```
  Set a shell variable for convenience:
  ```bash
  export GOEXEC=$(find /private/var/tmp/_bazel_$(whoami) -name "go" -path "*/rules_go~~go_sdk~*/bin/go" 2>/dev/null | head -1)
  # Linux: replace /private/var/tmp/_bazel_$(whoami) with ~/.cache/bazel
  ```

### Step 1: Build the binaries

Run all three builds in parallel from the repo root. Set `GOARCH` to match your machine (`arm64` for Apple Silicon, `amd64` for Intel/Linux).

```bash
cd go

ARCH=arm64   # or amd64 for Intel/Linux

GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 $GOEXEC build -o ../apiserver     ./cmd/apiserver     &
GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 $GOEXEC build -o ../controllermgr ./cmd/controllermgr &
GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 $GOEXEC build -o ../worker        ./cmd/worker        &
wait && echo "All builds done"
```

> **Why `CGO_ENABLED=0`?** The distroless base image has no C runtime. Disabling CGO produces a fully static binary that works without it.

### Step 2: Build the Docker images

```bash
cd ..   # repo root

for svc in apiserver controllermgr worker; do
  docker build --platform linux/$ARCH \
    -f docker/service.Dockerfile \
    --build-arg BINARY_PATH=$svc \
    --build-arg CONFIG_PATH=go/cmd/$svc/config \
    -t ghcr.io/michelangelo-ai/$svc:local-dev \
    --quiet . && echo "$svc image: OK" &
done
wait && echo "All images built"
```

### Step 3: Delete and recreate the sandbox

```bash
cd python
ma sandbox delete
ma sandbox create --workflow cadence   # or --workflow temporal
```

### Step 4: Import images into k3d

```bash
k3d image import \
  ghcr.io/michelangelo-ai/apiserver:local-dev \
  ghcr.io/michelangelo-ai/controllermgr:local-dev \
  ghcr.io/michelangelo-ai/worker:local-dev \
  -c michelangelo-sandbox
```

### Step 5: Point deployments at the local images

```bash
kubectl set image deployment/michelangelo-apiserver    apiserver=ghcr.io/michelangelo-ai/apiserver:local-dev
kubectl set image deployment/michelangelo-controllermgr app=ghcr.io/michelangelo-ai/controllermgr:local-dev
kubectl set image deployment/michelangelo-worker        app=ghcr.io/michelangelo-ai/worker:local-dev
```

> **Note:** `kubectl set image` is ephemeral — running `ma sandbox sync` or `helm upgrade` will revert the images to the Helm defaults. For a durable override, pass `--set` flags when creating the sandbox:
> ```bash
> ma sandbox create \
>   --set images.apiserver=ghcr.io/michelangelo-ai/apiserver:local-dev \
>   --set images.controllermgr=ghcr.io/michelangelo-ai/controllermgr:local-dev \
>   --set images.worker=ghcr.io/michelangelo-ai/worker:local-dev
> ```

### Step 6: Verify

```bash
kubectl rollout status deployment/michelangelo-apiserver \
  deployment/michelangelo-controllermgr \
  deployment/michelangelo-worker
kubectl get pods -l app.kubernetes.io/instance=michelangelo
```

All three pods should reach `Running`. Confirm the image is the local build:

```bash
kubectl describe pod -l app.kubernetes.io/component=apiserver | grep Image:
```

### Iterating on changes

After each code change, rebuild only the affected binary and image, then re-import and restart that one pod:

```bash
# Example: rebuilding only the worker
cd go && GOOS=linux GOARCH=$ARCH CGO_ENABLED=0 $GOEXEC build -o ../worker ./cmd/worker
cd ..
docker build --platform linux/$ARCH -f docker/service.Dockerfile \
  --build-arg BINARY_PATH=worker \
  --build-arg CONFIG_PATH=go/cmd/worker/config \
  -t ghcr.io/michelangelo-ai/worker:local-dev --quiet .
k3d image import ghcr.io/michelangelo-ai/worker:local-dev -c michelangelo-sandbox
kubectl rollout restart deployment/michelangelo-worker
kubectl rollout status  deployment/michelangelo-worker
```

### Troubleshooting

- **Pod stuck in `ImagePullBackOff`**: The default `pullPolicy` is `IfNotPresent`. If k3d can't find the image, make sure you ran `k3d image import` after building. Check with `k3d image ls -c michelangelo-sandbox | grep local-dev`.
- **`exec format error`**: Architecture mismatch — you built for `arm64` but the k3d node is `amd64` (or vice versa). Rebuild with the correct `ARCH`.
- **Bazel Go binary not found**: Run any `./tools/bazel build` target first to trigger the SDK download, then re-run the `find` command.

### Cleanup

Remove the locally built binaries from the repo root:

```bash
rm -f apiserver controllermgr worker
```

To tear down the sandbox:

```bash
ma sandbox delete
```

---

## Option B — CI build via GitHub Actions

Use this option when your branch is ready to share, you need multi-arch images (`linux/amd64` + `linux/arm64`), or you want a stable image tag to share with teammates.

### Prerequisites

- Push access to the repository (or a fork with GitHub Actions enabled)
- GHCR pull access from your k3d cluster (public images work automatically)

### 1) Create or switch to your feature branch
```bash
git checkout -b my-feature-branch
# or
git checkout my-feature-branch
```

### 2) Update the dev release workflow to build images from your branch
Edit `.github/workflows/dev-release.yml` and set the `on.push.branches` list to your branch name:

```yaml
on:
  workflow_dispatch:
  push:
    branches: [ my-feature-branch ]
```

- The workflow builds multi-arch images for these services via a matrix: `controllermgr`, `worker`, and `apiserver`.
- Images are pushed to `ghcr.io/michelangelo-ai/<service>` and tagged automatically, including a tag matching your branch name (via `type=ref,event=branch`).

### 3) Commit changes and push your branch to trigger the build
```bash
git add .github/workflows/dev-release.yml
git commit -m "Enable dev release for my-feature-branch"
git push origin $(git branch --show-current)
```

> **Caution**: Only use `git push -f` (force push) if you intentionally need to overwrite remote history. In most cases, a regular `git push` is sufficient and safer.

> **Important**: Don't merge the `dev-release.yml` change into `main` — it's only for your branch. Revert it before opening a PR, or exclude it from your PR's commits.

### 4) Wait for images to be published
- Monitor the GitHub Actions run for `Dev Release` on your branch.
- Upon success, images will be available as:
  - `ghcr.io/michelangelo-ai/apiserver:my-feature-branch`
  - `ghcr.io/michelangelo-ai/controllermgr:my-feature-branch`
  - `ghcr.io/michelangelo-ai/worker:my-feature-branch`

### 5) Point the sandbox at your branch images

The sandbox deploys core services via the Helm chart at `helm/michelangelo/`. Override the image tags using `--set` when creating the sandbox:

```bash
cd python
ma sandbox create \
  --set images.apiserver=ghcr.io/michelangelo-ai/apiserver:my-feature-branch \
  --set images.controllermgr=ghcr.io/michelangelo-ai/controllermgr:my-feature-branch \
  --set images.worker=ghcr.io/michelangelo-ai/worker:my-feature-branch
```

Or, if the sandbox is already running, import and swap the images:

```bash
kubectl set image deployment/michelangelo-apiserver    apiserver=ghcr.io/michelangelo-ai/apiserver:my-feature-branch
kubectl set image deployment/michelangelo-controllermgr app=ghcr.io/michelangelo-ai/controllermgr:my-feature-branch
kubectl set image deployment/michelangelo-worker        app=ghcr.io/michelangelo-ai/worker:my-feature-branch
```

### 6) Verify deployment
- Ensure the pods for `apiserver`, `controllermgr`, and `worker` are running.
- Confirm they are using your branch image tags via `kubectl describe pod <pod-name> | grep Image:`.
- Exercise your changes via the sandbox workflows or APIs as needed.

### Troubleshooting
- **Builds not triggering**: Confirm `.github/workflows/dev-release.yml` includes your branch under `on.push.branches` and that you pushed to the exact branch name.
- **Image pull errors**: Ensure the action completed successfully and images exist at `ghcr.io/michelangelo-ai`. If private, verify permissions for your cluster's image puller.
- **Wrong image tag**: Double-check your `--set` flags reference the exact branch name.
- **Multi-arch issues**: The workflow builds `linux/amd64` and `linux/arm64`. Confirm your cluster nodes match one of these.

### Cleanup
```bash
ma sandbox delete
```

---

## Next Steps

- [Sandbox Setup Guide](../getting-started/sandbox-setup.md) — create and configure the local development environment
- [Building from Source](building-michelangelo-ai-from-source.md) — full build instructions for all components
- [Helm Chart Reference](../operator-guides/helm-chart.md) — `values.yaml` reference including `images.*` overrides
