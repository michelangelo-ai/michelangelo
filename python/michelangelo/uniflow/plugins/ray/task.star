load("@plugin", "atexit", "json", "os", "ray", "time")
load("../../commons.star", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

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

USER_ID = os.environ.get("USER_ID", "default_user")
IMAGE_PULL_POLICY = os.environ.get("IMAGE_PULL_POLICY", "Never")

# This function defines the orchestration logic for Ray tasks.
#
# Configures and starts a Ray cluster based on provided specifications and environment,
# then runs a specified Ray job on this cluster. It continuously monitors the job's status, reporting progress
# and handling job completion or failure. The function ensures the Ray cluster is terminated upon job completion
# or failure to release resources.
#
# Parameters:
#     task_path (str): The path to the Ray task to be executed. Ex: uber.ai.michelangelo.sandbox.bert_cola.train.train
#     cache_version (str, optional): The version of the cache to use. If not provided, we will use a default cache version calculated from the image id and apply_to_local_diff/draft.
#     cache_enabled (bool, optional): If True, the task will try to reuse cached results if the same task is run with the same arguments. Otherwise, the task will always run and produce a new cached result.
#     head_cpu (int, optional): The number of CPUs for the head node.
#     head_memory (str, optional): The memory allocation for the head node.
#     head_disk (str, optional): The disk size for the head node.
#     head_gpu (int, optional): The number of GPUs for the head node. Can be 0 if no GPU is required.
#     head_object_store_memory (int, optional): The amount of memory (in bytes) to start the object store with on the head node. https://docs.ray.io/en/releases-2.9.2/cluster/cli.html#cmdoption-ray-start-object-store-memory
#     worker_cpu (int, optional): The number of CPUs for each worker node.
#     worker_memory (str, optional): The memory allocation for each worker node.
#     worker_disk (str, optional): The disk size for each worker node.
#     worker_gpu (int, optional): The number of GPUs for each worker node. Can be 0 if no GPU is required.
#     worker_object_store_memory (int, optional): The amount of memory (in bytes) to start the object store with on each worker node. https://docs.ray.io/en/releases-2.9.2/cluster/cli.html#cmdoption-ray-start-object-store-memory
#     worker_instances (int, optional): The number of worker instances. Can be 0 for head-only clusters.
#     gpu_sku (str, optional): The SKU for GPUs.
#     zone (str, optional): The deployment zone for the cluster.
#     breakpoint (bool, optional): If True, runs the task till completion or failure, however the cluster is not immediately terminated afterwards, allowing time for debugging and profiling the cluster's state.
#
# Returns:
#     callable: A callable function that, when executed, runs the specified Ray job on the configured Ray cluster,
#     monitors its execution, and handles cleanup and reporting.
#
# Note:
#     The function uses environmental variables for overriding resource specifications.
#     It also ensures proper cleanup by terminating the Ray cluster and unregistering exit hooks upon job completion or failure.
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
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get("MA_NAMESPACE", "default")
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)

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
        cluster_namespace = namespace
        cluster_image = get_task_image(task_name)
        print("ray | create cluster:", "ns:", cluster_namespace, "image:", cluster_image, "task_name:", task_name)

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
            debug_enabled = breakpoint,
        )
        cluster = ray.create_cluster(cluster)
        cluster_name = cluster["metadata"]["name"]
        cluster_namespace = cluster["metadata"]["namespace"]

        print("ray | cluster created:", "ns=" + cluster_namespace, "n=" + cluster_name)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_message = "Ray Cluster Created Successfully",
            task_state = TASK_STATE_RUNNING,
            start_time = start_time_formated_str,
            end_time = "",
        )

        def terminate_cluster():
            ray.terminate_cluster(cluster_name, cluster_namespace, "job failed", "TERMINATION_TYPE_FAILED")
            print("ray | cluster terminated:", "ns=" + cluster_namespace, "n=" + cluster_name)

        atexit.register(terminate_cluster)

        # Run job
        result_url = get_result_url()
        entrypoint = ray_job_entrypoint(task_path, result_url, args, kwargs)
        print("ray | run job:", "task_path=" + task_path)
        job = ray.create_job(
            entrypoint,
            ray_job_namespace = cluster_namespace,
            ray_job_name = cluster_name,
        )
        print("ray | +run job: job=" + str(job))

        def report_ray_task_result():
            end_time_seconds = time.time()
            end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
            if job["status"]["state"] == "RAY_JOB_STATE_SUCCEEDED":
                report_progress(
                    task_path = task_path,
                    task_name = task_name,
                    task_message = "Ray Job Succeeded",
                    task_state = TASK_STATE_SUCCEEDED,
                    start_time = start_time_formated_str,
                    end_time = end_time_formated_str,
                )
            elif job["status"]["state"] == "RAY_JOB_STATE_KILLED":
                message = job.get("message", "unknown reason")
                task_message = "Ray Job killed with {}".format(message)
                report_progress(
                    task_path = task_path,
                    task_name = task_name,
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
                    task_message = task_message,
                    task_state = TASK_STATE_FAILED,
                    start_time = start_time_formated_str,
                    end_time = end_time_formated_str,
                )

        atexit.register(report_ray_task_result)

        if breakpoint:
            print("ray | breakpoint:", "ns=" + cluster_namespace, "n=" + cluster_name)

            time.sleep(seconds = 60 * 60 * 24)
            err_message = "internal: breakpoint timeout"
            print("ray | error:", err_message)
            fail(err_message)

        # Terminate cluster
        if job["status"]["state"] == "RAY_JOB_STATE_SUCCEEDED":
            ray.terminate_cluster(cluster_name, cluster_namespace, "job succeeded", "TERMINATION_TYPE_SUCCEEDED")
        else:
            ray.terminate_cluster(cluster_name, cluster_namespace, "job failed", "TERMINATION_TYPE_FAILED")

        report_ray_task_result()
        atexit.unregister(terminate_cluster)
        atexit.unregister(report_ray_task_result)

        # Read result from the storage
        if job["status"]["state"] != "RAY_JOB_STATE_SUCCEEDED":
            fail("internal:", "message:bad job status:", job["status"]["state"], job)

        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            task_message = "Ray Task Completed Successfully",
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

    return "python3 -m michelangelo.uniflow.core.run_task --task '" + task_path + "' --args '" + args + "' --kwargs '" + kwargs + "' --result-url '" + result_url + "'"

