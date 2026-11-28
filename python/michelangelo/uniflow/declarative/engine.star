"""Declarative pipeline interpreter.

run(spec, params, registry) walks the task graph of a DeclarativeWorkflow and
dispatches each task to the factory that the registry holds for its type.

- spec: the DeclarativeWorkflow as a dict with proto field names. None means
  unset.
- params: parameter name -> value.
- registry: task type -> {"factory", "job_specs_key", "capabilities"}.

The result is {"states": {task: state}, "outputs": {task: value}}. If a task
failed, the workflow ends FAILED after the walk. When a task callable failed,
the workflow error holds the errors of the failed callables in the first such
level, joined with "; ".
"""

load("../commons.star", "TASK_STATE_FAILED", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "report_progress")
load("@plugin", "atexit", "concurrent", "json", "time")

# This dialect has no while loop and no recursion, so each walk is a bounded
# for loop.
_MAX_STEPS = 100000

_ENGINE_CONFIG_KEYS = ["entry_point", "args", "kwargs"]

_FIRST_POLL_SECONDS = 5
_MAX_POLL_SECONDS = 300

_LEVEL_WAIT_BUDGET_SECONDS = 7 * 24 * 60 * 60

_GATES = {
    "OUTCOME_UNSPECIFIED": [TASK_STATE_SUCCEEDED],
    "OUTCOME_SUCCEEDED": [TASK_STATE_SUCCEEDED],
    "OUTCOME_FAILED": [TASK_STATE_FAILED],
    "OUTCOME_COMPLETED": [TASK_STATE_SUCCEEDED, TASK_STATE_FAILED],
}

def next_poll_delay(poll):
    """Return the seconds to sleep after poll number poll, counted from 0."""
    return min(_FIRST_POLL_SECONDS << min(poll, 7), _MAX_POLL_SECONDS)

def _poll_count(budget):
    total = 0
    for poll in range(budget):
        if total >= budget:
            return poll
        total += next_poll_delay(poll)
    return budget

_MAX_POLLS = _poll_count(_LEVEL_WAIT_BUDGET_SECONDS)

def run(spec, params, registry):
    """Run every task of spec and return the states and outputs."""
    tasks = spec.get("tasks") or {}
    workflow_config = spec.get("workflow_config") or {}
    _validate(tasks, registry)

    states = {}
    outputs = {}
    refs = {"params": _typed_params(spec, params), "states": states, "outputs": outputs, "ancestors": _ancestors(tasks)}
    failed_batches = []
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
            batch = _run_level(runnable, tasks, registry, workflow_config, refs)
            if batch != None:
                failed_batches.append(batch)

    failed = [name for name in sorted(states.keys()) if states[name] == TASK_STATE_FAILED]
    if failed:
        summary = "declarative: failed task(s): " + ", ".join(failed)
        if failed_batches:
            # get() raises the joined errors of the failed callables in the
            # batch, which replace the summary as the workflow error.
            print(summary)
            failed_batches[0].get()
        fail(summary)
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

def _run_level(runnable, tasks, registry, workflow_config, refs):
    """Run one level and return its batch if a task callable failed, else None."""
    states = refs["states"]
    outputs = refs["outputs"]
    outcomes = {}
    running = {}
    watch = {}
    callables = []
    for name in runnable:
        task = tasks[name]
        entry = registry[task["task_function"]]
        config, err = _resolve(task.get("config") or {}, "config", name, refs)
        job_specs = {}
        if err == None:
            job_specs, err = _resolve(task.get("job_specs") or {}, "job_specs", name, refs)
        if err == None:
            err = _check_types(config)
        if err != None:
            _report(name, task, TASK_STATE_FAILED, "Failed: " + err)
            outcomes[name] = {"state": TASK_STATE_FAILED, "output": None}
            continue
        task_callable = entry["factory"](config.get("entry_point"), **_factory_kwargs(name, task, entry, config, job_specs, workflow_config))
        callables.append(concurrent.new_callable(_dispatch, outcomes, running, watch, name, task_callable, config.get("args") or [], config.get("kwargs") or {}))

    batch = None
    if callables:
        batch = _wait(callables, running, watch, workflow_config)
    for name in runnable:
        states[name] = outcomes[name]["state"]
        outputs[name] = outcomes[name]["output"]
    if [name for name in running.keys() if states[name] == TASK_STATE_FAILED]:
        return batch
    return None

