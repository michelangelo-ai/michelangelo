# Notification Delivery Setup

Michelangelo does not bundle an email or Slack delivery service — it provides a notification workflow that calls pluggable activity functions when a PipelineRun changes state. By default those functions are no-ops that log a warning. This guide shows platform operators how to wire in real delivery by replacing the default Sink implementations.

It covers the notification flow, Helm configuration, how to implement email and Slack delivery, supported event types, and verification.

---

## How Notifications Work

When a PipelineRun transitions to a matching state, the controller manager starts an `io.michelangelo.notification.PipelineRunFanout` workflow on the `notification_worker` Temporal or Cadence task queue. The worker picks it up and fans out to each configured Sink (one per channel type).

```text
┌─────────────────────────────────────────────────┐
│ controller manager                              │
│ ├─ Detects PipelineRun state transition         │
│ └─ Starts PipelineRunFanout workflow            │
│    └─ task queue: notification_worker            │
└───────────────────────┬─────────────────────────┘
                        │ Temporal / Cadence
                        ▼
┌─────────────────────────────────────────────────┐
│ worker                                          │
│ ├─ EmailSink → SendMessageToEmailActivity       │
│ ├─ SlackSink → SendMessageToSlackActivity       │
│ └─ (custom sinks: PagerDuty, Teams, etc.)       │
└─────────────────────────────────────────────────┘
```

**Operators** replace the default Sink implementations (or add new ones) and configure Helm values.
**Users** annotate their PipelineRun specs with the channels and event types they want notified — see [Pipeline Notifications](../user-guides/ml-pipelines/notifications.md) for the user-facing guide.

---

## Prerequisites

- A running Temporal or Cadence cluster reachable from the worker pod.
- The worker Helm release is deployed (see [Platform Setup](setup/platform-setup.md)).
- Ability to build and push a custom worker image, or to fork the `go/cmd/worker` package.

---

## Step 1: Verify Helm Configuration

The `notification_worker` task queue is included in the default Helm values. Verify it is present in your `values.yaml`:

```yaml
workflow:
  # ... host, provider, domain ...
  taskLists:
    - default
    - trigger_run
    - notification_worker
```

Configure the notification settings — the Studio base URL enables deep links in messages, and the sender email sets the From address for outgoing email:

```yaml
notification:
  taskList: notification_worker
  studioBaseURL: "https://ml.mycompany.com/studio/"   # leave empty to omit links
  senderEmail: "notifications@mycompany.com"           # used as the email From address
```

Apply and restart:

```bash
helm upgrade michelangelo ./helm/michelangelo -f values.yaml
kubectl rollout restart deployment/michelangelo-worker -n <release-namespace>
```

:::tip
Set `notification.taskList` to `""` to disable notifications entirely — the controller manager will skip dispatch.
:::

---

## Step 2: Implement Delivery

The default `EmailSink` and `SlackSink` call activity functions that are no-ops — they log a warning (`no-op: no transport configured`) and return nil. The recommended approach uses `fx.Decorate` to replace the default Sink list without modifying the shared package:

```go
import (
    notification "github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"
    "go.uber.org/fx"
)

func options() fx.Option {
    return fx.Options(
        // ... existing modules ...

        // Replace default sinks with real delivery implementations.
        fx.Decorate(func() []notification.Sink {
            return []notification.Sink{
                &myEmailSink{},   // implements notification.Sink
                &mySlackSink{},   // implements notification.Sink
            }
        }),
    )
}
```

Each Sink must implement the `notification.Sink` interface:

```go
type Sink interface {
    Notify(ctx workflow.Context, logger *zap.Logger, notif *v2pb.Notification, msg Message) error
}
```

The `Message` struct passed to each Sink contains:

| Field             | Type                | Description                                      |
|-------------------|---------------------|--------------------------------------------------|
| `Subject`         | `string`            | Short summary line (e.g. email subject)          |
| `Body`            | `string`            | Plain-text body, suitable for any channel        |
| `FormattedBodies` | `map[string]string` | Format-specific overrides (e.g. `text/html`, `text/slack`) |
| `SendAs`          | `string`            | Sender identity (e.g. email From address)        |

Each Sink checks `FormattedBodies` for its preferred format and falls back to `Body`. See the built-in `EmailSink` and `SlackSink` in `go/worker/workflows/notification/sinks.go` for reference implementations.

Alternatively, you can directly replace the activity function bodies in `go/worker/activities/notification/activities.go` and rebuild the worker image. This is simpler but less portable across upgrades.

#### Email activity signature

```go
func SendMessageToEmailActivity(ctx context.Context, req *SendMessageToEmailActivityRequest) error
```

`SendMessageToEmailActivityRequest` fields:

| Field     | Type       | Description                          |
|-----------|------------|--------------------------------------|
| `To`      | `[]string` | Recipient email addresses            |
| `Cc`      | `[]string` | CC addresses (optional)              |
| `Bcc`     | `[]string` | BCC addresses (optional)             |
| `Subject` | `string`   | Generated subject line               |
| `ReplyTo` | `string`   | Reply-to address (optional)          |
| `HTML`    | `string`   | HTML body (optional)                 |
| `Text`    | `string`   | Plain-text body                      |
| `SendAs`  | `string`   | Sender address shown to recipient    |

