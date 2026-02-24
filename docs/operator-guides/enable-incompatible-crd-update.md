# Enabling Incompatible CRD Updates (Flipr) for a Given CRD

## Overview

When the Michelangelo API server starts, it syncs Custom Resource Definitions (CRDs) from the proto-generated schemas to the Kubernetes cluster. If a CRD schema change is **backward incompatible** and there are **existing instances** of that CRD in the cluster, the sync will fail by default to protect production data.

The **incompatible update allow list** (flipr) lets you explicitly opt-in specific CRDs to bypass this safety check, allowing incompatible schema changes to be applied even when existing instances are present.

## When Do You Need This?

You need to enable the flipr for a CRD when **all** of the following are true:

1. You are making a **backward-incompatible** schema change to a CRD (see [What Counts as Incompatible?](#what-counts-as-incompatible))
2. There are **existing instances** of that CRD in the cluster
3. You want the CRD update to proceed despite the incompatibility

> **Note:** If the CRD has **no existing instances** in the cluster, incompatible changes are applied automatically without needing the flipr.

## What Counts as Incompatible?

The CRD sync engine compares the old and new OpenAPI v3 schemas and flags the following as **incompatible** changes:

| Change Type | Compatible? | Example |
|---|---|---|
| Add a new field | ✅ Yes | Adding a new `description` field to `ProjectSpec` |
| Remove an existing field | ❌ No | Removing `pipeline` from `TriggerRunSpec` |
| Change a field's type | ❌ No | Changing `replicas` from `integer` to `string` |
| Remove items from `oneOf`/`anyOf`/`allOf` | ❌ No | Removing a variant from a union type |
| Add items to `oneOf`/`anyOf`/`allOf` | ✅ Yes | Adding a new variant to a union type |
| Change validation rules (min, max, format) | ✅ Yes | Tightening a `maximum` constraint |
| Change documentation fields (description, title, example) | ✅ Yes | Updating a field description |

These rules apply **recursively** through the entire schema hierarchy (nested objects, arrays, etc.).

## How It Works

### Architecture

```
┌─────────────────────────────────────────────────────┐
│                   API Server Startup                 │
│                                                      │
│  main.go                                             │
│  ┌─────────────────────────────────────────────┐     │
│  │ crd.SyncCRDs(                               │     │
│  │   group,                                    │     │
│  │   incompatibleUpdateAllowList: []string{    │     │
│  │     "deployments.michelangelo.api",  ◄──────┼──── Flipr: CRDs listed here
│  │   },                                        │     │        bypass the safety check
│  │   yamlSchemas,                              │     │
│  │ )                                           │     │
│  └──────────────────┬──────────────────────────┘     │
│                     │                                │
│                     ▼                                │
│  ┌─────────────────────────────────────────────┐     │
│  │ For each CRD:                               │     │
│  │   1. Compare old vs new schema              │     │
│  │   2. If incompatible AND has instances:      │     │
│  │      - In allow list? → Update proceeds ✅   │     │
│  │      - Not in list?   → Error, abort ❌      │     │
│  └─────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────┘
```

### Decision Flow

For each CRD during sync, the system follows this logic:

```
Schema changed?
├── No  → Skip (no update needed)
└── Yes → Is the change compatible?
    ├── Yes → Update CRD ✅
    └── No  → Is CRD in the incompatible update allow list?
        ├── Yes → Update CRD ✅ (flipr enabled)
        └── No  → Are there existing instances?
            ├── No  → Update CRD ✅ (safe, no data at risk)
            └── Yes → ERROR ❌ (abort to protect data)
```

## Step-by-Step: Enabling the Flipr for a CRD

### Step 1: Identify the CRD Name

CRD names follow the Kubernetes convention: `<plural>.<group>`. All Michelangelo CRDs use the group `michelangelo.api`.

| CRD Kind | CRD Name (for allow list) |
|---|---|
| CachedOutput | `cachedoutputs.michelangelo.api` |
| Cluster | `clusters.michelangelo.api` |
| Deployment | `deployments.michelangelo.api` |
| InferenceServer | `inferenceservers.michelangelo.api` |
| Model | `models.michelangelo.api` |
| ModelFamily | `modelfamilys.michelangelo.api` |
| Pipeline | `pipelines.michelangelo.api` |
| PipelineRun | `pipelineruns.michelangelo.api` |
| Project | `projects.michelangelo.api` |
| RayCluster | `rayclusters.michelangelo.api` |
| RayJob | `rayjobs.michelangelo.api` |
| Revision | `revisions.michelangelo.api` |
| SparkJob | `sparkjobs.michelangelo.api` |
| TriggerRun | `triggerruns.michelangelo.api` |

> **Tip:** You can verify the exact CRD name by checking the generated YAML in `proto-go/api/v2/<resource>.pb.go` — look for the `metadata.name` field inside `YamlSchemas["<Kind>"]`.

### Step 2: Add the CRD to the Allow List in Code

Edit the `SyncCRDs` call in the appropriate service's `main.go`:

**For the API Server** (`go/cmd/apiserver/main.go`):

```go
// Before (default — no incompatible updates allowed):
crd.SyncCRDs(v2pb.GroupVersion.Group,
    []string{},
    v2pb.YamlSchemas),

// After (enabling incompatible updates for Deployment CRD):
crd.SyncCRDs(v2pb.GroupVersion.Group,
    []string{
        "deployments.michelangelo.api",
    },
    v2pb.YamlSchemas),
```

You can add **multiple CRDs** to the allow list at once:

```go
crd.SyncCRDs(v2pb.GroupVersion.Group,
    []string{
        "deployments.michelangelo.api",
        "pipelineruns.michelangelo.api",
    },
    v2pb.YamlSchemas),
```

### Step 3: Build and Deploy

Build the API server with your changes:

```bash
bazel run //go/cmd/apiserver:apiserver
```

When the API server starts, it will log the CRD sync activity:

```
INFO  CRD sync config  {"config": {"EnableCRDUpdate": true, ...}}
INFO  Compare CRD in cluster with new CRD definition, and conditionally update CRDs
INFO  CRD exists, compare CRD schema  {"name": "deployments.michelangelo.api"}
INFO  Update CRD definition.           {"name": "deployments.michelangelo.api"}
INFO  CRD updated                      {"name": "deployments.michelangelo.api"}
```

### Step 4: Remove from Allow List After Rollout

> ⚠️ **Important:** Once the incompatible change has been rolled out to all clusters, **remove** the CRD from the allow list to restore the safety check.

```go
// Revert back to empty allow list
crd.SyncCRDs(v2pb.GroupVersion.Group,
    []string{},
    v2pb.YamlSchemas),
```

## Configuration Reference

### CRD Sync Config (`config/base.yaml`)

The CRD sync behavior is also controlled by the YAML configuration:

```yaml
apiserver:
  crdSync:
    enableCRDUpdate: true       # Master switch: set to false to disable all CRD updates
    enableCRDDeletion: true     # Allow deleting CRDs no longer defined in schemas
    crdVersions:                # Multi-version configuration
      projects.michelangelo.api:
        versions: [v2]
        storageVersion: v2
```

| Config Key | Type | Description |
|---|---|---|
| `enableCRDUpdate` | `bool` | Master switch for CRD sync. When `false`, no CRDs are created/updated/deleted. |
| `enableCRDDeletion` | `bool` | When `true`, CRDs in the cluster but not in schemas will be deleted (if no instances exist). |
| `crdVersions` | `map` | Per-CRD version configuration for multi-version CRDs. |

### Retry Behavior

The CRD upsert operation uses exponential backoff with up to **3 retries** for transient errors (conflict, server timeout, too many requests, internal errors, service unavailable).

## Safety Considerations

1. **Data Loss Risk:** Incompatible changes can cause existing CRD instances to become invalid or lose data. Ensure you have a migration plan for existing resources.

2. **Temporary Override:** The allow list should be treated as a **temporary** measure. Remove the CRD from the allow list as soon as the rollout is complete.

3. **Test First:** Always test incompatible changes in a sandbox/staging environment before applying to production. Use the Michelangelo sandbox:
   ```bash
   # Start sandbox for testing
   ma sandbox start
   ```

4. **Check Instances:** Before enabling the flipr, verify how many instances exist:
   ```bash
   kubectl get <resource> --all-namespaces --no-headers | wc -l
   # e.g., kubectl get deployments.michelangelo.api --all-namespaces --no-headers | wc -l
   ```

5. **Monitor Logs:** After deploying, watch the API server logs for CRD sync errors:
   ```bash
   kubectl logs -f <apiserver-pod> | grep -i "crd"
   ```

## Troubleshooting

### Error: "Schema is incompatible, and there are existing instances"

```
failed to update CRD. Schema is incompatible, and there are existing instances. Abort updating CRD <name>
```

**Cause:** You made an incompatible schema change, and the CRD has existing instances, but the CRD is not in the allow list.

**Fix:** Add the CRD to the `incompatibleUpdateAllowList` in `SyncCRDs()` as described in [Step 2](#step-2-add-the-crd-to-the-allow-list-in-code).

### Error: "CRD has version X that is not in the new CRD"

```
CRD <name> has version v1 that is not in the new CRD
```

**Cause:** You removed a version from the CRD schema that still exists in the cluster. Michelangelo does not support removing CRD versions.

**Fix:** Keep all existing versions in the schema. Add new versions alongside existing ones using the `crdVersions` configuration.

### CRD Update Silently Skipped

If you don't see any update logs for a CRD, check:
1. `enableCRDUpdate` is set to `true` in `config/base.yaml`
2. The schema actually has changes (the system skips CRDs with no diff)

## Related Resources

- [How to Write APIs](../contributing/how-to-write-apis.md) — Guide for defining new CRD schemas in proto
- [API Framework](api-framework.md) — Overview of the Michelangelo API framework
- [Multi-Version CRD Support](../../proto/test/api/README.md) — Guide for setting up multi-version CRDs with webhook conversion
