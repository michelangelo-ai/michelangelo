# Custom Docker Images for Feature Branches and Sandbox Testing

> **Who is this for?** This guide is for **contributors** developing Michelangelo's core platform services (API server, controller manager, worker). If you're building ML pipelines using the Michelangelo SDK, you don't need custom Docker images -- the default sandbox images work out of the box.

This guide explains two ways to build and test custom images:

- **[Option A — Local build](#option-a--local-build-no-github-push-required)**: build on your machine, import directly into k3d. Fast iteration loop — no GitHub push or CI required.
- **[Option B — CI build](#option-b--ci-build-via-github-actions)**: push your branch, let GitHub Actions build multi-arch images and push them to GHCR, then point the sandbox at those images.

---

## Option A — Local build (no GitHub push required)

Use this option when you want a fast feedback loop or are not ready to push your branch.

### Prerequisites

- Docker with BuildKit enabled (Docker Desktop ≥ 4.x)
- `k3d` — a sandbox cluster must already exist or you will create one in step 5
- The bazel-managed Go 1.24 binary (downloaded automatically on the first `bazel build`):
  ```bash
  # Locate it after any bazel build has run
  find /private/var/tmp/_bazel_$(whoami) -name "go" -path "*/rules_go~~go_sdk~*/bin/go" 2>/dev/null | head -1
  ```
  Set a shell variable for convenience:
  ```bash
  export GOBIN=$(find /private/var/tmp/_bazel_$(whoami) -name "go" -path "*/rules_go~~go_sdk~*/bin/go" 2>/dev/null | head -1)
  ```

### Step 1: Build the binaries

Run all three builds in parallel from the repo root. The binaries must target `linux/arm64` (Apple Silicon) or `linux/amd64` (Intel/CI).

```bash
cd go

# Apple Silicon (arm64)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $GOBIN build -o ../apiserver     ./cmd/apiserver     &
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $GOBIN build -o ../controllermgr ./cmd/controllermgr &
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $GOBIN build -o ../worker        ./cmd/worker        &
wait && echo "All builds done"
```

> **Why `CGO_ENABLED=0`?** The distroless base image has no C runtime. Disabling CGO produces a fully static binary that works without it.

### Step 2: Build the Docker images

```bash
cd ..   # repo root

for svc in apiserver controllermgr worker; do
  docker build --platform linux/arm64 \
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
poetry run ma sandbox delete
poetry run ma sandbox create --workflow cadence   # or --workflow temporal
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
cd go && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $GOBIN build -o ../worker ./cmd/worker
cd ..
docker build --platform linux/arm64 -f docker/service.Dockerfile \
  --build-arg BINARY_PATH=worker \
  --build-arg CONFIG_PATH=go/cmd/worker/config \
  -t ghcr.io/michelangelo-ai/worker:local-dev --quiet .
k3d image import ghcr.io/michelangelo-ai/worker:local-dev -c michelangelo-sandbox
kubectl rollout restart deployment/michelangelo-worker
kubectl rollout status  deployment/michelangelo-worker
```

---

## Option B — CI build via GitHub Actions

Use this option when your branch is ready to share, you need multi-arch images (`linux/amd64` + `linux/arm64`), or you want a stable image tag to share with teammates.

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

### 4) Wait for images to be published
- Monitor the GitHub Actions run for `Dev Release` on your branch.
- Upon success, images will be available as:
  - `ghcr.io/michelangelo-ai/apiserver:my-feature-branch`
  - `ghcr.io/michelangelo-ai/controllermgr:my-feature-branch`
  - `ghcr.io/michelangelo-ai/worker:my-feature-branch`

### 5) Update sandbox manifests to use your new image tag
Edit the following files to set the image tag to your branch name:
- `python/michelangelo/cli/sandbox/resources/michelangelo-apiserver.yaml`
- `python/michelangelo/cli/sandbox/resources/michelangelo-controllermgr.yaml`
- `python/michelangelo/cli/sandbox/resources/michelangelo-worker.yaml`

Example (replace `my-feature-branch` with your branch):
```yaml
# michelangelo-apiserver.yaml
spec:
  containers:
    - name: michelangelo-apiserver
      image: ghcr.io/michelangelo-ai/apiserver:my-feature-branch
```

```yaml
# michelangelo-controllermgr.yaml
spec:
  containers:
    - name: app
      image: ghcr.io/michelangelo-ai/controllermgr:my-feature-branch
```

```yaml
# michelangelo-worker.yaml
spec:
  containers:
    - name: app
      image: ghcr.io/michelangelo-ai/worker:my-feature-branch
```

Note: The repository already contains working examples where the image tag equals the branch name.

### 6) Start the sandbox to test your changes
From the `python/` directory in the repo root:
```bash
poetry install
source .venv/bin/activate
ma sandbox create
```

Useful operations:
- Recreate: `ma sandbox delete && ma sandbox create`
- Inspect: `kubectl get pods -A | grep michelangelo`
- Logs (example): `kubectl logs pod/michelangelo-controllermgr -f`

### 7) Verify deployment
- Ensure the pods for `apiserver`, `controllermgr`, and `worker` are running.
- Confirm they are using your branch image tags via `kubectl describe pod <pod-name>`.
- Exercise your changes via the sandbox workflows or APIs as needed.

### Troubleshooting
- Builds not triggering: Confirm `.github/workflows/dev-release.yml` includes your branch under `on.push.branches` and that you pushed to the exact branch name.
- Image pull errors: Ensure the action completed successfully and images exist at `ghcr.io/michelangelo-ai`. If private, verify permissions for your cluster's image puller.
- Wrong image tag: Double-check manifests reference your exact branch name.
- Multi-arch issues: The workflow builds `linux/amd64` and `linux/arm64`. Confirm your cluster nodes match one of these.

### Cleanup
```bash
ma sandbox delete
```
