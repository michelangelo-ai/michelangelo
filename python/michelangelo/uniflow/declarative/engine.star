"""Declarative pipeline interpreter.

run(spec, params, registry) walks the task graph of a DeclarativeWorkflow and
dispatches each task to the factory that the registry holds for its type.

- spec: the DeclarativeWorkflow as a dict with proto field names. None means
  unset.
- params: parameter name -> value.
- registry: task type -> {"factory", "job_specs_key", "capabilities"}.

The result is {"states": {task: state}, "outputs": {task: value}}. If a task
failed, run calls fail() after the walk, so the workflow ends FAILED.
"""

load("../commons.star", "TASK_STATE_FAILED", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "report_progress")
load("@plugin", "concurrent", "time")

# This dialect has no while loop and no recursion, so each walk is a bounded
# for loop.
_MAX_STEPS = 100000

# Config keys that the engine consumes. The rest go to the factory.
_ENGINE_CONFIG_KEYS = ["entry_point", "args", "kwargs"]

_WAIT_INTERVAL_SECONDS = 5
_MAX_WAIT_POLLS = 720

# Gate outcome -> the dependency states that satisfy it. A SKIPPED dependency
# satisfies no gate, so a skip propagates.
_GATES = {
    "OUTCOME_UNSPECIFIED": [TASK_STATE_SUCCEEDED],
    "OUTCOME_SUCCEEDED": [TASK_STATE_SUCCEEDED],
    "OUTCOME_FAILED": [TASK_STATE_FAILED],
    "OUTCOME_COMPLETED": [TASK_STATE_SUCCEEDED, TASK_STATE_FAILED],
}

def run(spec, params, registry):
    """Run every task of spec and return the states and outputs."""
    tasks = spec.get("tasks") or {}
    workflow_config = spec.get("workflow_config") or {}
    _validate(tasks, registry)

    states = {}
    outputs = {}
    for level in levels(spec):
        runnable = []
        for name in level:
            if _gates_met(tasks[name], states):
                runnable.append(name)
            else:
                _report(name, tasks[name], TASK_STATE_SKIPPED, "Skipped: dependency gate not satisfied")
                states[name] = TASK_STATE_SKIPPED
                outputs[name] = None
        if runnable:
            _run_level(runnable, tasks, registry, workflow_config, states, outputs)

    failed = [name for name in sorted(states.keys()) if states[name] == TASK_STATE_FAILED]
    if failed:
        fail("declarative: failed task(s): " + ", ".join(failed))
    return {
        "states": {name: states[name] for name in sorted(states.keys())},
        "outputs": {name: outputs[name] for name in sorted(outputs.keys())},
    }

def levels(spec):
    """Return the topological levels of spec, each level in sorted name order."""
    tasks = spec.get("tasks") or {}
    pending = sorted(tasks.keys())
    placed = {}
    result = []
    for _ in range(len(pending) + 1):
        if not pending:
            break
        level = [name for name in pending if all([dep in placed for dep in _dep_names(tasks[name])])]
        if not level:
            fail("declarative: cycle among tasks: " + ", ".join(pending))
        for name in level:
            placed[name] = True
        result.append(level)
        pending = [name for name in pending if name not in placed]
    return result

def _validate(tasks, registry):
    for name in sorted(tasks.keys()):
        task = tasks[name]
        if task["task_function"] not in registry:
            fail("declarative: task {} has type {}, which is not in the registry".format(repr(name), repr(task["task_function"])))
        for dep in _deps(task):
            if dep["task"] not in tasks:
                fail("declarative: task {} depends on unknown task {}".format(repr(name), repr(dep["task"])))
            if _when(dep) not in _GATES:
                fail("declarative: task {} has unknown gate {} on {}".format(repr(name), repr(_when(dep)), repr(dep["task"])))

def _deps(task):
    return task.get("depends_on") or []

def _dep_names(task):
    return [dep["task"] for dep in _deps(task)]

def _when(dep):
    return dep.get("when") or "OUTCOME_UNSPECIFIED"

def _gates_met(task, states):
    for dep in _deps(task):
        if states[dep["task"]] not in _GATES[_when(dep)]:
            return False
    return True

def _report(name, task, state, message):
    ts = time.utc_format_seconds(TIME_FOMART, time.time())
    report_progress(
        task_path = (task.get("config") or {}).get("entry_point") or "",
        task_name = name,
        task_message = message,
        task_state = state,
        start_time = ts,
        end_time = ts,
    )

def _run_level(runnable, tasks, registry, workflow_config, states, outputs):
    outcomes = {}
    callables = []
    for name in runnable:
        task = tasks[name]
        entry = registry[task["task_function"]]
        config = task.get("config") or {}
        # The factory runs in the workflow thread, so a factory that fails
        # stops the workflow with its own error text.
        task_callable = entry["factory"](config.get("entry_point"), **_factory_kwargs(name, task, entry, config, task.get("job_specs") or {}, workflow_config))
        callables.append(concurrent.new_callable(_dispatch, outcomes, name, task_callable, config.get("args") or [], config.get("kwargs") or {}))

    cap = workflow_config.get("max_concurrency") or 0
    if cap < 1:
        cap = len(callables)
    batch = concurrent.batch_run(callables, cap)

    # batch.get() raises when a member failed; is_ready() does not. The
    # outcomes come from the shared dict instead.
    for _ in range(_MAX_WAIT_POLLS):
        if batch.is_ready():
            break
        time.sleep(seconds = _WAIT_INTERVAL_SECONDS)
    if not batch.is_ready():
        fail("declarative: wait budget exhausted")

    for name in runnable:
        states[name] = outcomes[name]["state"]
        outputs[name] = outcomes[name]["output"]

def _factory_kwargs(name, task, entry, config, job_specs, workflow_config):
    kwargs = dict(job_specs.get(entry["job_specs_key"]) or {})
    for key, value in config.items():
        if key not in _ENGINE_CONFIG_KEYS:
            kwargs[key] = value
    kwargs["alias"] = name
    retry_attempts = _retry_attempts(task, entry, workflow_config)
    if retry_attempts != None:
        kwargs["retry_attempts"] = retry_attempts
    return kwargs

def _retry_attempts(task, entry, workflow_config):
    """Return the retries after the first attempt, or None for no policy."""
    if "retry" not in entry["capabilities"]:
        return None

    # A task policy replaces the default policy as a whole.
    policy = task.get("retry")
    if policy == None:
        policy = workflow_config.get("default_retry")
    if policy == None:
        return None
    max_attempts = policy.get("max_attempts") or 0
    if max_attempts < 1:
        return None
    return max_attempts - 1

def _dispatch(outcomes, name, task_callable, args, kwargs):
    # Starlark has no try/except, and a task callable signals failure with
    # fail(). The pessimistic FAILED stays when it fails, so the rest of the
    # graph still runs.
    outcomes[name] = {"state": TASK_STATE_FAILED, "output": None}
    result = task_callable(*args, **kwargs)
    outcomes[name] = {"state": TASK_STATE_SUCCEEDED, "output": result}
    return result
