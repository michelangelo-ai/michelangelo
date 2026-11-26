load("/michelangelo/uniflow/declarative/engine.star", "run")
load("/michelangelo/uniflow/plugins/spark/task.star", "spark_task")

_REGISTRY = {"core/spark": {"factory": spark_task, "job_specs_key": "spark", "capabilities": ["retry"]}}

def run_spark(spec, params):
    return run(spec, params, _REGISTRY)
