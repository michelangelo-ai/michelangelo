---
name: ma-sandbox-reset
description: Tear down the Michelangelo sandbox cluster and recreate it from scratch. Use when the cluster is in a broken state, after major schema changes, or when you want a clean environment. Triggers on "reset sandbox", "start fresh", "sandbox is broken". For service-level issues without nuking the cluster, use /ma-sandbox-debug instead.
user-invocable: true
---

# Sandbox Reset

Full teardown and recreation of the local Michelangelo sandbox. Use when `ma sandbox sync` or `ma sandbox health` can't recover the cluster.

## Step 1: Delete the cluster

```bash
REPO_ROOT=$(git rev-parse --show-toplevel)
source "$REPO_ROOT/python/.venv/bin/activate"   # or prefix every command with: poetry run
ma sandbox delete
```

Tears down the k3d cluster and all resources inside it. Takes ~30 seconds.

## Step 2: Recreate

```bash
ma sandbox create
```

## Step 3: Seed demo data (recommended)

```bash
cd "$REPO_ROOT/python"
poetry run ma sandbox demo pipeline
```

Without this the UI loads but shows no data. Skip only if you have a specific reason to start empty.

## Step 4: Verify

```bash
poetry run ma sandbox health
```

All checks should pass. Open the sandbox UI and confirm the seeded demo project is visible.
