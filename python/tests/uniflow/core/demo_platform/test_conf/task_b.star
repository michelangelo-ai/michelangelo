load("commons.star", "echo")

def task_b(task_path):
    def f(*args, **kw):
        return _run(task_path, args, kw)

    return f

def _run(task_path, args, kw):
    return echo(task_type = "task_b", task_path = task_path, args = args, kw = kw)
