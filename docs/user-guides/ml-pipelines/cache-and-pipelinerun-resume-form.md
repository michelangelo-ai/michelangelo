# Uniflow Caching: Applied to both Remote Run and MA studio PipelineRun

For each task in a Uniflow Remote Run,  we have cached and indexed the task results after the task execution. Next time when you execute the task, you have the option to skip the task execution by reusing the cached results. 

So far, we support to cache and index the result produced by **a Ray or Spark task**. The cached result will be available for **30 days.**

The following is an example on how to config a ray task to index and reuse the result.  The same method can also be applied to a spark task.

**Uniflow Task**

```
# Argument
#   cache_enabled: True => Reuse the cache if there is a cache hit
#                  False => Force rerun the step.
#   cache_version: A user defined string including numbers, letters and '-'.
@uniflow.task(
    config=Ray(
        ....
    ),
    cache_enabled = True,
    cache_version = "test-cache-version",
)
def feature_join(...):
    ...
    return results
```

In this configuration, the result of the task feature\_join will be indexed with the following cache keys.  

* Task function path. Users can not specify this cache key. It is the relative function module path in uberone, e.g., uberai.michelangelo.maf.feature\_prep.feature\_prep  
* Hash value of task input metadata. Users can not specify this cache key. It is calculated by the serialized metadata of the task inputs. The task input metadata includes storage location saving the task input, task data type etc.  
* a user-defined cache\_version. Users can specify this cache key with a str consisting of numbers, letters, "-" and "\_".

If cache\_enabled \= true, when executing the feature\_join, we will try to query the cached results with the above cache keys and skip the task if any cached results are hit.

If cache\_enable \= false, we will force rerun the feature\_join task and the produced result will be indexed with the cache keys. Note that in this case, any existing cached result with the same cache keys will be overwritten by the new result.

## MA Studio PipelineRun Resume From

Now Uniflow PipelineRun can also support resume from step. Essentially, it relies on the uniflow cache logic. 

We can resume from a specific step using mactl with the follow cmd. 

```
mactl pipeline run -n <namespace> --revision <pipeline-revision-name> --resume_from <pipeline-run-name>:<step-name>
```

**Important Notice:** To skip a step during resume from, Uniflow requires that the input of the step is not changed. 