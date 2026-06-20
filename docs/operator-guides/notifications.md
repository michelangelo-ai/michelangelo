# Notification Delivery Setup

Michelangelo does not bundle an email or Slack delivery service — it provides a notification workflow that calls pluggable activity functions when a PipelineRun changes state. By default those functions are no-ops. This guide shows platform operators how to wire in real delivery by implementing the activity functions in their own worker binary.

It covers the notification flow, Helm configuration, how to implement email and Slack delivery, supported event types, and verification.

---

## How Notifications Work

When a PipelineRun transitions to a terminal state, the controller manager starts a `PRNotificationWorkflow` on the `notification_worker` Temporal or Cadence task queue. The worker picks it up and executes one activity per configured channel.

```text
┌─────────────────────────────────────────────────┐
│ controller manager                              │
│ ├─ Detects PipelineRun state transition         │
│ └─ Starts PRNotificationWorkflow                │
│    └─ task queue: notification_worker           │
└───────────────────────┬─────────────────────────┘
                        │ Temporal / Cadence
                        ▼
┌─────────────────────────────────────────────────┐
│ worker                                          │
│ ├─ SendMessageToEmailActivity (no-op by default)│
│ └─ SendMessageToSlackActivity (no-op by default)│
└─────────────────────────────────────────────────┘
```

**Operators** implement the activity functions and register them in the worker binary.
**Users** annotate their PipelineRun specs with the channels and event types they want notified.

---

## Prerequisites

- A running Temporal or Cadence cluster reachable from the worker pod.
- The worker Helm release is deployed (see [Platform Setup](setup/platform-setup.md)).
- Ability to build and push a custom worker image, or to fork the `go/cmd/worker` package.

---

## Step 1: Enable the `notification_worker` Task Queue

The worker listens on multiple task queues. Add `notification_worker` to the `taskLists` array in your Helm values:

```yaml
workflow:
  # ... host, provider, domain ...
  taskLists:
    - default
    - trigger_run
    - pipeline_run
    - notification_worker   # add this
```

Apply with:

```bash
helm upgrade michelangelo ./helm/michelangelo -f values.yaml
```

Restart the worker pod to pick up the new task queue:

```bash
kubectl rollout restart deployment/michelangelo-worker -n <release-namespace>
```

---

## Step 2: Implement Delivery

Both activity functions in `go/worker/activities/notification/activities.go` are no-ops with a `TODO` comment. You have two paths:

In your fork of `go/cmd/worker/main.go`, use `fx.Decorate` to replace the activity implementations without modifying the shared package:

```go
import (
    notificationactivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/notification"
    "go.uber.org/fx"
)

func options() fx.Option {
    return fx.Options(
        // ... existing modules ...

        fx.Decorate(func() notificationactivities.EmailSender {
            return &myEmailClient{}
        }),
        fx.Decorate(func() notificationactivities.SlackSender {
            return &mySlackClient{}
        }),
    )
}
```

Implement `myEmailClient` and `mySlackClient` to call your organization's email and Slack APIs.

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

Users add a `notifications` block to their PipelineRun spec. No operator action is needed for this step — it is shown here so you can verify end-to-end behavior.

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

### Supported event types

| Event type                             | Triggers when                    |
|----------------------------------------|----------------------------------|
| `EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED` | Run completed successfully    |
| `EVENT_TYPE_PIPELINE_RUN_STATE_FAILED`    | Run failed                    |
| `EVENT_TYPE_PIPELINE_RUN_STATE_KILLED`    | Run was killed                |
| `EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED`   | Run was skipped               |

---

## Message Body

The workflow builds subject lines and message bodies automatically from the PipelineRun name, namespace, state, and a Studio URL. The Studio URL is constructed from the constant `_defaultMaURL` in `go/base/notification/types/types.go`. To point it at your own Michelangelo Studio deployment, update that constant and rebuild the worker image.

Example email subject:
```
Pipeline Run (my-training-run) has completed with state FAILED
```

Example Slack message:
```
Pipeline Run (my-training-run) has completed with state FAILED:
- Name: my-training-run
- Project: my-project
- State: FAILED
- Pipeline Type: TRAIN
- <https://michelangelo.your-org.com/ma/my-project/train/my-training-run|Michelangelo Studio URL>
```

---

## Verification

### Worker startup logs

After deploying the worker with `notification_worker` in `taskLists`, check the worker pod logs:

```bash
kubectl logs -n <release-namespace> deployment/michelangelo-worker | grep notification_worker
```

You should see:

```
INFO  Started Worker  {"TaskQueue": "notification_worker", "WorkerID": "..."}
```

If this line is absent, verify that `notification_worker` is in `workflow.taskLists` in your Helm values and that the worker pod was restarted after the upgrade.

### Temporal / Cadence workflow history

To confirm that the workflow fired and the activities ran:

```bash
# Temporal
temporal workflow show --workflow-id "<namespace>.<run-name>.notification" --namespace default

# Cadence
cadence --domain default workflow show --workflow_id "<namespace>.<run-name>.notification"
```

A successful run shows `ActivityTaskCompleted` events for `SendMessageToEmailActivity` and/or `SendMessageToSlackActivity`.

### Activity no-op check

If the workflow history shows `ActivityTaskCompleted` but no emails or Slack messages arrived, the activity implementations are still no-ops. Confirm you have replaced them with real delivery logic and rebuilt the worker image.

---

## Next Steps

- [Platform Setup](setup/platform-setup.md) — configure workflow engine endpoints and task queue settings for the worker
- [Helm Chart](helm-chart.md) — full `values.yaml` reference including `workflow.taskLists`
- [Jobs Overview](jobs/index.md) — understand how PipelineRuns are scheduled and what triggers state transitions
