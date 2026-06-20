# Upgrading Michelangelo

This file documents breaking changes that require action when upgrading between releases.

---

## PR #1283 — Notification system overhaul

### 1. Proto enum: `EVENT_TYPE_PIPELINE_RUN_STATE_RUNNING` deprecated

**What changed:** `EVENT_TYPE_PIPELINE_RUN_STATE_RUNNING = 10` is now marked
`[deprecated = true]` in `proto/api/v2/notification.proto`. A replacement value
`EVENT_TYPE_PIPELINE_RUN_STATE_STARTED = 11` has been added.

**Action required:** Update any PipelineRun `spec.notifications[].eventTypes`
configurations that reference `EVENT_TYPE_PIPELINE_RUN_STATE_RUNNING` to use
`EVENT_TYPE_PIPELINE_RUN_STATE_STARTED` instead.

The worker continues to accept both values for one release to give operators
time to migrate. The deprecated value will be removed in a future release.

---

### 2. Workflow rename: `PRNotificationWorkflow` → `io.michelangelo.notification.PipelineRunFanout`

**What changed:** The notification workflow was renamed from `PRNotificationWorkflow`
to `io.michelangelo.notification.PipelineRunFanout` (reverse-DNS style, to avoid
collisions in shared Cadence/Temporal namespaces). The constant is defined in
`go/base/notification/types/types.go` as `PipelineRunNotificationWorkflowName`.

**Action required:** If your fork registers or dispatches the notification workflow
by its string name rather than using the constant, update the string to
`io.michelangelo.notification.PipelineRunFanout`.

The worker also registers `PRNotificationWorkflow` as an alias for one release so
that in-flight executions dispatched by a pre-upgrade controller manager can drain
without timing out. The alias (`DeprecatedPRNotificationWorkflowName`) will be
removed in the following release.

---

### 3. Module move: `notificationActivities.Module` and `notificationWorkflows.Module`

**What changed:** `Module` variables in both
`go/worker/activities/notification` and `go/worker/workflows/notification`
have been moved out of those packages. Registration now happens explicitly in
`go/cmd/worker/main.go` via `fx.Invoke(RegisterNotificationActivities)` and
the `notificationWorkflows.Module` fx option.

**Action required:** If your fork imported `notificationActivities.Module` or
`notificationWorkflows.Module` from the old locations and wired them into your
own fx application, update the import paths and wiring to match the pattern in
`go/cmd/worker/main.go`. Specifically:

- Remove any direct `fx.Options(notificationActivities.Module)` calls that
  relied on the old shared-package module.
- Add `notificationWorkflows.Module` to your fx options if not already present.
- Add `fx.Invoke(RegisterNotificationActivities)` (or your own equivalent) to
  register the activity functions on each worker.

See `go/cmd/worker/main.go` for the reference implementation.
