# Buildkite Uniflow OSS smoke evidence

Collected on **November 19, 2026** for [michelangelo-ai/michelangelo#2444](https://github.com/michelangelo-ai/michelangelo/pull/2444).
This is an evidence-only branch, not part of the source PR's file diff.

## Source and environment

- Live-tested source and review-fix head: `c392fb49141fb2ad1f73e44f1b433f9ddad6ea8b`.
- Go: the stock OSS `go/cmd/worker`, with its complete Fx/plugin/activity registrations,
  running against a separate local Cadence instance. Native executables from the official
  MySQL 8.0, Cadence v1.2.14, and Cadence Web v3.35.6 images were run locally because
  this test host's Docker endpoint was unavailable. This was not the internal MA worker
  or a full Kubernetes sandbox.
- Python: built a local wheel, installed it in an isolated Python 3.12.16 environment,
  cleared `PYTHONPATH`, verified installed plugin bytes against the source commit,
  and used the ordinary `create_context().run()` local execution path. The bare wheel's
  read-only quickstart was also tested without Ray installed.
- Live API authentication came from an operator-authenticated loopback proxy.
  Credentials were not workflow arguments or results. Operator and private fixture
  configuration are deliberately not included in this bundle.

## Verified live results

| Implementation | Trigger / get / wait | Invoke | Cancel |
| --- | --- | --- | --- |
| Stock OSS Go worker / Starlark | [49729918: passed](https://buildkite.com/uber/go-code-udj/builds/49729918) | [49730026: passed](https://buildkite.com/uber/go-code-udj/builds/49730026) | [49730197: canceled](https://buildkite.com/uber/go-code-udj/builds/49730197) |
| Installed-wheel Python local run | [49728325: passed](https://buildkite.com/uber/go-code-udj/builds/49728325) | [49728546: passed](https://buildkite.com/uber/go-code-udj/builds/49728546) | [49728696: canceled](https://buildkite.com/uber/go-code-udj/builds/49728696) |

Independent read-only checks verified the actual build IDs, numbers, terminal states,
fixture revision/configuration, all seven public field types, clean script-job states,
and the expected `Uniflow Buildkite plugin smoke: passed` marker in each successful
build's actual job logs. The build snapshot's `commit` is the separate job-fixture
repository revision, not the OSS implementation commit.

The Go workflow completed with **47 events** and exactly seven scheduled
activities: three `TriggerBuildkiteBuild`, one `GetBuildkiteBuild`, two
`SensorBuildkiteBuild`, and one `CancelBuildkiteBuild`. Receipts include their actual
OSS-qualified activity types. Submission and cancellation retry policies were absent.

### Actual Cadence UI

![Actual completed OSS Cadence workflow](cadence-oss-summary.png)

The screenshot is cropped before serialized workflow inputs. The onboarding modal
was dismissed using its normal UI button; the displayed status/result were not altered.

- Domain: `buildkite-oss`; task list: `buildkite-oss-smoke`
- Workflow: `buildkite-oss-go-20261119-46aa2e12`
- Run: `6b1760e1-48ce-4112-aa48-8c210500cdb2`
- [Local Cadence run](http://127.0.0.1:18088/domains/buildkite-oss/workflows/buildkite-oss-go-20261119-46aa2e12/6b1760e1-48ce-4112-aa48-8c210500cdb2/summary)
  (requires access/port-forwarding to the test host while this local stack remains running).

### Installed-wheel local-run output

![Rendering of captured installed-wheel local-run output](python-wheel-local-smoke.png)

This is an explicitly labeled browser rendering of captured stdout and independent
verification output, **not a terminal screenshot**. [Verbatim text](python-local-run.txt)
and [machine-readable verified receipts](smoke-receipts.json) are included.

## Additional live cancellation regression

The test held the acknowledgement of a real Buildkite POST, canceled its Cadence parent,
and independently verified automatic cleanup of
[49728423: canceled](https://buildkite.com/uber/go-code-udj/builds/49728423).
History verified the ordering `parent cancel request < submission completion < cleanup`,
exactly one cancellation activity, no subsequent sensor call, and retention of the
original cancellation cause. The existing Starlark SDK's cancellation/failure-envelope
behavior is retained; this change does not claim to change that platform UI status.

## Earlier fixture attempt

In the first Go run, build 49728533 reported an overall `passed` state while its fixture
job hit the fixture's five-minute limit. The actual log printed the expected success
marker and the command exited zero, but the job did not have a clean terminal state.
That attempt was **not used as the clean smoke proof** above: the entire Go suite was
repeated, and the replacement builds have clean script jobs as well as success markers.
The compact anomaly is retained in the receipts; raw agent logs are not published.

## Local validation and boundaries

- Three scoped Bazel suites passed, including a final uncached test execution.
- Go race tests and vet passed; activity/plugin statement coverage: **98.4% / 97.0%**.
- **131 tests** passed on Python **3.9.25 and 3.12.16** with **100% line and branch coverage**.
- Repository-pinned Ruff **0.9.10** lint/format and native Gazelle passed.
- The exact documented Secret-backed worker patch was validated against a Helm-rendered
  Deployment using `kubectl --local`; the existing object-storage `envFrom` and all other
  container fields were preserved. No live Kubernetes rollout was performed.
- `aifx verify` found no repository configuration, so it ran no checks; it is not counted
  as an extra verification pass.
- Live remote execution used Cadence. Temporal and direct bearer-token transport were
  tested hermetically, not with a live Temporal server or newly provisioned Buildkite token.
- No SDK release, wheel publication, AV dependency bump, raw workflow history/input,
  gateway configuration, credentials, or raw job environments/logs are included.
