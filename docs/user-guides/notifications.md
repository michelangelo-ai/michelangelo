---
sidebar_position: 9
sidebar_label: "Pipeline Notifications"
---

# Pipeline Notifications

Michelangelo can send email or Slack notifications when a pipeline run, trigger run, or pipeline reaches a terminal state. Notifications are configured directly in the resource spec YAML — no separate setup is required per pipeline.

:::note
Notification delivery requires platform-level integration with a Communication API Gateway (CAG). If notifications are not being delivered in your deployment, contact your Michelangelo operator to confirm the integration is configured. See the [operator integration note](#operator-integration) below.
:::

## Configuration

Add a `notifications` field to any `PipelineRun`, `TriggerRun`, or `Pipeline` spec. Each entry in the list is one notification rule — you can have multiple rules with different event types and destinations.

```yaml
notifications:
  - notificationType: NOTIFICATION_TYPE_EMAIL
    resourceType: RESOURCE_TYPE_PIPELINE_RUN
    eventTypes:
      - EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED
      - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
    emails:
      - "you@example.com"
      - "team@example.com"

  - notificationType: NOTIFICATION_TYPE_SLACK
    resourceType: RESOURCE_TYPE_PIPELINE_RUN
    eventTypes:
      - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
      - EVENT_TYPE_PIPELINE_RUN_STATE_KILLED
    slackDestinations:
      - "#ml-alerts"
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `notificationType` | Yes | `NOTIFICATION_TYPE_EMAIL` or `NOTIFICATION_TYPE_SLACK` |
| `resourceType` | Yes | The resource being watched. See [Resource Types](#resource-types). |
| `eventTypes` | Yes | One or more events that trigger this notification. See [Event Types](#event-types). |
| `emails` | For email | List of recipient email addresses. |
| `slackDestinations` | For Slack | List of Slack channel names (e.g. `#alerts`). These are channel names, not webhook URLs — routing is handled by the platform. |

## Event Types

Choose events based on the `resourceType` of the notification:

### Pipeline Run events (`RESOURCE_TYPE_PIPELINE_RUN`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED` | Run completed successfully |
| `EVENT_TYPE_PIPELINE_RUN_STATE_FAILED` | Run failed |
| `EVENT_TYPE_PIPELINE_RUN_STATE_KILLED` | Run was manually stopped |
| `EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED` | Run was skipped (e.g. by a trigger concurrency policy) |

### Trigger Run events (`RESOURCE_TYPE_TRIGGER_RUN`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_TRIGGER_RUN_STATE_SUCCEEDED` | Trigger run completed |
| `EVENT_TYPE_TRIGGER_RUN_STATE_FAILED` | Trigger run failed |
| `EVENT_TYPE_TRIGGER_RUN_STATE_KILLED` | Trigger run was stopped |

### Pipeline events (`RESOURCE_TYPE_PIPELINE`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_PIPELINE_STATE_READY` | Pipeline build succeeded and is ready to run |
| `EVENT_TYPE_PIPELINE_STATE_ERROR` | Pipeline build failed |

## Resource Types

| Resource type | Use with |
|---------------|----------|
| `RESOURCE_TYPE_PIPELINE_RUN` | `PipelineRun` spec or any spec that creates runs |
| `RESOURCE_TYPE_TRIGGER_RUN` | `TriggerRun` spec |
| `RESOURCE_TYPE_PIPELINE` | `Pipeline` spec (build-time events) |

## Message Format

Both channels receive the same information, formatted for the medium.

**Slack message:**
```
Pipeline Run (my-training-run) has completed with state FAILED:
- Name: my-training-run
- Project: my-project
- State: FAILED
- Pipeline Type: TRAIN
- <https://michelangelo-studio.example.com/ma/my-project/train/runs/my-training-run|Michelangelo Studio URL>
```

**Email:**
- Subject: `Pipeline Run (my-training-run) has completed with state FAILED`
- Sender: `michelangelo@uber.com`
- Body contains the same fields as the Slack message, as plain text with a link to MA Studio

For ASL (Amazon States Language) pipelines, a Cadence Log URL is appended to the message.

## Full Example: TriggerRun with Both Channels

This example sends email on success or failure and Slack only on failure or kill:

```yaml
apiVersion: michelangelo.api/v2
kind: TriggerRun
metadata:
  name: training-pipeline-backfill-trigger
  namespace: my-project
spec:
  pipeline:
    name: training-pipeline
    namespace: my-project
  trigger:
    cronSchedule:
      cron: "0 8 * * *"
    maxConcurrency: 3
  startTimestamp: 2025-10-01T00:00:00Z
  endTimestamp: 2025-10-08T00:00:00Z
  notifications:
    - notificationType: NOTIFICATION_TYPE_EMAIL
      resourceType: RESOURCE_TYPE_PIPELINE_RUN
      eventTypes:
        - EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED
        - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
      emails:
        - "you@example.com"

    - notificationType: NOTIFICATION_TYPE_SLACK
      resourceType: RESOURCE_TYPE_PIPELINE_RUN
      eventTypes:
        - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
        - EVENT_TYPE_PIPELINE_RUN_STATE_KILLED
      slackDestinations:
        - "#ml-alerts"
```

Apply it with the CLI:

```bash
ma apply -f trigger.yaml
```

## Operator Integration

Notification delivery is handled by two Temporal/Cadence activities in the Michelangelo worker:

- `SendMessageToSlackActivity` — sends the formatted message to the specified Slack channel
- `SendMessageToEmailActivity` — sends the formatted email to all recipients

These activities are stubs that must be connected to a Communication API Gateway (CAG) or equivalent messaging service by your platform operator. The activities receive a `channel` + `text` (Slack) or a full email request struct (`to`, `cc`, `subject`, `html`, `text`, `send_as`) and are responsible for delivery.

If you are an operator implementing this integration, see:
- `go/worker/activities/notification/activities.go` — activity stubs with the request types and integration comments
- `go/worker/workflows/notification/workflows.go` — the workflow that calls the activities
- `go/base/notification/types/types.go` — message generation helpers
