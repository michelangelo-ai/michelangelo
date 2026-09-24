---
sidebar_position: 6
sidebar_label: "Cascade Delete"
---

# Pipeline Cascade Delete

Deleting a Pipeline cascades to all of its child resources — PipelineRuns, TriggerRuns, and Revisions — in a single operation. This is the **default behavior**: there is no feature flag to enable. PipelineRuns and TriggerRuns each have an in-flight workflow (Spark/Ray job, Cadence/Temporal schedule) that is cancelled gracefully first, with its final state preserved in MySQL, before the child is removed. Revisions carry no in-flight work — they are pure snapshots — so they are simply soft-deleted (see [Revisions are soft-deleted, not retained](#revisions-are-soft-deleted-not-retained)).

It is built on Kubernetes [foreground garbage collection](https://kubernetes.io/docs/concepts/architecture/garbage-collection/#foreground-deletion): the Pipeline is not removed until all children have been drained and deleted.

There is exactly one cascade relationship: `Pipeline → {PipelineRun, TriggerRun, Revision}`. Deleting any other entity (Model, Deployment, …) does not cascade.

:::warning
Cascade is on by default and **irreversible** — deleting a Pipeline terminates and removes all of its child runs, and soft-deletes its Revisions. To keep the children, delete with the `orphan` propagation policy (see [Controlling cascade behavior](#controlling-cascade-behavior)).
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
**Garbage collection bypasses child-level RBAC.** The GC deletes children with the **controller's** permissions, not the caller's. A user who is permitted to delete a Pipeline therefore causes all of its PipelineRuns, TriggerRuns, and Revisions to be deleted **even if that user has no delete permission on the children themselves**. Child-level RBAC does **not** protect children from cascade. Restrict Pipeline-delete access accordingly.
:::

## How it works

1. **User deletes a Pipeline** with `foreground` propagation (the CLI default).
2. **Kubernetes GC stamps children for deletion.** All three child kinds carry `ownerReferences` to the Pipeline (with `blockOwnerDeletion: true`). PipelineRuns and TriggerRuns get theirs stamped at creation via API hooks; Revisions get theirs stamped by the Pipeline controller itself, in the same reconcile that snapshots the Revision CR (Revisions have no external Create RPC to hook into — the Pipeline controller creates them directly). GC sets a `deletionTimestamp` on each child once stamped.
3. **Drain finalizers cancel in-flight work — PipelineRun and TriggerRun only.** A PipelineRun's drain finalizer cancels its Cadence/Temporal workflow; a TriggerRun's deletes the Temporal schedule (or Cadence cron) and terminates any open run. The finalizer is removed once the run reaches a terminal state. Revisions carry no in-flight workflow (they are immutable snapshots of pipeline content), so they have no drain finalizer and are deleted as soon as GC processes them.
4. **Ingester finalizes each child.** For PipelineRun/TriggerRun: while the drain finalizer is present, the ingester refreshes MySQL with the current state; once it is removed, the ingester upserts the final state and removes its own finalizer, deleting the child from etcd — the run history therefore survives in MySQL. For Revision: the ingester has no retain policy for this kind, so it soft-deletes the MySQL row and removes its finalizer in one step (see [Revisions are soft-deleted, not retained](#revisions-are-soft-deleted-not-retained)).
5. **Pipeline deletion completes** once all children are gone (under `foreground`).

## Drain finalizers

| Finalizer | Applied to | Purpose |
|-----------|-----------|---------|
| `pipelineruns.michelangelo.uber.com/drain` | PipelineRun | Cancels the pipeline workflow before GC deletes the CR |
| `triggerruns.michelangelo.uber.com/drain` | TriggerRun | Deletes the cron schedule and terminates any open workflow run |

Revision has no drain finalizer — it carries no in-flight work, so there is nothing to cancel before GC removes it.

A drain finalizer is installed **before** the ownerReference, so a PipelineRun or TriggerRun can never become GC-eligible without its drain finalizer in place. Newly created PipelineRuns/TriggerRuns receive the ownerReference at creation via API hooks; children predating the hook are backfilled once during reconciliation (a transitional migration). Revisions receive their ownerReference in the Pipeline controller's reconcile loop going forward; **there is no backfill for Revisions created before this cascade relationship existed** — pre-existing Revisions are not retroactively owned and will not be cascade-deleted by their Pipeline.

## Revisions are soft-deleted, not retained

PipelineRun and TriggerRun are **retain** kinds: the ingester preserves their final state in MySQL when GC removes the CR. Revision is deliberately **not** a retain kind — cascade-deleted Revisions are **soft-deleted** by the ingester's default path, the same as any other non-retained kind removed outside the API server. There is no equivalent to the "final state" preservation PipelineRun/TriggerRun get; the Revision's MySQL row is simply marked deleted.

## Safety timeout

The drain safety timeout is **per child**. Each child's drain finalizer enforces its own **24-hour** timeout, keyed off **that child's** `deletionTimestamp`: if a drain has not completed within 24 hours of when the child was stamped for deletion, the child performs a best-effort engine teardown, force-removes its drain finalizer regardless of workflow state, and lets GC proceed. The timeout is hard-coded (24h) and not configurable. There is no Pipeline-level timeout — each run drains and times out independently.

:::warning
The 24-hour safety timeout is a last resort — it removes the drain finalizer even if the workflow is still running. Under normal operation, drains complete in minutes. If the timeout fires, investigate why the drain was wedged via the controller manager logs and the `cascade_child_drain_timeout_total` metric.
:::

## Metrics

Four `cascade_*` metrics (owner-ref backfills, drain duration, drain timeouts, active drains) are always emitted, labeled by `kind` (`pipeline_run` or `trigger_run`). These are drain-specific, so they are not emitted for Revision, which has no drain phase. See [Monitoring & Observability](operations/monitoring.md#cascade-delete) for the metric reference, scrape configuration, and alert rules.

## Limitations

- **Pipeline-only scope.** Deleting a Model, Deployment, or other entity type does not cascade.
- **Children in scope.** PipelineRun, TriggerRun, and Revision are treated as Pipeline children. PipelineRun and TriggerRun get drain-then-retain treatment; Revision is soft-deleted immediately with no drain phase (see [Revisions are soft-deleted, not retained](#revisions-are-soft-deleted-not-retained)).
- **ownerReference stamping.** PipelineRun/TriggerRun ownerReferences are stamped at creation via API hooks; CRs predating the hook are backfilled once during reconciliation (a transitional migration). Revision ownerReferences are stamped by the Pipeline controller on reconcile going forward, with **no backfill** for Revisions that already existed before this cascade relationship shipped — those will not cascade-delete with their Pipeline.

## User experience

```
$ ma pipeline delete -n my-project --name training-pipeline

 ! WARNING: deleting pipeline 'training-pipeline' will also terminate
   and permanently delete all of its child runs (PipelineRuns,
   TriggerRuns) and soft-delete its Revisions. This cannot be undone.
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
