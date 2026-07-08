---
sidebar_position: 6
sidebar_label: "Cascade Delete"
---

# Pipeline Cascade Delete

Deleting a Pipeline cascades to all of its child resources — PipelineRuns and TriggerRuns — in a single operation. This is the **default behavior**: there is no feature flag to enable. Each child's in-flight workflow (Spark/Ray job, Cadence/Temporal schedule) is cancelled gracefully first, its final state is preserved in MySQL, and only then is the child removed.

It is built on Kubernetes [foreground garbage collection](https://kubernetes.io/docs/concepts/architecture/garbage-collection/#foreground-deletion): the Pipeline is not removed until all children have been drained and deleted.

There is exactly one cascade relationship: `Pipeline → {PipelineRun, TriggerRun}`. Deleting any other entity (Model, Deployment, …) does not cascade.

:::warning
Cascade is on by default and **irreversible** — deleting a Pipeline terminates and removes all of its child runs. To keep the children, delete with the `orphan` propagation policy (see [Controlling cascade behavior](#controlling-cascade-behavior)).
:::

## Controlling cascade behavior

Cascade is not governed by any cluster-wide switch. Two standard Kubernetes mechanisms control it:

**1. Propagation policy (chosen per delete).** Whether children are removed — and whether the delete waits for them — is the delete's [propagation policy](https://kubernetes.io/docs/concepts/architecture/garbage-collection/#cascading-deletion):

| Policy | Effect | How to invoke |
|--------|--------|---------------|
| `foreground` (default) | Cascade and **wait**: the Pipeline is not removed until every child is drained and gone | `ma pipeline delete …` (the CLI default), or `kubectl delete pipeline <name> --cascade=foreground` |
| `background` | Cascade but **don't wait**: the Pipeline is removed immediately; the GC deletes children asynchronously | `kubectl delete pipeline <name> --cascade=background` |
| `orphan` | **Keep** the children: the Pipeline is removed and its runs are left in place with no parent | `kubectl delete pipeline <name> --cascade=orphan` |

The `ma pipeline delete` CLI always uses `foreground`. To opt out of cascade for a single delete, use `kubectl` with `--cascade=orphan`.

**2. RBAC (who may delete).** Who may delete a Pipeline is governed by Kubernetes RBAC.

:::danger
**Garbage collection bypasses child-level RBAC.** The GC deletes children with the **controller's** permissions, not the caller's. A user who is permitted to delete a Pipeline therefore causes all of its PipelineRuns and TriggerRuns to be deleted **even if that user has no delete permission on the runs themselves**. Child-level RBAC does **not** protect runs from cascade. Restrict Pipeline-delete access accordingly.
:::

## How it works

1. **User deletes a Pipeline** with `foreground` propagation (the CLI default).
2. **Kubernetes GC stamps children for deletion.** PipelineRuns and TriggerRuns carry `ownerReferences` to the Pipeline (with `blockOwnerDeletion: true`), stamped at creation via API hooks; GC sets a `deletionTimestamp` on each child.
3. **Drain finalizers cancel in-flight work.** A PipelineRun's drain finalizer cancels its Cadence/Temporal workflow; a TriggerRun's deletes the Temporal schedule (or Cadence cron) and terminates any open run. The finalizer is removed once the run reaches a terminal state.
4. **Ingester retains final state.** While a child's drain finalizer is present, the ingester refreshes MySQL with the current state; once it is removed, the ingester upserts the final state and removes its own finalizer, deleting the child from etcd. The run history therefore survives in MySQL.
5. **Pipeline deletion completes** once all children are gone (under `foreground`).

## Drain finalizers

| Finalizer | Applied to | Purpose |
|-----------|-----------|---------|
| `pipelineruns.michelangelo.uber.com/drain` | PipelineRun | Cancels the pipeline workflow before GC deletes the CR |
| `triggerruns.michelangelo.uber.com/drain` | TriggerRun | Deletes the cron schedule and terminates any open workflow run |

A drain finalizer is installed **before** the ownerReference, so a child can never become GC-eligible without its drain finalizer in place. Newly created children receive the ownerReference at creation via API hooks; children predating the hook are backfilled once during reconciliation (a transitional migration).

## Safety timeout

The drain safety timeout is **per child**. Each child's drain finalizer enforces its own **24-hour** timeout, keyed off **that child's** `deletionTimestamp`: if a drain has not completed within 24 hours of when the child was stamped for deletion, the child performs a best-effort engine teardown, force-removes its drain finalizer regardless of workflow state, and lets GC proceed. The timeout is hard-coded (24h) and not configurable. There is no Pipeline-level timeout — each run drains and times out independently.

:::warning
The 24-hour safety timeout is a last resort — it removes the drain finalizer even if the workflow is still running. Under normal operation, drains complete in minutes. If the timeout fires, investigate why the drain was wedged via the controller manager logs and the `cascade_child_drain_timeout_total` metric.
:::

## Metrics

Four `cascade_*` metrics (owner-ref backfills, drain duration, drain timeouts, active drains) are always emitted, labeled by `kind` (`pipeline_run` or `trigger_run`). See [Monitoring & Observability](operations/monitoring.md#cascade-delete) for the metric reference, scrape configuration, and alert rules.

## Limitations

- **Pipeline-only scope.** Deleting a Model, Deployment, or other entity type does not cascade.
- **Children in scope.** Only PipelineRun and TriggerRun are treated as Pipeline children.
- **ownerReference stamping.** ownerReferences are stamped at creation via API hooks; CRs predating the hook are backfilled once during reconciliation (a transitional migration).

## User experience

```
$ ma pipeline delete -n my-project --name training-pipeline

 ! WARNING: deleting pipeline 'training-pipeline' will also terminate
   and permanently delete all of its child runs (PipelineRuns,
   TriggerRuns). This cannot be undone.
 > delete pipeline 'training-pipeline'? [y/N] y
```

Pass `--yes` to skip the confirmation prompt (useful for scripting).

| Scenario | Outcome |
|----------|---------|
| Pipeline has active children (default `foreground`) | Children drained then deleted (see [How it works](#how-it-works)); typically **minutes** |
| No children (default `foreground`) | Pipeline deleted in **seconds** |
| A child's drain gets stuck | After **24h** that child's own safety timeout force-removes its drain finalizer, then deletion proceeds |
| `kubectl delete … --cascade=orphan` | Pipeline deleted immediately; children left in place (opt out of cascade) |
| `kubectl delete … --cascade=background` | Pipeline removed immediately; children GC'd asynchronously |

:::note
The MA Studio UI delete action does **not** yet cascade — it removes the Pipeline only and leaves child runs in place. Cascading from the UI is a planned follow-up. Until then, use `ma pipeline delete` (or `kubectl … --cascade=foreground`) when you need children removed.
:::

## Next steps

- [CLI Reference](../user-guides/reference/cli.md) — `ma pipeline delete` usage and examples
- [Pipeline Management](../user-guides/ml-pipelines/pipeline-management.md) — user-facing guide to deleting Pipelines
- [Monitoring & Observability](operations/monitoring.md) — scrape configuration and cascade alert rules
- [Troubleshooting](operations/troubleshooting.md) — diagnosing stuck cascade deletions
