# Uniflow caching and pipeline run resume

## What you'll learn

* How to configure task-level caching to skip re-execution
* How cache keys are determined
* How to resume a pipeline run from a specific step

## Task caching

For each task in a Uniflow Remote Run, we cache and index the task results after execution. Next time you execute the task, you have the option to skip execution by reusing the cached results.

We support caching the results produced by **Ray or Spark tasks**. The cached result will be available for **28 days** (platform-managed, not user-configurable).

The following is an example of how to configure a Ray task to index and reuse results. The same method can also be applied to a Spark task.

```python
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

# Argument
#   cache_enabled: True => Reuse the cache if there is a cache hit
#                  False => Force rerun the step.
#   cache_version: A user defined string including numbers, letters and '-'.
@uniflow.task(
    config=RayTask(
        ....
    ),
    cache_enabled=True,
    cache_version="test-cache-version",
)
def feature_join(...):
    ...
    return results
```

In this configuration, the result of the task `feature_join` will be indexed with the following cache keys:

* **Task function path** -- Users cannot specify this cache key. It is the relative function module path, e.g., `michelangelo.maf.feature_prep.feature_prep`.
* **Hash value of task input metadata** -- Users cannot specify this cache key. It is calculated by the serialized metadata of the task inputs. The task input metadata includes storage location, task data type, etc.
* **User-defined cache_version** -- Users can specify this cache key with a string consisting of numbers, letters, `-` and `_`.

If `cache_enabled=True`, when executing `feature_join`, we will try to query the cached results with the above cache keys and skip the task if any cached results are hit.

If `cache_enabled=False`, we will force rerun the `feature_join` task and the produced result will be indexed with the cache keys. Note that in this case, any existing cached result with the same cache keys will be overwritten by the new result.

## Pipeline run resume

Uniflow pipeline runs support resume from a specific step. This relies on the Uniflow cache logic.

Resume from a specific step using the `ma` CLI:

```bash
ma pipeline run -n <namespace> --revision <pipeline-revision-name> --resume_from <pipeline-run-name>:<step-name>
```

**Important:** To skip a step during resume, Uniflow requires that the input of the step has not changed.
