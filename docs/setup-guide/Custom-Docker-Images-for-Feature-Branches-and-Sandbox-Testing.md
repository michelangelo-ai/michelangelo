# Custom Docker Images for Feature Branches and Sandbox Testing

This guide explains how to build and publish custom Docker images for a feature branch using the dev release workflow, update sandbox manifests to reference those images, and run the sandbox to test your changes.


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
# Force-push only if you intend to overwrite remote history
git push -f origin $(git branch --show-current)
```

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
From the repo root:
```bash
poetry install
poetry run ma sandbox create
```

Useful operations:
- Recreate: `poetry run ma sandbox delete && poetry run ma sandbox create`
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
poetry run ma sandbox delete
```
