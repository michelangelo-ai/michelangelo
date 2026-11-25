load("../commons.star", "TASK_STATE_SUCCEEDED")

def run(spec, params, registry):
    return {"states": {}, "outputs": {}, "echo": [spec, params]}
