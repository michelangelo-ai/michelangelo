# Retry and cache test pipeline

A four-task Ray workflow for checking Uniflow's manual-retry cache behavior. See [Uniflow caching and pipeline run resume](../../../../docs/user-guides/ml-pipelines/cache-and-pipelinerun-resume-form.md) for the feature itself.

`task_a` and `task_b` run concurrently, then `task_c` and `task_d` run concurrently. `task_c` and `task_d` take `task_a`'s result as an argument, so retrying `task_a` (which returns a fresh, unique value each time it genuinely runs) invalidates their cache. `task_b` has no data dependency on `task_a`, so it keeps its cache hit.

## Prerequisites

- **Extras**: `poetry install --extras "example"` (provides Ray).
- **GPU**: not needed. Each task is a 1-CPU Ray task that only sleeps.
- **Sandbox**: not needed for a local run. The retry and resume behavior is a remote-run feature, so exercising it requires a cluster such as the sandbox.

## Run locally

From the `python/` directory:

```bash
PYTHONPATH=. poetry run python examples/pipelines/retry_cache_test/retry_cache_test.py
```

The tasks sleep for 10 s (`task_a`), 60 s (`task_b`), 10 s (`task_c`) and 30 s (`task_d`). The first pair runs concurrently and then the second pair, so expect at least about 90 seconds. Nothing is printed by the workflow itself.

To force a task to fail so you can retry it, pass `fail_b=True`, `fail_c=True` or `fail_d=True` to the `ctx.run(...)` call at the bottom of `retry_cache_test.py`.

## Run on a cluster

`pipeline.yaml` registers this workflow as the `retry-cache-test-pipeline` pipeline in namespace `ma-dev-test`, with `filePath: examples.pipelines.retry_cache_test.retry_cache_test` and the image `ghcr.io/michelangelo-ai/examples:main`. Change the namespace (and the image, if you built your own) to match your environment, then:

```bash
ma pipeline apply -f examples/pipelines/retry_cache_test/pipeline.yaml
ma pipeline run -n <namespace> --name retry-cache-test-pipeline
```

To resume or retry a step, see `--resume_from` in the guide linked above.