def _check_types(config):
    """Return an error if a config key that the engine consumes has a wrong type."""
    for key, want in [("entry_point", "string"), ("args", "list"), ("kwargs", "dict")]:
        value = config.get(key)
        if value == None and key != "entry_point":
            continue
        if type(value) != want:
            return "config.{}: must be a {}, got {}".format(key, want, type(value))
    return None

def _wait(callables, running, watch, workflow_config):
    """Run the callables of one level and return their batch.

    When the wait budget ends, this function removes the drain hook before
    fail. Thus the exit does not drain the tasks that still run.
    """
    cap = workflow_config.get("max_concurrency") or 0
    if cap < 1:
        cap = len(callables)
    batch = concurrent.batch_run(callables, cap)

    # A cancel fails a sleep, and the workflow closes when this thread ends.
    # The poll loop sleeps in its own thread, so that a task that starts
    # after a cancel can see that the loop ended.
    watch["poll"] = concurrent.batch_run([concurrent.new_callable(_poll, batch)], 1)

    # After a cancel, each task still cleans up in its own thread. The exit
    # hook blocks until every task has ended.
    drain = batch.get
    atexit.register(drain)
    watch["poll"].get()
    atexit.unregister(drain)
    if batch.is_ready():
        return batch
    still = [name for name in sorted(running.keys()) if not running[name].is_ready()]
    fail("declarative: wait budget exhausted; still running: " + ", ".join(still))

def _poll(batch):
    # batch.get() raises when a member failed; is_ready() does not. The
    # outcomes come from the shared dict instead.
    for poll in range(_MAX_POLLS):
        if batch.is_ready():
            return
        time.sleep(seconds = next_poll_delay(poll))

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

    policy = task.get("retry")
    if policy == None:
        policy = workflow_config.get("default_retry")
    if policy == None:
        return None
    max_attempts = policy.get("max_attempts") or 0
    if max_attempts < 1:
        return None
    return max_attempts - 1

def _dispatch(outcomes, running, watch, name, task_callable, args, kwargs):
    # Starlark has no try/except, and a task callable signals failure with
    # fail(). The pessimistic FAILED stays when it fails, so the rest of the
    # graph still runs.
    outcomes[name] = {"state": TASK_STATE_FAILED, "output": None}

    # After a cancel, the worker lets a new activity run to its end. A task
    # that the concurrency cap held back would then run in full.
    if watch["poll"].is_ready():
        fail("declarative: task {} did not start: the wait for its level ended".format(repr(name)))

    # A batch of one tells a running callable from a failed one: is_ready()
    # is true only after the callable returned or failed.
    call = concurrent.batch_run([concurrent.new_callable(_call, task_callable, args, kwargs)], 1)
    running[name] = call
    result = call.get()[0]
    outcomes[name] = {"state": TASK_STATE_SUCCEEDED, "output": result}
    return result

def _call(task_callable, args, kwargs):
    # concurrent.new_callable passes no keyword arguments.
    return task_callable(*args, **kwargs)

def _typed_params(spec, params):
    # JSON decoding gives an int for an integral number, but a NUMBER
    # parameter always reaches a task as a float.
    declared = spec.get("parameters") or {}
    result = {}
    for name, value in params.items():
        if value != None and (declared.get(name) or {}).get("type") == "PARAMETER_TYPE_NUMBER":
            value = float(value)
        result[name] = value
    return result

def _ancestors(tasks):
    """Return task name -> the set of its transitive dependencies."""
    result = {}
    for name in tasks.keys():
        seen = {}
        stack = _dep_names(tasks[name])
        for _ in range(_MAX_STEPS):
            if not stack:
                break
            dep = stack.pop()
            if dep not in seen:
                seen[dep] = True
                stack.extend(_dep_names(tasks[dep]))
        result[name] = seen
    return result

def _resolve(value, path, name, refs):
    """Return (value, error) with each reference in a string leaf resolved.

    A resolved value is not scanned again.
    """
    root = {}
    work = [(root, "value", value, path)]
    for i in range(_MAX_STEPS):
        if i >= len(work):
            break
        parent, key, node, at = work[i]
        if type(node) == "string":
            node, err = _resolve_string(node, name, refs)
            if err != None:
                return None, at + ": " + err
        elif type(node) == "dict":
            copy = {}
            for k in node.keys():
                copy[k] = None
                work.append((copy, k, node[k], at + "." + k))
            node = copy
        elif type(node) == "list":
            copy = [None] * len(node)
            for j in range(len(node)):
                work.append((copy, j, node[j], "{}[{}]".format(at, j)))
            node = copy
        parent[key] = node
    if len(work) > _MAX_STEPS:
        fail("declarative: task {} has too many values in {}".format(repr(name), path))
    return root["value"], None

