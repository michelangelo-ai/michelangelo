load("@plugin", "atexit", "json", "os", "ray", "time")
load("../../commons.star", "RESOURCE_AFFINITY_LABEL_CLOUD_ZONE", "RESOURCE_AFFINITY_LABEL_SUPPORT_GPU", "RESOURCE_AFFINITY_LABEL_ZONE", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "create_cached_output", "get_cache_keys", "get_env_label", "get_project_resource_annotation", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

CREATE_CLUSTER_TIMEOUT_SECONDS = 60 * 30  # Timeout duration for cluster creation in seconds.
RAY_ENV = {
    "RAY_DEDUP_LOGS": "0",
    # RAY_NUM_REDIS_GET_RETRIES controls the number of retries for a worker node to connect to the GCS (Global Control Service) at startup.
    # Source: https://github.com/ray-project/ray/blob/releases/2.9.2/python/ray/_private/node.py#L688
    # The default value is 20, giving a worker about 140 seconds to connect to the GCS (7 seconds per retry).
    # We need to provide workers more time to connect because the Job Controller doesn't support gang scheduling of nodes.
    # As a result, a worker node might start significantly earlier than the head node.
    # Calculate RAY_NUM_REDIS_GET_RETRIES to allow workers approximately CREATE_CLUSTER_TIMEOUT_SECONDS to connect to the GCS, assuming 7 seconds per retry.
    "RAY_NUM_REDIS_GET_RETRIES": str((CREATE_CLUSTER_TIMEOUT_SECONDS // 7) + 1),
}
RAY_DEFAULT_HEAD_CPU = os.environ.get("RAY_DEFAULT_HEAD_CPU", "8")
RAY_DEFAULT_HEAD_MEMORY = os.environ.get("RAY_DEFAULT_HEAD_MEMORY", "32Gi")
RAY_DEFAULT_HEAD_DISK = os.environ.get("RAY_DEFAULT_HEAD_DISK", "512Gi")
RAY_DEFAULT_HEAD_GPU = os.environ.get("RAY_DEFAULT_HEAD_GPU", "0")

RAY_DEFAULT_WORKER_CPU = os.environ.get("RAY_DEFAULT_WORKER_CPU", "8")
RAY_DEFAULT_WORKER_MEMORY = os.environ.get("RAY_DEFAULT_WORKER_MEMORY", "32Gi")
RAY_DEFAULT_WORKER_DISK = os.environ.get("RAY_DEFAULT_WORKER_DISK", "512Gi")
RAY_DEFAULT_WORKER_GPU = os.environ.get("RAY_DEFAULT_WORKER_GPU", "0")
RAY_DEFAULT_WORKER_INSTANCES = os.environ.get("RAY_DEFAULT_WORKER_INSTANCES", "1")

RAY_DEFAULT_GPU_SKU = os.environ.get("RAY_DEFAULT_GPU_SKU", "")
RAY_DEFAULT_ZONE = os.environ.get("RAY_DEFAULT_ZONE", "")

def task(
        task_path,
        alias = None,
        cache_version = None,
        cache_enabled = False,
        head_cpu = RAY_DEFAULT_HEAD_CPU,
        head_memory = RAY_DEFAULT_HEAD_MEMORY,
        head_disk = RAY_DEFAULT_HEAD_DISK,
        head_gpu = RAY_DEFAULT_HEAD_GPU,
        head_object_store_memory = None,
        worker_cpu = RAY_DEFAULT_WORKER_CPU,
        worker_memory = RAY_DEFAULT_WORKER_MEMORY,
        worker_disk = RAY_DEFAULT_WORKER_DISK,
        worker_gpu = RAY_DEFAULT_WORKER_GPU,
        worker_object_store_memory = None,
        worker_instances = RAY_DEFAULT_WORKER_INSTANCES,
        gpu_sku = RAY_DEFAULT_GPU_SKU,
        zone = RAY_DEFAULT_ZONE,
        breakpoint = False):
    def callable(*args, **kwargs):
        os.environ["UF_TASK_IMAGE"] = "michelanngelo-python-serivce"
        os.environ["UF_STORAGE_URL"] = "/tmp/uf_storage"
        task_name = get_task_name(task_path, alias)
        cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version)
        namespace = "default"
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
        #        if cache_enabled:  # Check if the result is cached
        #            cached_output = get_cached_output(namespace, cache_keys)
        #            if cached_output != None:
        #                cached_result_json_url = cached_output.get("spec", {}).get("storageUri", "")
        #                if cached_result_json_url != "":
        #                    end_time_seconds = time.time()
        #                    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        #                    report_progress(
        #                        task_path = task_path,
        #                        task_name = task_name,
        #                        task_log = "",
        #                        task_message = "Ray Task Skipped with Cache Hit",
        #                        task_state = TASK_STATE_SKIPPED,
        #                        start_time = start_time_formated_str,
        #                        end_time = end_time_formated_str,
        #                        output = cached_output.get("metadata", {}).get("name", ""),
        #                    )
        #                    return io_read_json(cached_result_json_url)

        # Apply resource overrides
        _head_cpu = os.environ.get("RAY_OVERRIDE_HEAD_CPU." + task_path, head_cpu)
        _head_memory = os.environ.get("RAY_OVERRIDE_HEAD_MEMORY." + task_path, head_memory)
        _head_disk = os.environ.get("RAY_OVERRIDE_HEAD_DISK." + task_path, head_disk)
        _head_gpu = os.environ.get("RAY_OVERRIDE_HEAD_GPU." + task_path, head_gpu)

        _worker_cpu = os.environ.get("RAY_OVERRIDE_WORKER_CPU." + task_path, worker_cpu)
        _worker_memory = os.environ.get("RAY_OVERRIDE_WORKER_MEMORY." + task_path, worker_memory)
        _worker_disk = os.environ.get("RAY_OVERRIDE_WORKER_DISK." + task_path, worker_disk)
        _worker_gpu = os.environ.get("RAY_OVERRIDE_WORKER_GPU." + task_path, worker_gpu)
        _worker_instances = os.environ.get("RAY_OVERRIDE_WORKER_INSTANCES." + task_path, worker_instances)

        _gpu_sku = os.environ.get("RAY_OVERRIDE_GPU_SKU." + task_path, gpu_sku)
        _zone = os.environ.get("RAY_OVERRIDE_ZONE." + task_path, zone)

        # Apply resource types
        _head_cpu = int(_head_cpu)
        _head_gpu = int(_head_gpu)
        _worker_cpu = int(_worker_cpu)
        _worker_gpu = int(_worker_gpu)
        _worker_instances = int(_worker_instances)

        # Create cluster
        cluster_namespace = "default"
        cluster_image = get_task_image(task_name)
        print("ray | create cluster:", "ns:", cluster_namespace, "image:", cluster_image, "task_name:", task_name)
        if _worker_gpu + _head_gpu > 0:
            # By default, we will launch ray gpu jobs to phx8 with A100 for phx4 GPU decommission
            if _zone == "" and _gpu_sku == "":
                _zone = "phx8"
                _gpu_sku = "A100"

        cluster = ray_cluster_spec(
            namespace = cluster_namespace,
            image = cluster_image,
            head_resource = resource_dict(
                cpu = _head_cpu,
                memory = _head_memory,
            ),
            worker_resource = resource_dict(
                cpu = _worker_cpu,
                memory = _worker_memory,
            ),
            worker_instances = _worker_instances,
            zone = _zone,
            debug_enabled = breakpoint,
            head_object_store_memory = head_object_store_memory,
            worker_object_store_memory = worker_object_store_memory,
        )
        cluster_res = ray.create_cluster(cluster)
        print(cluster_res)
        cluster = cluster_res["rayCluster"]
        cluster_url = "http://localhost:8265/#/cluster"
        cluster_name = cluster["metadata"]["name"]
        cluster_namespace = cluster["metadata"]["namespace"]
        head_node = "ray-cluster-head-8kt9g"
        head_ip = "8265"
        dashboard_port = "8088"

        print("ray | cluster created:", "ns=" + cluster_namespace, "n=" + cluster_name, "url=" + cluster_url, "head_ip=" + head_ip)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = cluster_url,
            task_message = "Ray Cluster Created Successfully",
            task_state = TASK_STATE_RUNNING,
            start_time = start_time_formated_str,
            end_time = "",
        )

        def terminate_cluster():
            # ray.terminate_cluster(cluster_namespace, cluster_name)
            print("ray | cluster terminated:", "ns=" + cluster_namespace, "n=" + cluster_name)

        atexit.register(terminate_cluster)

        # Comment out hacky way to access ray dashboard through ssh tunnel port forwarding
        # print("ray | dashboad ssh:", "ssh -fMNL 8265:{}:{} bastion.uber.com; open http://localhost:8265".format(head_ip, dashboard_port))
        print("ray | cluster dashboard:", "http://localhost:8265/#/cluster")
        if breakpoint:
            jupyter_notebook_port = head_node.get("jupyterNotebookPort")
            print("ray | jupyter ssh:", "ssh -fMNL 8888:{}:{} bastion.uber.com; open http://localhost:8888".format(head_ip, jupyter_notebook_port))

        # Run job
        dashboard_url = "http://{}:{}".format(head_ip, dashboard_port)
        result_url = get_result_url()
        entrypoint = ray_job_entrypoint(task_path, result_url, args, kwargs)
        print("ray | run job:", "task_path=" + task_path)
        job = ray.create_job(
            dashboard_url,
            entrypoint,
            ray_job_namespace = cluster_namespace,
            ray_job_name = cluster_name,
        )
        print("ray | +run job: job=" + str(job))

        def report_ray_task_result():
            end_time_seconds = time.time()
            end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
            if job["ray_job"]["status"]["state"] == "RAY_JOB_STATE_SUCCEEDED":
                report_progress(
                    task_path = task_path,
                    task_name = task_name,
                    task_log = cluster_url,
                    task_message = "Ray Job Succeeded",
                    task_state = TASK_STATE_SUCCEEDED,
                    start_time = start_time_formated_str,
                    end_time = end_time_formated_str,
                )
            elif job["ray_job"]["status"]["state"] == "RAY_JOB_STATE_KILLED":
                message = job.get("message", "unknown reason")
                task_message = "Ray Job killed with {}".format(message)
                report_progress(
                    task_path = task_path,
                    task_name = task_name,
                    task_log = cluster_url,
                    task_message = task_message,
                    task_state = TASK_STATE_KILLED,
                    start_time = start_time_formated_str,
                    end_time = end_time_formated_str,
                )
            else:
                error_type = job.get("errorType", "internal")
                message = job.get("message", "unknown error")
                task_message = "Ray Job Failed with {} Error: {}".format(error_type, message)
                report_progress(
                    task_path = task_path,
                    task_name = task_name,
                    task_log = cluster_url,
                    task_message = task_message,
                    task_state = TASK_STATE_FAILED,
                    start_time = start_time_formated_str,
                    end_time = end_time_formated_str,
                )

        atexit.register(report_ray_task_result)

        if breakpoint:
            print("ray | breakpoint:", "ns=" + cluster_namespace, "n=" + cluster_name, "head_ip=" + head_ip)

            # TODO: andrii: use HITL instead of sleep.
            time.sleep(seconds = 60 * 60 * 24)
            err_message = "internal: breakpoint timeout"
            print("ray | error:", err_message)
            fail(err_message)

        # Terminate cluster
        # if job["status"] == "SUCCEEDED":
        # we must unregister the exit hook to avoid calling them for other tasks.
        terminate_cluster()
        report_ray_task_result()
        atexit.unregister(terminate_cluster)
        atexit.unregister(report_ray_task_result)

        # Read result from the storage
        if job["ray_job"]["status"]["state"] != "RAY_JOB_STATE_SUCCEEDED":
            fail("internal:", "message:bad job status:", job["ray_job"]["status"]["state"], job)

        created_cached_output = create_cached_output(
            namespace = cluster_namespace,
            cache_keys = cache_keys,
            zone = "",  #TODO: baoquan: add zone info to cache
            ttl_in_days = 0,
            task_name = task_name,
            result_json_url = result_url,
        )
        cached_output_name = created_cached_output
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_SUCCEEDED,
            task_log = cluster_url,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            task_message = "Ray Task Completed Successfully",
            output = cached_output_name,
        )
        return io_read_json(result_url)

    def with_overrides(alias = alias):
        return task(
            task_path = task_path,
            alias = alias,
            cache_version = cache_version,
            cache_enabled = cache_enabled,
            head_cpu = head_cpu,
            head_memory = head_memory,
            head_disk = head_disk,
            head_gpu = head_gpu,
            head_object_store_memory = head_object_store_memory,
            worker_cpu = worker_cpu,
            worker_memory = worker_memory,
            worker_disk = worker_disk,
            worker_gpu = worker_gpu,
            worker_object_store_memory = worker_object_store_memory,
            worker_instances = worker_instances,
            gpu_sku = gpu_sku,
            zone = zone,
            breakpoint = breakpoint,
        )

    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable

