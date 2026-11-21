---
last_verified: 2026-11-19
---

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
def release(pipeline: str, branch: str, commit: str):
    submitted = trigger_build(
        pipeline,
        branch=branch,
        commit=commit,
        message="Model release",
        env={"RELEASE_CHANNEL": "staging"},
        metadata={"source": "uniflow"},
    )
    snapshot = get_build(pipeline, submitted["number"])
    completed = wait_for_build(
        pipeline,
        submitted["number"],
        timeout_seconds=1800,
        poll_interval=15,
    )
    return {"snapshot": snapshot, "completed": completed}
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
| `BUILDKITE_ALLOWED_PIPELINES` | Required comma-separated pipeline slugs, such as `model-release,smoke-test`. All operations fail closed when unset. Whitespace around slugs is ignored; wildcards and malformed/empty entries are rejected. |
| `BUILDKITE_API_TOKEN` | Buildkite API bearer token. Required with the default endpoint. Inject it from your deployment's secret manager. |
| `BUILDKITE_API_URL` | Optional REST API base URL. Defaults to `https://api.buildkite.com/v2`. An explicitly configured, authenticated proxy may supply its own authentication instead of using a bearer token. |

Use a token with only the necessary `read_builds` and `write_builds` scopes
and access to the intended pipelines. Installations must authorize which
workflow authors may use that worker identity. A configured organization is
not a replacement for pipeline authorization. The allowlist is an additional
worker-side boundary, not per-author or per-build authorization: permitted
workflow authors can choose the branch, commit, and non-reserved job environment,
and can cancel any build in an allowed pipeline. Only allow pipelines whose
code and secrets those authors are trusted to use. Use dedicated workers,
restricted Buildkite identities, and pipeline-side controls for separate trust
domains; do not share a privileged worker with untrusted workflow authors.

HTTPS is required except for an explicitly configured loopback HTTP proxy.
URLs containing credentials, queries, or fragments are rejected. Redirects
are not followed, so credentials and mutations cannot be forwarded to a
different endpoint.
Loopback HTTP requests also bypass environment-configured forward proxies so
credentials cannot be sent off-host over plaintext HTTP.

No configuration is needed for workflows that do not use Buildkite: the
stock worker registers the plugin but validates its configuration on first
use. Installing this plugin does not create pipelines or grant access.
Never bake a token into a worker image or include it in workflow arguments.

### Helm worker setup

The shipped chart does not expose `worker.extraEnv` or `worker.extraEnvFrom`.
Until it does, apply the following **explicit post-install Deployment patch**
from your deployment automation. It leaves the existing object-storage
`envFrom` untouched and references a separate Secret for the token. Reapply
and verify the patch after Helm upgrades. Use a worker image containing this
plugin before enabling workflows that call it.

Create the Secret in the worker's namespace using your secret manager, or
from a protected token file (not a token literal in shell history):

```bash
NAMESPACE=michelangelo
RELEASE=michelangelo
kubectl --namespace "$NAMESPACE" create secret generic buildkite-credentials \
  --from-file=BUILDKITE_API_TOKEN=/secure/path/buildkite-api-token
```

Save this non-secret patch as `buildkite-worker.patch.yaml`, replacing the
organization and pipeline slugs:

```yaml
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: BUILDKITE_ORGANIZATION
              value: my-org
            - name: BUILDKITE_ALLOWED_PIPELINES
              value: model-release,smoke-test
            - name: BUILDKITE_API_TOKEN
              valueFrom:
                secretKeyRef:
                  name: buildkite-credentials
                  key: BUILDKITE_API_TOKEN
```

Select the chart's worker by its release/component labels and apply the
strategic merge patch. The container name `app` matches the chart:

```bash
WORKER_DEPLOYMENT="$(kubectl --namespace "$NAMESPACE" get deployment \
  --selector="app.kubernetes.io/instance=$RELEASE,app.kubernetes.io/component=worker" \
  --output=jsonpath='{.items[0].metadata.name}')"
test -n "$WORKER_DEPLOYMENT"  # Stop if no worker Deployment matched.
kubectl --namespace "$NAMESPACE" patch deployment "$WORKER_DEPLOYMENT" \
  --type=strategic --patch-file=buildkite-worker.patch.yaml
kubectl --namespace "$NAMESPACE" rollout status deployment/"$WORKER_DEPLOYMENT"
```

Do not add Buildkite credentials to the object-storage Secret. For a custom
authenticated proxy, set `BUILDKITE_API_URL` on the worker and omit the token
only if the proxy provides authentication. Configuration is read when the
worker starts, so restart workers after rotating environment-backed secrets.

