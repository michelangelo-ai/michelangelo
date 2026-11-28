load("/michelangelo/uniflow/declarative/engine.star", "run")
load("/michelangelo/uniflow/plugins/ray/task.star", "task")

_REGISTRY = {"core/ray": {"factory": task, "job_specs_key": "ray", "capabilities": ["retry"]}}

def run_ray(spec, params):
    return run(spec, params, _REGISTRY)
