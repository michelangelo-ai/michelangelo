load("/michelangelo/uniflow/commons.star", "TASK_STATE_FAILED", "TASK_STATE_SUCCEEDED", "report_progress")
load("/michelangelo/uniflow/declarative/engine.star", "levels", "run")
load("@plugin", "time")

def _report(alias, state):
    report_progress(task_path = "fake", task_name = alias, task_state = state)

_ABSENT = "absent"

def _ok(entry_point, alias = None, retry_attempts = _ABSENT, **fields):
    def call(*args, **kwargs):
        _report(alias, TASK_STATE_SUCCEEDED)
        return {"ep": entry_point, "alias": alias, "args": list(args), "kwargs": kwargs, "retry": retry_attempts, "fields": fields}

    return call

def _fail(entry_point, alias = None, retry_attempts = None, **fields):
    def call(*args, **kwargs):
        _report(alias, TASK_STATE_FAILED)
        fail("fake failure: " + alias)

    return call

def _fail_silent(entry_point, alias = None, **fields):
    def call(*args, **kwargs):
        fail("silent failure: " + alias)

    return call

def _bad_factory(entry_point, alias = None, **fields):
    fail("bad factory: " + alias)

def _noretry(entry_point, alias = None, **fields):
    def call(*args, **kwargs):
        _report(alias, TASK_STATE_SUCCEEDED)
        return {"ep": entry_point, "alias": alias, "fields": fields}

    return call

# The result round trip can turn 1.0 into 1, so this fake reports the
# Starlark type of each field.
def _types(entry_point, alias = None, **fields):
    def call(*args, **kwargs):
        _report(alias, TASK_STATE_SUCCEEDED)
        return {"types": {k: type(v) for k, v in fields.items()}, "fields": fields}

    return call

def _sleep(entry_point, alias = None, seconds = 10, **fields):
    def call(*args, **kwargs):
        start = time.time()
        time.sleep(seconds = seconds)
        _report(alias, TASK_STATE_SUCCEEDED)
        return {"start": int(start)}

    return call

def _entry(factory, job_specs_key, capabilities):
    return {"factory": factory, "job_specs_key": job_specs_key, "capabilities": capabilities}

REGISTRY = {
    "core/ray": _entry(_ok, "ray", ["retry"]),
    "fake/bad_factory": _entry(_bad_factory, "fake", []),
    "fake/fail": _entry(_fail, "fake", ["retry"]),
    "fake/fail_silent": _entry(_fail_silent, "fake", []),
    "fake/noretry": _entry(_noretry, "fake", []),
    "fake/ok": _entry(_ok, "fake", ["retry"]),
    "fake/sleep": _entry(_sleep, "fake", []),
    "fake/types": _entry(_types, "fake", []),
}

def run_case(spec, params):
    return run(spec, params, REGISTRY)

def run_timed(spec, params):
    start = time.time_ns()
    result = run(spec, params, REGISTRY)
    return {"result": result, "elapsed": (time.time_ns() - start) // 1000000000}

def levels_of(spec):
    return {"levels": levels(spec)}
