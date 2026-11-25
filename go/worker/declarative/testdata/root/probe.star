load("/michelangelo/uniflow/declarative/engine.star", "run")

def probe(spec, params):
    res = run(spec, params, {})
    echo_spec, echo_params = res["echo"]
    return {
        "keys": list(echo_spec.keys()),
        "spec": {k: type(v) for k, v in echo_spec.items()},
        "params": type(echo_params),
    }
