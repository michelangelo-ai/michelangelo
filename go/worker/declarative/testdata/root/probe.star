def probe(spec, params):
    return {
        "keys": list(spec.keys()),
        "spec": {k: type(v) for k, v in spec.items()},
        "params": type(params),
    }
