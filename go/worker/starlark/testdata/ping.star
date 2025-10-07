load("@plugin", "os", "time", "uuid", "workflow")

def main(verbose = True, **keywords):
    result = {
        "message": "PI is {}".format(3.14159265359),
        "execution_id": workflow.execution_id,
        "verbose": verbose,
        "keywords": keywords,
    }
    if not verbose:
        return result

    rand_uuid = uuid.uuid4()
    extra = {
        "execution_run_id": workflow.execution_run_id,
        "epoch_nanosec": time.time_ns(),
        "epoch_sec": time.time(),
        "environ": os.environ,
        "uuid": {
            "urn": rand_uuid.urn,
            "hex": rand_uuid.hex,
        },
    }
    result.update(extra)
    return result
