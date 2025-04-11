load("@plugin", "ray")

def test_create_cluster():
    spec = {
        "metadata": {
            "name": "uf-ray-test",
            "namespace": "default",
        },
        "spec": {
            "user": {"name": "test-user"},
            "rayVersion": "2.3.1",  # Keeping original version
            "head": {
                "serviceType": "ClusterIP",
                "rayStartParams": {
                    "block": "true",
                    "dashboard-host": "0.0.0.0",
                },
                "pod": {
                    "spec": {
                        "containers": [
                            {
                                "name": "head",
                                "image": "test-image",
                                "envFrom": [
                                    {
                                        "configMapRef": {
                                            "localObjectReference": {
                                                "name": "michelangelo-config",
                                            },
                                        },
                                    },
                                ],
                                "lifecycle": {
                                    "postStart": {
                                        "exec": {
                                            "command": ["/bin/sh", "-c", "echo", "'Initializing Ray Head'"],
                                        },
                                    },
                                },
                            },
                        ],
                    },
                },
            },
            "workers": [
                {
                    "minInstances": 1,
                    "maxInstances": 2,
                    "nodeType": "worker-group-1",
                    "rayStartParams": {
                        "block": "true",
                        "dashboard-host": "0.0.0.0",
                    },
                    "pod": {
                        "spec": {
                            "containers": [
                                {
                                    "name": "worker",
                                    "image": "test-image",
                                    "envFrom": [
                                        {
                                            "configMapRef": {
                                                "localObjectReference": {
                                                    "name": "michelangelo-config",
                                                },
                                            },
                                        },
                                    ],
                                    "lifecycle": {
                                        "postStart": {
                                            "exec": {
                                                "command": ["/bin/sh", "-c", "echo", "'Initializing Ray Worker'"],
                                            },
                                        },
                                    },
                                },
                            ],
                        },
                    },
                },
            ],
            "rayConf": {},
        },
    }
    return ray.create_cluster(spec)

def test_create_job():
    return ray.create_job("python3 -m michelangelo.uniflow.core.run_task --task 'examples.bert_cola.data.load_data' --args '[\"glue\",\"cola\"]' --kwargs '{\"tokenizer_max_length\":128}' --result-url 's3://default/d47efe2f682f4965bcf119f9d9a06eb1.json'", "default", "test-ray-job")
