# Buildkite service plugin

The Buildkite plugin lets a Uniflow workflow submit a build, inspect it, wait for
completion, or request cancellation. The Buildkite agents execute the job; the
Michelangelo worker only makes short API calls.

The same five functions work locally in Python and remotely through the Go
worker's `buildkite` Starlark module:

```python
from michelangelo.uniflow.core import workflow
from michelangelo.uniflow.core.lib.buildkite import (
    cancel_build,
    get_build,
    invoke_build,
    trigger_build,
    wait_for_build,
)


@workflow()
def release(pipeline: str, branch: str, commit: str) -> "dict":
    submitted = trigger_build(
        pipeline,
        branch=branch,
        commit=commit,
        message="Model release",
        env={"RELEASE_CHANNEL": "staging"},
        metadata={"source": "uniflow"},
    )
    snapshot = get_build(pipeline, submitted["number"])
    assert snapshot["id"] == submitted["id"]
    completed = wait_for_build(
        pipeline,
        submitted["number"],
        timeout_seconds=1800,
        poll_interval=15,
    )
    return completed
```

`invoke_build(...)` is the shorthand for `trigger_build(...)` followed by
`wait_for_build(...)`:

```python
completed = invoke_build(
    "model-release",
    branch="main",
    commit="HEAD",
    timeout_seconds=1800,
    poll_interval=15,
)
```

To cancel a previously submitted build, use
`cancel_build(pipeline, submitted["number"])`. Cancellation is asynchronous:
the returned snapshot may still say `canceling`. Inspect it with `get_build`
to confirm the final state; `wait_for_build` only succeeds for `passed`.

## Operator configuration

Set these environment variables on the **local Python process** for a local
run, or on the **Go worker** for a remotely compiled workflow:

| Variable | Purpose |
| --- | --- |
| `BUILDKITE_ORGANIZATION` | Required organization slug. It is fixed by the operator, not selected by workflow code. |
| `BUILDKITE_API_TOKEN` | Buildkite API bearer token. Required with the default endpoint. Inject it from your deployment's secret manager. |
| `BUILDKITE_API_URL` | Optional REST API base URL. Defaults to `https://api.buildkite.com/v2`. An explicitly configured, authenticated proxy may supply its own authentication instead of using a bearer token. |

Use a token with only the necessary `read_builds` and `write_builds` scopes
and access to the intended pipelines. Installations must authorize which
workflow authors may use that worker identity. A configured organization is
not a replacement for pipeline authorization.

HTTPS is required except for an explicitly configured loopback HTTP proxy.
URLs containing credentials, queries, or fragments are rejected. Redirects
are not followed, so credentials and mutations cannot be forwarded to a
different endpoint.
Loopback HTTP requests also bypass environment-configured forward proxies so
credentials cannot be sent off-host over plaintext HTTP.

No configuration is needed for workflows that do not use Buildkite: the
stock worker registers the plugin but validates its configuration on first
use. Installing this plugin does not create pipelines or grant access.
Inject optional worker environment variables using your normal deployment
configuration; never bake a token into a worker image.

### Environment variables versus function parameters

Organization, endpoint, and API credentials belong to the **operator's process
environment**. They are never workflow parameters, serialized activity inputs,
or returned values.

Pipeline, branch, commit, message, wait options, and per-build inputs belong to
the **function call**. The `env` argument is a dictionary of environment
overrides for the **Buildkite job**, not the Michelangelo process. Buildkite
populates its own standard job environment separately.

**Do not put secrets in `env`, `metadata`, or `message`.** Workflow arguments
are persisted in execution history, and Buildkite or an API proxy may log
submission payloads. Configure job secrets on the owning pipeline instead.

## API contract

| Operation | Meaning |
| --- | --- |
| `trigger_build(pipeline, branch="main", commit="HEAD", message="", env=None, metadata=None)` | Submit once and return the current snapshot, without waiting. |
| `get_build(pipeline, build_number)` | Return one snapshot, even if the build failed. |
| `wait_for_build(pipeline, build_number, timeout_seconds=3600, poll_interval=30)` | Poll until `passed`; fail on a terminal failure or an unknown state. |
| `invoke_build(pipeline, branch="main", commit="HEAD", message="", env=None, metadata=None, timeout_seconds=3600, poll_interval=30)` | Validate options, submit once, then wait. The wait budget starts after submission. |
| `cancel_build(pipeline, build_number)` | Request cancellation once and return its snapshot. |

Every snapshot has exactly these JSON-compatible fields:

```python
{
    "id": "globally-unique-build-uuid",
    "number": 42,
    "pipeline": "model-release",
    "state": "passed",
    "web_url": "https://buildkite.com/example/model-release/builds/42",
    "branch": "main",
    "commit": "git-commit",
}
```

`number` is a positive, pipeline-scoped integer (at most `2**63 - 1`); `id` is
the global UUID. Use **number**, not **id**, in subsequent operations. Missing
optional response strings normalize to `""`. Raw environments, metadata,
tokens, job lists, and response bodies are not returned. Responses larger than
8 MiB and malformed snapshots are rejected.
Status reads request `exclude_jobs=true` to avoid fetching large job lists that
are not part of the result contract. Numbered reads and cancellations reject a
response for a different build number.

Pipeline slugs match `[a-z0-9][a-z0-9_-]*`. `env` and `metadata` accept string
keys/values, an empty dictionary, or `None`. Wait budgets are integers from
1–86400 seconds; poll intervals are integers from 1–3600 seconds.

## Failures, timeouts, and cancellation

- `creating`, `scheduled`, `waiting`, `running`, `failing`, `blocked`, and `canceling`
  are pending. Only `passed` completes a wait successfully.
- Buildkite's independent `blocked` flag keeps a wait pending even if the
  reported state is `passed` or `failed`. The seven-field public snapshot
  remains unchanged; use `wait_for_build` for the completion guarantee.
- `failed`, `canceled`, `skipped`, and `not_run` fail the wait immediately.
  Unknown states, malformed responses, and permanent API errors also fail
  immediately.
- Only status polling retries transient transport errors, HTTP 408/429, and
  server errors. Remote polling is durable through Cadence/Temporal activity
  retries; no workflow thread sleeps on ordinary wall-clock time.
  If a sensor retry policy stops before the next backoff would fit, a durable
  timer keeps the remaining budget open rather than cancelling early. Activity
  queue time is bounded by the overall wait budget.
- Submission and cancellation are **not automatically retried**, including
  ambiguous transport failures. Buildkite does not deduplicate submissions.
  Inspect Buildkite before resubmitting after a lost acknowledgement.
  Workflow retries, resets, or explicit repeated calls can still create
  additional builds; this API does not promise exactly-once effects.
- A wait timeout or workflow cancellation requests best-effort cancellation
  using a separate, bounded activity. A cleanup failure never replaces the
  original error. Locally, `TimeoutError` and `KeyboardInterrupt` have the
  equivalent cleanup behavior; other forms of process termination cannot
  guarantee cleanup.
- API requests have a 30-second timeout, reduced to the remaining wait
  budget for local reads. Python socket timeouts are not strict total
  wall-clock limits. Both implementations check the wait deadline after
  receiving a result, so a late response cannot report success.

For direct Starlark use, load the module with
`load("@plugin", "buildkite")` and call the same operation names.