#### Slack activity signature

```go
func SendMessageToSlackActivity(ctx context.Context, req *SendMessageToSlackActivityRequest) error
```

`SendMessageToSlackActivityRequest` fields:

| Field     | Type     | Description                |
|-----------|----------|----------------------------|
| `Channel` | `string` | Slack channel ID or name   |
| `Text`    | `string` | Formatted message text     |

---

## Step 3: Configure Notifications on a PipelineRun

Users add a `notifications` block to their PipelineRun spec. No operator action is needed for this step — it is shown here so you can verify end-to-end behavior. For the full user-facing guide including CLI shorthand and all event types, see [Pipeline Notifications](../user-guides/ml-pipelines/notifications.md).

```yaml
apiVersion: michelangelo.api/v2
kind: PipelineRun
metadata:
  name: my-training-run
  namespace: my-project
spec:
  pipeline:
    name: my-training-pipeline
    namespace: my-project
  notifications:
    - notificationType: NOTIFICATION_TYPE_EMAIL
      resourceType: RESOURCE_TYPE_PIPELINE_RUN
      emails:
        - alice@example.com
        - oncall@example.com
      eventTypes:
        - EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED
        - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
    - notificationType: NOTIFICATION_TYPE_SLACK
      resourceType: RESOURCE_TYPE_PIPELINE_RUN
      slackDestinations:
        - "#ml-alerts"
      eventTypes:
        - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
```

For the full list of supported event types and resource types, see [Event Types](../user-guides/ml-pipelines/notifications.md#event-types) in the user guide.

---

## Message Body

The workflow builds subject lines and message bodies automatically from the PipelineRun name, namespace, state, and Studio URL (configured via `notification.studioBaseURL` in Helm values).

Example email subject:
```
Pipeline Run (my-training-run) state: FAILED
```

Example email body:
```
Pipeline Run Status Update:
- Name: my-training-run
- Project: my-project
- State: FAILED
- Pipeline Type: TRAIN
- Studio URL: https://ml.mycompany.com/studio/my-project/train/runs/my-training-run
```

Example Slack message (mrkdwn):
```
Pipeline Run (my-training-run) state: FAILED:
- Name: my-training-run
- Project: my-project
- State: FAILED
- Pipeline Type: TRAIN
- <https://ml.mycompany.com/studio/my-project/train/runs/my-training-run|Studio URL>
```

---

## Upgrade Notes

If upgrading from a previous release, the workflow name changed from `PRNotificationWorkflow` to `io.michelangelo.notification.PipelineRunFanout`. The worker registers the old name as a deprecated alias so that in-flight workflows dispatched by a pre-upgrade controller manager can drain (up to 60h `ExecutionStartToCloseTimeout`). The alias will be removed in a future release — no operator action is needed for the transition.

---

## Verification

### Worker startup logs

After deploying the worker, check the pod logs for the notification task queue:

```bash
kubectl logs -n <release-namespace> deployment/michelangelo-worker | grep notification_worker
```

You should see:

```
INFO  Started Worker  {"TaskQueue": "notification_worker", "WorkerID": "..."}
```

If this line is absent, verify that `notification_worker` is in `workflow.taskLists` in your Helm values and that the worker pod was restarted after the upgrade.

If you haven't replaced the default Sinks yet, you'll also see warnings when notifications fire:

```
WARN  SendMessageToEmailActivity called (no-op: no transport configured)
WARN  SendMessageToSlackActivity called (no-op: no transport configured)
```

These confirm the workflow is dispatching correctly — replace the Sinks with real delivery to stop seeing them.

### Temporal / Cadence workflow history

To confirm that the workflow fired and the activities ran, use the state-scoped workflow ID format `<namespace>.<run-name>.notification.<state>`:

```bash
# Temporal
temporal workflow show \
  --workflow-id "my-project.my-training-run.notification.failed" \
  --namespace default

# Cadence
cadence --domain default workflow show \
  --workflow_id "my-project.my-training-run.notification.failed"
```

A successful run shows `ActivityTaskCompleted` events for `SendMessageToEmailActivity` and/or `SendMessageToSlackActivity`.

### Activity no-op check

If the workflow history shows `ActivityTaskCompleted` but no emails or Slack messages arrived, the Sink implementations are still no-ops. Confirm you have replaced them with real delivery logic (see [Step 2](#step-2-implement-delivery)) and rebuilt the worker image.

---

## Next Steps

- [Pipeline Notifications (user guide)](../user-guides/ml-pipelines/notifications.md) — how users configure notification rules on their specs
- [Platform Setup](setup/platform-setup.md) — configure workflow engine endpoints and task queue settings for the worker
- [Helm Chart](helm-chart.md) — full `values.yaml` reference including `workflow.taskLists` and `notification.*`
- [Jobs Overview](jobs/index.md) — understand how PipelineRuns are scheduled and what triggers state transitions