# Constructs a Unified API resource for provisioning a Ray Cluster.
# This function generates a RayJob Custom Resource Definition (CRD) that defines the specifications for a Ray cluster.
# Refer to the RayJob CRD: https://github.com/michelangelo-ai/michelangelo/blob/main/proto/api/v2/ray_job.proto
#
# Parameters:
#     namespace (str):
#         - The Unified API namespace, also known as the Michelangelo Project ID.
#         - Example: "ma-dev-test"
#
#     image (str):
#         - The Docker image containing Ray, application code, and dependencies.
#         - Example: "127.0.0.1:5055/uber-usi/uber-one-michelangelo-sandbox:bkt1-produ-1719018451-45448"
#
#     head_resource (dict):
#         - Resource configuration for the Ray **head node**.
#         - Reference: `resource_dict` function in commons.star.
#
#     worker_resource (dict):
#         - Resource configuration for the Ray **worker nodes**.
#         - Reference: `resource_dict` function in commons.star.
#
#     worker_instances (int):
#         - Number of Ray worker instances to launch.
#         - Must be a non-negative integer.
#
#     debug_enabled (bool, optional):
#         - Enables debugging tools if set to True.
#         - Includes additional debugging utilities such as SYS_PTRACE capability.
#         - Defaults to False.
#
# Returns:
#     dict: A dictionary representing the RayJob CRD.
def ray_cluster_spec(
        namespace,
        image,
        head_resource,
        worker_resource,
        worker_instances,
        debug_enabled = False):
    env = dict(COMMONS_ENV.items())
    env.update(RAY_ENV)
    env.update(os.environ)
    env = [
        {"name": k, "value": v}
        for k, v in env.items()
    ]

    support_gpu = head_resource.get("gpu", 0) + worker_resource.get("gpu", 0) * worker_instances > 0

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
            "user": {"name": USER_ID},
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
                                "imagePullPolicy": IMAGE_PULL_POLICY,
                                "env": env,  # Keeping original variable
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
                    "minInstances": worker_instances,
                    "maxInstances": worker_instances,
                    "nodeType": "worker-group-1",
                    "objectStoreMemoryRatio": 0.0,
                    "rayStartParams": {
                        "block": "true",
                        "dashboard-host": "0.0.0.0",
                    },
                    "pod": {
                        "spec": {
                            "restartPolicy": "Never",
                            "containers": [
                                {
                                    "name": "worker",
                                    "resources": {
                                        "requests": worker_resource,
                                    },
                                    "image": image,
                                    "imagePullPolicy": IMAGE_PULL_POLICY,
                                    "env": env,
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