### Environment variables versus function parameters

Organization, allowed pipelines, endpoint, and API credentials belong to the
**operator's process environment**. They are never workflow parameters,
serialized activity inputs, or returned values.

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
| `wait_for_build(pipeline, build_number, timeout_seconds=3600, poll_interval=30, cancel_on_timeout=True)` | Poll until `passed`; fail on a terminal failure or an unknown state. **By default, timeout/interruption requests cancellation**, even for an externally submitted build. Set `cancel_on_timeout=False` to observe without canceling. |
| `invoke_build(pipeline, branch="main", commit="HEAD", message="", env=None, metadata=None, timeout_seconds=3600, poll_interval=30)` | Validate options, submit once, then wait with cleanup enabled for its submitted build. The wait budget starts after submission. |
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
keys/values, an empty dictionary, or `None`. Job environment keys must match
`[A-Za-z_][A-Za-z0-9_]*` and must not start with the reserved `BUILDKITE_`
prefix. Metadata keys are not subject to the environment-key restriction.
Wait budgets are integers from 1–86400 seconds; poll intervals are integers
from 1–3600 seconds. An interval longer than the wait budget is allowed: the
first poll still runs, and the next delay is bounded by the remaining budget.

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
- By default, a wait timeout or workflow cancellation requests best-effort cancellation
  using a separate, bounded activity. A cleanup failure never replaces the
  original error. Locally, `TimeoutError` and `KeyboardInterrupt` have the
  equivalent cleanup behavior; other forms of process termination cannot
  guarantee cleanup. `wait_for_build(..., cancel_on_timeout=False)` disables
  this cleanup on both timeout and interruption. Use it for builds you do
  not own or manual approvals you do not want to cancel.
- For remote `invoke_build`, cancellation during submission waits for the
  bounded, non-retried submission to return a build number before attempting
  cleanup. A lost or malformed acknowledgement can still leave a build whose
  number is unknown; inspect the pipeline before resubmitting.
- Local transient poll failures are logged without raw upstream details.
  If polling remains unavailable until timeout, the timeout includes the last
  sanitized poll error rather than presenting it only as a slow build.
- API requests have a 30-second timeout, reduced to the remaining wait
  budget for local reads. Python socket timeouts are not strict total
  wall-clock limits, so the local path also checks the deadline after each
  read. Remote activity completion is bounded by the workflow server's
  schedule-to-close timeout; a successful sensor result is retained even if
  the workflow worker resumes after the deadline.

## Try it locally

Install an SDK wheel containing this plugin, or install from the repository
root with `python -m pip install ./python`. Create a Buildkite
[API access token](https://buildkite.com/docs/apis/managing-api-tokens)
with access only to the intended pipelines: `read_builds` for observation,
and `write_builds` as well for submission/cancellation.

```bash
export BUILDKITE_ORGANIZATION=my-org
export BUILDKITE_ALLOWED_PIPELINES=model-release
export BUILDKITE_API_TOKEN="$(cat /secure/path/buildkite-api-token)"
python -c "from michelangelo.uniflow.core.lib.buildkite import get_build; print(get_build('model-release', 42))"
```

Replace `42` with an existing build number. This quickstart only reads a
snapshot; it does not create or cancel a build. To observe it until completion
without taking ownership, use
`wait_for_build("model-release", 42, cancel_on_timeout=False)`.

## Troubleshooting

| Symptom | Likely cause and action |
| --- | --- |
| `BUILDKITE_ORGANIZATION is required` | Set the organization on the local process or worker; malformed slugs are reported separately. |
| `BUILDKITE_ALLOWED_PIPELINES` error | Ask the operator to allow the exact pipeline slug. Unset lists deny all operations; no wildcard is accepted. |
| HTTP 401 | Check the worker/process token and its expiration. |
| HTTP 403 | Check `read_builds`/`write_builds` scopes and the identity's access to the pipeline. |
| HTTP 404 | Check the organization, pipeline slug, build number, and token visibility. |
| HTTP 422 | Check the submission's branch, commit, and other build parameters. |
| `did not pass within N seconds` | Check for a slow build, a manual block step, or the reported last poll error. Increase the budget if appropriate; use `cancel_on_timeout=False` when only observing. |
| `submission outcome may be unknown` | Inspect recent builds before retrying. A lost acknowledgement does not mean submission failed. |
| `unknown state` | Check Buildkite's state documentation and file an issue with the state name; the plugin fails closed on unrecognized states. |

For direct Starlark use, load the module with
`load("@plugin", "buildkite")` and call the same operation names.