def ray_job_entrypoint(task_path, result_url, args = None, kwargs = None):
    args = json.dumps(args) if args else "[]"
    kwargs = json.dumps(kwargs) if kwargs else "{}"

    # TODO: andrii: set --overrides
    return "python3 -m michelangelo.uniflow.core.run_task --task '" + task_path + "' --args '" + args + "' --kwargs '" + kwargs + "' --result-url '" + result_url + "'"

# Constructs a Unified API resource for a Ray Cluster. Refer to the RayJob CRD: https://sg.uberinternal.com/code.uber.internal/uber-code/go-code/-/blob/idl/code.uber.internal/uberai/michelangelo/api/v2beta1/ray_job.proto
#
# Parameters:
#     namespace: (string) The Unified API namespace, also known as the Michelangelo Project ID. Ex: ma-dev-test
#     image: (string) The Docker image that includes Ray, the application code, and its dependencies. Ex: 127.0.0.1:5055/uber-usi/uber-one-michelangelo-sandbox:bkt1-produ-1719018451-45448
#     head_resource: (dict) Resource configuration for the head node. Refer to the resource_dict function in commons.star.
#     worker_resource: (dict) Resource configuration for the worker nodes. Refer to the resource_dict function in commons.star.
#     worker_instances: (int) The number of worker instances. Must be a non-negative integer.
#     zone: (string, optional) The zone where the cluster will be deployed.
#     debug_enabled: (bool, optional) Defaults to False. If True, the Ray cluster will run additional debugging tools, such as Jupyter Lab, SYS_PTRACE capability, etc.
#
# Returns:
#     A dictionary representing the RayJob CRD.
def ray_cluster_spec(
        namespace,
        image,
        head_resource,
        worker_resource,
        worker_instances,
        zone = "",
        debug_enabled = False,
        head_object_store_memory = None,
        worker_object_store_memory = None):
    env = dict(COMMONS_ENV.items())
    env.update(RAY_ENV)
    env.update(os.environ)
    env = [
        {"name": k, "value": v}
        for k, v in env.items()
    ]
    temp_dir = "/mnt/mesos/sandbox"
    export_hadoop_classpath = "export CLASSPATH=$(hadoop classpath --glob)"

    head_cmd = [
        # export_hadoop_classpath,
        # "python3 -m uber.ai.michelangelo.tools.hadoop.setup_hadoop",
        # "(python3 -m uber.ai.michelangelo.tools.hadoop.token_renewer &)",
    ]
    if debug_enabled:
        # Run Jupyter Lab in the head node.
        jupyter_cmd = "; ".join([
            "pip install ipykernel --upgrade",  # https://t3.uberinternal.com/browse/MA-37993
            "jupyter lab --no-browser --port=$JUPYTER_NOTEBOOK_PORT --allow-root --ip=0.0.0.0 --NotebookApp.allow_origin='*' --NotebookApp.token='' --NotebookApp.password=''",
        ])
        head_cmd.append("( " + jupyter_cmd + " & )")

    head_ray_start = [
        # "ray start --head",
        # "--temp-dir={}".format(temp_dir),
        # "--object-manager-port=$OBJECT_MANAGER_PORT",
        # "--port=$RAY_PORT",
        # "--ray-client-server-port=$RAY_CLIENT_PORT",
        # "--dashboard-port=$DASHBOARD_PORT",
        # "--metrics-export-port=$METRICS_EXPORT_PORT",
        # "--dashboard-host='0.0.0.0'",
        # "--dashboard-agent-listen-port=$DASHBOARD_AGENT_LISTEN_PORT",
        # "--resources='{\"head\":1}'",
    ]

    if head_object_store_memory != None:
        head_ray_start.append("--object-store-memory={}".format(head_object_store_memory))

    head_cmd += [
        " ".join(head_ray_start),
        "sleep infinity",
    ]
    head_cmd = " && ".join(head_cmd)

    # Cluster Management CLI: https://docs.ray.io/en/releases-2.9.2/cluster/cli.html#ray-start
    worker_ray_start = [
        # "ray start",
        # "--temp-dir={}".format(temp_dir),
        # "--object-manager-port=$OBJECT_MANAGER_PORT",
        # "--metrics-export-port=$METRICS_EXPORT_PORT",
        # "--address=$RAY_IP",
        # "--worker-port-list={}".format(",".join(["$W_" + str(i) for i in range(worker_resource["cpu"] * 5)])),
        # "--num-cpus={}".format(worker_resource["cpu"]),
        # "--num-gpus={}".format(worker_resource.get("gpu", 0)),
        # "--block",
    ]

    if worker_object_store_memory != None:
        worker_ray_start.append("--object-store-memory={}".format(worker_object_store_memory))

    worker_cmd = " && ".join([
        export_hadoop_classpath,
        "export $(cat /data/head_info.env | tr -d ' ' | xargs -L 1)",
        "python3 -m uber.ai.michelangelo.tools.hadoop.setup_hadoop",
        "echo ray start",
        " ".join(worker_ray_start),
        # Uber Ray Operator injects extra args, however, we don't use them. Source: https://code.uberinternal.com/D12719989
        # Add echo command in the end to consume operator-injected args without breaking the whole command.
        "echo extra: ",
    ])
    support_gpu = head_resource.get("gpu", 0) + worker_resource.get("gpu", 0) * worker_instances > 0
    # match_labels = get_ray_job_match_labels(support_gpu, zone)

    annotations = {}
    if debug_enabled:
        # Add SYS_PTRACE capability for profiling.
        annotations["michelangelo/profiling-ptrace-enabled"] = "true"

    return {
        "metadata": {
            "generateName": "uf-ray-",
            "namespace": "default",
            "annotations": annotations,
        },
        "spec": {
            "user": {"name": "weric"},
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
                                "resources": {
                                    "requests": head_resource,
                                },
                                "image": image,  # Keeping original variable
                                "imagePullPolicy": "Never",
                                "env": env,  # Keeping original variable
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
                    "minInstances": worker_instances,  # Keeping original variable
                    "maxInstances": worker_instances,  # Keeping original variable
                    "nodeType": "worker-group-1",
                    "objectStoreMemoryRatio": 0.0,
                    "rayStartParams": {
                        "block": "true",
                        "dashboard-host": "0.0.0.0",
                    },
                    "pod": {
                        "spec": {
                            "containers": [
                                {
                                    "name": "worker",
                                    "resources": {
                                        "requests": worker_resource,
                                    },
                                    "image": image,  # Keeping original variable
                                    "imagePullPolicy": "Never",
                                    "env": [
                                        {
                                            "name": "configMapRef",
                                            "value": "michelangelo-config",
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

# Get the match labels for the ray job.
# Args:
#   support_gpu: whether the ray job requires gpu
#   zone: the zone of the ray job
#
# Returns:
#   match_labels: the match labels for the ray job
def get_ray_job_match_labels(support_gpu, zone):
    env_label = get_env_label()
    match_labels = {
        RESOURCE_AFFINITY_LABEL_SUPPORT_GPU: str(support_gpu).lower(),
        env_label: "true",
    }

    if zone:
        match_labels[RESOURCE_AFFINITY_LABEL_ZONE] = zone

    project_annotations = get_project_resource_annotation(os.environ["MA_NAMESPACE"], os.environ["MA_NAMESPACE"])
    cloud_zone = project_annotations.get(RESOURCE_AFFINITY_LABEL_CLOUD_ZONE, "")

    return match_labels
