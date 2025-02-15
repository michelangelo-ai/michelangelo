load("@plugin", "ray")

def test_create_cluster():
    spec = {
        "metadata": {
            "namespace": "test-namespace",
            "name": "test-name",
        },
        "spec": {
            "affinity": {
                "resourceAffinity": {
                    "selector": {
                        "matchLabels": {
                            "resourcepool.michelangelo/zone": "dca60",
                        },
                    },
                },
            },
            "head": {
                "pod": {
                    "resource": {
                        "cpu": 2,
                        "memory": "8G",
                        "gpu": 1,
                    },
                },
            },
            "worker": {
                "pod": {
                    "resource": {
                        "cpu": 2,
                        "memory": "8G",
                        "gpu": 1,
                    },
                },
            },
        },
    }
    ray.create_cluster(spec, timeout_seconds = 60, poll_seconds = 10)

def test_terminate_cluster():
    ray.terminate_cluster("test-namespace", "test-name", type = 1, reason = "default")

def test_run_job():
    # TODO: andrii: implement test_run_job
    return "ok"
