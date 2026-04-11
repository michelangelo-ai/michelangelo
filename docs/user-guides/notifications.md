# Pipeline Notifications

Stay informed about your ML pipeline outcomes without constantly checking the dashboard. Michelangelo can send you **email or Slack notifications** whenever a pipeline run succeeds, fails, gets stopped, or hits any other terminal state.

Notifications are configured directly in your resource spec YAML — just add a `notifications` block and you're set. No separate setup, no external webhook configuration, and no per-pipeline toggles to manage.

:::note
Notifications are configured through YAML specs only. The Michelangelo Studio UI does not currently support notification configuration.
:::

:::caution Operator Implementation Required
Notification delivery is not enabled by default in open-source deployments. The notification workflow fires correctly when pipeline states change, but the email and Slack delivery activities are stubs that must be implemented by your platform operator. See the reference files below for implementation details.
:::

## When to Use Notifications

Notifications are especially useful when you:

- **Run long training jobs** and want to know the moment they finish (or fail)
- **Schedule recurring pipelines** with triggers and need alerts when something goes wrong
- **Work as a team** and want pipeline outcomes posted to a shared Slack channel
- **Run nightly backfills** and want a morning email summary of what succeeded or failed

## Quick Start

Here's the fastest way to get started: add a `notifications` block to your pipeline run spec to get an email when it succeeds or fails.

```yaml
notifications:
  - notificationType: NOTIFICATION_TYPE_EMAIL
    resourceType: RESOURCE_TYPE_PIPELINE_RUN
    eventTypes:
      - EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED
      - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
    emails:
      - "you@example.com"
```

That's it! When you apply this spec with `ma apply -f your-spec.yaml`, Michelangelo will send you an email each time the run reaches one of those states.

Want Slack notifications instead? Swap the type and destination:

```yaml
notifications:
  - notificationType: NOTIFICATION_TYPE_SLACK
    resourceType: RESOURCE_TYPE_PIPELINE_RUN
    eventTypes:
      - EVENT_TYPE_PIPELINE_RUN_STATE_FAILED
    slackDestinations:
      - "#ml-alerts"
```

You can also combine both in a single spec — see the [full example](#full-example) below.

## Configuration Reference

Add a `notifications` field to any `PipelineRun`, `TriggerRun`, or `Pipeline` spec. Each entry in the list is one notification rule, and you can have as many rules as you need with different event types and destinations.

:::tip
You can configure notifications in your YAML specs at any time. Messages will be delivered once your operator has enabled notification delivery.
:::

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `notificationType` | Yes | `NOTIFICATION_TYPE_EMAIL` or `NOTIFICATION_TYPE_SLACK` |
| `resourceType` | Yes | The type of resource you're watching. See [Resource Types](#resource-types). |
| `eventTypes` | Yes | One or more events that trigger this notification. See [Event Types](#event-types). |
| `emails` | For email | List of recipient email addresses. |
| `slackDestinations` | For Slack | List of Slack channel names (e.g., `#alerts`). Use channel names, not webhook URLs — the platform handles routing for you. |

### Resource Types

| Resource type | What it watches |
|---------------|-----------------|
| `RESOURCE_TYPE_PIPELINE_RUN` | Individual pipeline run outcomes (success, failure, etc.) |
| `RESOURCE_TYPE_TRIGGER_RUN` | Trigger run outcomes for scheduled or event-driven runs |
| `RESOURCE_TYPE_PIPELINE` | Pipeline build events (build succeeded or failed) |

### Event Types

Choose events based on the `resourceType` you're watching.

#### Pipeline Run Events (`RESOURCE_TYPE_PIPELINE_RUN`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED` | Run completed successfully |
| `EVENT_TYPE_PIPELINE_RUN_STATE_FAILED` | Run failed |
| `EVENT_TYPE_PIPELINE_RUN_STATE_KILLED` | Run was manually stopped |
| `EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED` | Run was skipped (e.g., by a trigger concurrency policy) |

#### Trigger Run Events (`RESOURCE_TYPE_TRIGGER_RUN`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_TRIGGER_RUN_STATE_SUCCEEDED` | Trigger run completed |
| `EVENT_TYPE_TRIGGER_RUN_STATE_FAILED` | Trigger run failed |
| `EVENT_TYPE_TRIGGER_RUN_STATE_KILLED` | Trigger run was stopped |

#### Pipeline Events (`RESOURCE_TYPE_PIPELINE`)

| Event type | When it fires |
|------------|---------------|
| `EVENT_TYPE_PIPELINE_STATE_READY` | Pipeline build succeeded and is ready to run |
| `EVENT_TYPE_PIPELINE_STATE_ERROR` | Pipeline build failed |

## Message Format

Both email and Slack notifications include the same core information, formatted appropriately for each medium.

**Slack message example:**

```
Pipeline Run (my-training-run) has completed with state FAILED:
- Name: my-training-run
- Project: my-project
- State: FAILED
- Pipeline Type: TRAIN
- <https://michelangelo-studio.example.com/ma/my-project/train/runs/my-training-run|Michelangelo Studio URL>
```

**Email example:**

- **Subject:** `Pipeline Run (my-training-run) has completed with state FAILED`
- **Body:**
  ```
  Your Michelangelo Studio Pipeline Run Has Status Update:
  - Name: my-training-run
  - Project: my-project
  - State: FAILED
  - Pipeline Type: TRAIN
  - Michelangelo Studio URL: https://michelangelo-studio.example.com/ma/my-project/train/runs/my-training-run
  ```

## Full Example

This `TriggerRun` spec sets up a daily training pipeline backfill with dual-channel notifications — email on success or failure, and Slack only on failure or kill:

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

## Updating or Removing Notifications

To change your notification settings, update the `notifications` block in your spec and re-apply:

```bash
ma apply -f your-spec.yaml
```

To stop receiving notifications, remove the `notifications` block entirely and re-apply the spec.

## Troubleshooting

**I configured notifications but I'm not receiving them.**

- Double-check that your `notificationType`, `resourceType`, and `eventTypes` are valid values from the tables above. Typos in enum values will be silently ignored.
- For email, verify the addresses in `emails` are correct.
- For Slack, confirm the channel name in `slackDestinations` exists and is accessible by the platform.
- Notification delivery depends on operator-level implementation. If everything looks correct in your spec but notifications still aren't arriving, check with your platform administrator.

**I'm only getting some of my notifications.**

- Make sure you've listed all the event types you care about. For example, if you want alerts on both success and failure, you need both `EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED` and `EVENT_TYPE_PIPELINE_RUN_STATE_FAILED` in your `eventTypes` list.
- Each notification rule is independent. If you have separate rules for email and Slack, check that each one has the correct event types.

## Enabling Notification Delivery

Notification delivery is not active in open-source deployments by default. The notification workflow fires correctly when pipeline states change, but the email and Slack delivery steps are stubs that your platform operator must implement.

To enable delivery, an operator needs to provide concrete implementations for two activity stubs:

- **Email delivery** — implement the email send activity to connect to your SMTP provider or email API (e.g., SendGrid, SES).
- **Slack delivery** — implement the Slack send activity to post messages to channels using the Slack API or incoming webhooks.

Once implemented and deployed, notifications configured in your specs will begin delivering automatically — no changes to your YAML are required.

Contact your platform administrator if notifications are configured correctly but messages are not arriving.

## What's Next

- [Pipeline Running Modes](./ml-pipelines/pipeline-running-modes.md) — learn about the different ways to execute your pipelines
- [ML Pipelines Overview](./ml-pipelines/index.md) — understand the broader pipeline framework