def _resolve_string(s, name, refs):
    parts = []
    rest = s
    for _ in range(len(s)):
        start = rest.find("{{")
        if start < 0:
            break
        end = rest.find("}}", start)
        if end < 0:
            return None, "unterminated {} in {}".format(repr("{{"), repr(s))
        token = rest[start:end + 2]
        value, err = _lookup(token, name, refs)
        if err != None:
            return None, err
        if token == s:
            return value, None
        parts.append(rest[:start])
        parts.append(_text(value))
        rest = rest[end + 2:]
    parts.append(rest)
    return "".join(parts), None

def _lookup(token, name, refs):
    body = token[2:-2].strip()
    if body.startswith("params."):
        param = body[len("params."):]
        if not _is_name(param):
            return None, "reference {} has a malformed name".format(repr(token))
        if param not in refs["params"]:
            return None, "reference {} names an unknown parameter".format(repr(token))
        return refs["params"][param], None
    if body.startswith("tasks."):
        parts = body[len("tasks."):].split(".")
        if len(parts) < 2 or parts[1] != "output":
            return None, "reference {} must name {}".format(repr(token), repr("tasks.{}.output".format(parts[0])))
        dep = parts[0]
        fields = parts[2:]
        if not _is_name(dep) or not all([_is_field(f) for f in fields]):
            return None, "reference {} has a malformed name".format(repr(token))
        if dep not in refs["ancestors"][name]:
            return None, "reference {} names task {}, which is not a dependency".format(repr(token), repr(dep))
        if refs["states"][dep] != TASK_STATE_SUCCEEDED:
            return None, "reference {} names task {}, which did not succeed".format(repr(token), repr(dep))
        value = refs["outputs"][dep]
        for f in fields:
            if type(value) != "dict" or f not in value:
                return None, "reference {} names field {}, which the output does not have".format(repr(token), repr(f))
            value = value[f]
        return value, None
    return None, "reference {} must start with {} or {}".format(repr(token), repr("params."), repr("tasks."))

_LOWER = "abcdefghijklmnopqrstuvwxyz"
_DIGITS = "0123456789"
_FIELD_START = _LOWER + _LOWER.upper() + "_"

def _is_name(s):
    if len(s) < 1 or len(s) > 63 or s[0] not in _LOWER:
        return False
    return all([c in _LOWER + _DIGITS + "_" for c in s.elems()])

def _is_field(s):
    if len(s) < 1 or s[0] not in _FIELD_START:
        return False
    return all([c in _FIELD_START + _DIGITS for c in s.elems()])

def _text(value):
    """Return the text of value embedded in a longer string.

    The text is the JSON encoding of value, except that a string is not
    quoted. An integral float has no fraction, as in Go encoding/json.
    """
    if type(value) == "string":
        return value
    if type(value) == "float":
        return _float_text(value)
    return json.dumps(value)

def _float_text(f):
    # Go writes the shortest digits, in positional form when 1e-6 <= |f| <
    # 1e21 and in exponent form otherwise. Starlark str() gives the same
    # shortest digits, so only the layout changes.
    s = str(f)
    mantissa, _, exponent = s.partition("e")
    if f != 0 and (abs(f) < 1e-6 or abs(f) >= 1e21):
        if not exponent:
            return s
        return mantissa.removesuffix(".0") + "e" + exponent[0] + (exponent[1:].lstrip("0") or "0")
    sign = ""
    if mantissa.startswith("-"):
        sign = "-"
        mantissa = mantissa[1:]
    whole, _, frac = mantissa.partition(".")
    digits = (whole + frac).rstrip("0") or "0"
    point = len(whole)
    if exponent:
        point += int(exponent[0] + (exponent[1:].lstrip("0") or "0"))
    if point <= 0:
        return sign + "0." + "0" * -point + digits
    if point >= len(digits):
        return sign + digits + "0" * (point - len(digits))
    return sign + digits[:point] + "." + digits[point:]
