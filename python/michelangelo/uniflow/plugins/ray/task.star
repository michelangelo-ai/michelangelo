load("@plugin", "atexit", "json", "os", "ray", "time")
load("../../commons.star", "DEFAULT_RETRY_ATTEMPTS", "CACHE_OPERATION_GET", "CACHE_OPERATION_PUT", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "create_cached_output", "get_cache_enabled", "get_cache_keys", "get_cached_output", "get_pythonpath", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

DEFAULT_CREATE_CLUSTER_TIMEOUT_SECONDS = 60 * 30  # Timeout duration for cluster creation in seconds.
RAY_ENV = {
    "RAY_DEDUP_LOGS": "0",
    # RAY_NUM_REDIS_GET_RETRIES controls the number of retries for a worker node to connect to the GCS (Global Control Service) at startup.
    # Source: https://github.com/ray-project/ray/blob/releases/2.9.2/python/ray/_private/node.py#L688
    # The default value is 20, giving a worker about 140 seconds to connect to the GCS (7 seconds per retry).
    # We need to provide workers more time to connect because the Job Controller doesn't support gang scheduling of nodes.
    # As a result, a worker node might start significantly earlier than the head node.
    # Calculate RAY_NUM_REDIS_GET_RETRIES to allow workers approximately DEFAULT_CREATE_CLUSTER_TIMEOUT_SECONDS to connect to the GCS, assuming 7 seconds per retry.
    "RAY_NUM_REDIS_GET_RETRIES": str((DEFAULT_CREATE_CLUSTER_TIMEOUT_SECONDS // 7) + 1),
    "PYTHONPATH": get_pythonpath(),
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

RAY_LOG_URL_PREFIX = os.environ.get("RAY_LOG_URL_PREFIX")

def get_ray_log_url(ray_job_name):
    """
    Generate a log URL for a Ray job based on the job name.
    Only generates URL when RAY_LOG_URL_PREFIX environment variable is provided.
    Expected format: {RAY_LOG_URL_PREFIX}/{ray_job_name}.log

    Args:
        ray_job_name: The name of the Ray job (e.g., "uf-ray-abc123")

    Returns:
        str: The complete log URL or empty string if prefix not configured
    """
    if RAY_LOG_URL_PREFIX and ray_job_name:
        return "{}/{}.log".format(RAY_LOG_URL_PREFIX, ray_job_name)
    return ""

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
#     runtime_env (dict, optional): The runtime environment for the cluster. https://docs.ray.io/en/latest/ray-core/api/doc/ray.runtime_env.RuntimeEnv.html
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
        retry_attempts = DEFAULT_RETRY_ATTEMPTS,
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
        breakpoint = False,
        runtime_env = None):
    def callable(*args, **kwargs):
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get("MA_NAMESPACE", "default")
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
        final_cache_enabled = get_cache_enabled(cache_enabled, task_name)
        if final_cache_enabled:  # Check if the result is cached
            cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_GET)
            print("ray | cache enabled with key", "key:", cache_keys)
            cached_output = get_cached_output(namespace, cache_keys)
            if cached_output != None:
                cached_result_json_url = cached_output.get("spec", {}).get("storageUri", "")
                if cached_result_json_url != "":
                    print("ray | found cache output", "cached_result_json_url:", cached_result_json_url)
                    end_time_seconds = time.time()
                    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
                    report_progress(
                        task_path = task_path,
                        task_name = task_name,
                        task_log = "",
                        task_message = "Ray Task Skipped with Cache Hit",
                        task_state = TASK_STATE_SKIPPED,
                        start_time = start_time_formated_str,
                        end_time = end_time_formated_str,
                        output = cached_output.get("metadata", {}).get("name", ""),
                        retry_attempt_id = "",
                    )
                    result = io_read_json(cached_result_json_url)
                    print("ray | cached", "result:", result)
                    return result

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

        _retry_attempts = retry_attempts

        # Apply resource types
        _head_cpu = int(_head_cpu)
        _head_gpu = int(_head_gpu)
        _worker_cpu = int(_worker_cpu)
        _worker_gpu = int(_worker_gpu)
        _worker_instances = int(_worker_instances)

        result_url = get_result_url()

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
            runtime_env = runtime_env,
        )

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):

            job_state, job, cluster_url, ray_job_name = execute_ray_task(
                task_path=task_path,
                task_name=task_name,
                cluster=cluster,
                cluster_namespace=cluster_namespace,
                runtime_env=runtime_env,
                start_time_formated_str=start_time_formated_str,
                result_url=result_url,
                args=args,
                kwargs=kwargs,
                retry_attempt_id=retry_attempt_id,
                total_retry_attempt=total_retry_attempt,
                cache_version=cache_version,
                namespace=namespace,
                breakpoint=breakpoint,
            )

            retryable = process_terminated_ray_job(
                job_state,
                job,
                task_name,
                task_path,
                args,
                kwargs,
                cache_version,
                namespace,
                result_url,
                start_time_formated_str,
                retry_attempt_id,
                total_retry_attempt,
                cluster_url,
                ray_job_name,
            )

            if retryable == False:
                break

        result = io_read_json(result_url)
        print("ray | caching", "result:", result)
        return result

    def with_overrides(alias = alias, config = ray_config(), retry_attempts = DEFAULT_RETRY_ATTEMPTS):
        return task(
            task_path = task_path,
            alias = alias,
            cache_version = cache_version,
            cache_enabled = cache_enabled,
            retry_attempts = retry_attempts,
            head_cpu = head_cpu if "head_cpu" not in config else config["head_cpu"],
            head_memory = head_memory if "head_memory" not in config else config["head_memory"],
            head_disk = head_disk if "head_disk" not in config else config["head_disk"],
            head_gpu = head_gpu if "head_gpu" not in config else config["head_gpu"],
            head_object_store_memory = head_object_store_memory if "head_object_store_memory" not in config else config["head_object_store_memory"],
            worker_cpu = worker_cpu if "worker_cpu" not in config else config["worker_cpu"],
            worker_memory = worker_memory if "worker_memory" not in config else config["worker_memory"],
            worker_disk = worker_disk if "worker_disk" not in config else config["worker_disk"],
            worker_gpu = worker_gpu if "worker_gpu" not in config else config["worker_gpu"],
            worker_object_store_memory = worker_object_store_memory if "worker_object_store_memory" not in config else config["worker_object_store_memory"],
            worker_instances = worker_instances if "worker_instances" not in config else config["worker_instances"],
            gpu_sku = gpu_sku if "gpu_sku" not in config else config["gpu_sku"],
            zone = zone if "zone" not in config else config["zone"],
            breakpoint = breakpoint if "breakpoint" not in config else config["breakpoint"],
            runtime_env = runtime_env if "runtime_env" not in config else config["runtime_env"],
        )

    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable

def process_terminated_ray_job(job_state, job, task_name, task_path, args, kwargs, cache_version, namespace, result_url, start_time_formated_str, retry_attempt_id, total_retry_attempt, cluster_url, ray_job_name):

    retryable = False

    if job_state == TASK_STATE_SUCCEEDED:

        cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_PUT)
        created_cached_output = create_cached_output(
            namespace = namespace,
            cache_keys = cache_keys,
            zone = "",
            ttl_in_days = 0,
            task_name = task_name,
            result_json_url = result_url,
        )
        cached_output_name = created_cached_output.get("metadata", {}).get("name", "")
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)

        # Generate log URL from Ray job name
        generated_log_url = get_ray_log_url(ray_job_name)
        log_url = generated_log_url if generated_log_url else cluster_url

        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_SUCCEEDED,
            task_log = log_url,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            task_message = "Ray Task Completed Successfully",
            output = cached_output_name,
            retry_attempt_id = retry_attempt_id,
        )
        print("Ray job succeeded, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") succeeded")

    elif job_state == TASK_STATE_KILLED:
        print("Ray task killed, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ").  no retry should be performed")
        fail("Ray task killed, no retry should be performed")

    elif job_state == TASK_STATE_FAILED:
        print("Ray task failed, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") failed")
        if retry_attempt_id < total_retry_attempt:
            retryable = True
        else:
            print("Ray task failed after all (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") attempts were exhausted")
            fail("Ray task failed after all retry attempts were exhausted ", "internal:", "message:bad job status:", job["status"]["state"], job)

    return retryable


def execute_ray_task(task_path, task_name, cluster, cluster_namespace, runtime_env, start_time_formated_str, result_url, args, kwargs, retry_attempt_id, total_retry_attempt, cache_version, namespace, breakpoint=False):

    print("Ray job running, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ")")
    report_progress(
        task_path = task_path,
        task_name = task_name,
        task_log = "",
        task_message = "Provisioning Ray Cluster...",
        task_state = TASK_STATE_PENDING,
        start_time = start_time_formated_str,
        end_time = "",
        retry_attempt_id = retry_attempt_id,
    )

    cluster = ray.create_cluster(cluster, timeout_seconds = DEFAULT_CREATE_CLUSTER_TIMEOUT_SECONDS)
    cluster_url = cluster["status"].get("jobUrl", "UAPI did not report RayJob URL")
    cluster_name = cluster["metadata"]["name"]
    cluster_namespace = cluster["metadata"]["namespace"]

    print("ray | cluster created:", "ns=" + cluster_namespace, "n=" + cluster_name, "url=" + cluster_url)
    report_progress(
        task_path = task_path,
        task_name = task_name,
        task_log = cluster_url,
        task_message = "Ray Cluster Created Successfully",
        task_state = TASK_STATE_RUNNING,
        start_time = start_time_formated_str,
        end_time = "",
        output = "",
        retry_attempt_id = retry_attempt_id,
    )

    atexit.register(terminate_cluster, cluster_namespace, cluster_name)

    # Run job
    entrypoint = ray_job_entrypoint(task_path, result_url, args, kwargs)
    print("ray | run job:", "task_path=" + task_path)
    job = ray.create_job(
        entrypoint,
        ray_job_namespace = cluster_namespace,
        ray_job_name = cluster_name,
    )
    print("ray | +run job: job=" + str(job))

    # Extract Ray job ID/name from job object - try job ID first, then metadata name, then cluster name
    ray_job_name = (job.get("spec", {}).get("jobId") or
                   job.get("status", {}).get("jobId") or
                   job.get("metadata", {}).get("name", cluster_name))
    generated_log_url = get_ray_log_url(ray_job_name)
    log_url = generated_log_url if generated_log_url else cluster_url

    atexit.register(report_ray_task_result, job, task_path, task_name, cluster_url, start_time_formated_str, args, kwargs, retry_attempt_id, cache_version, namespace, result_url)

    if breakpoint:
        print("ray | breakpoint:", "ns=" + cluster_namespace, "n=" + cluster_name)

        time.sleep(seconds = 60 * 60 * 24)
        err_message = "internal: breakpoint timeout"
        print("ray | error:", err_message)
        fail(err_message)

    # Terminate cluster
    job_state = report_ray_task_result(job, task_path, task_name, cluster_url, start_time_formated_str, args, kwargs, retry_attempt_id, cache_version, namespace, result_url)
    if job_state == TASK_STATE_SUCCEEDED:
        ray.terminate_cluster(cluster_name, cluster_namespace, "job succeeded", "TERMINATION_TYPE_SUCCEEDED")
    else:
        ray.terminate_cluster(cluster_name, cluster_namespace, "job failed", "TERMINATION_TYPE_FAILED")

    atexit.unregister(terminate_cluster)
    atexit.unregister(report_ray_task_result)

    return(job_state, job, cluster_url, ray_job_name)

def terminate_cluster(cluster_namespace, cluster_name):
    ray.terminate_cluster(cluster_name, cluster_namespace, "job failed", "TERMINATION_TYPE_FAILED")
    print("ray | cluster terminated:", "ns=" + cluster_namespace, "n=" + cluster_name)

def report_ray_task_result(job, task_path, task_name, cluster_url, start_time_formated_str, args, kwargs, retry_attempt_id, cache_version, namespace, result_url):
    end_time_seconds = time.time()
    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)

    cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_PUT)
    created_cached_output = create_cached_output(
        namespace = namespace,
        cache_keys = cache_keys,
        zone = "",
        ttl_in_days = 0,
        task_name = task_name,
        result_json_url = result_url,
    )
    cached_output_name = created_cached_output.get("metadata", {}).get("name", "")

    if job["status"]["state"] == "RAY_JOB_STATE_SUCCEEDED":
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = cluster_url,
            task_message = "Ray Job Succeeded",
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            output = cached_output_name,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_SUCCEEDED
    elif job["status"]["state"] == "RAY_JOB_STATE_KILLED":
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
            output = cached_output_name,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_KILLED
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
            output = cached_output_name,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_FAILED

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
#     runtime_env (dict, optional):
#         - The runtime environment for the cluster. https://docs.ray.io/en/latest/ray-core/api/doc/ray.runtime_env.RuntimeEnv.html
#
# Returns:
#     dict: A dictionary representing the RayJob CRD.
def ray_cluster_spec(
        namespace,
        image,
        head_resource,
        worker_resource,
        worker_instances,
        debug_enabled = False,
        runtime_env = None):
    ray_init_kwargs = os.environ.get("_RAY_INIT_KWARGS", {})
    ray_init_kwargs["runtime_env"] = runtime_env
    env = dict(COMMONS_ENV.items())
    env.update(RAY_ENV)
    env.update(os.environ)
    env.update({"_RAY_INIT_KWARGS": str(ray_init_kwargs)})
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
                        # TODO: Uncomment below to enable GPU node scheduling
                        # "tolerations": [
                        #     {"key": "nvidia.com/gpu", "operator": "Exists", "effect": "NoSchedule"}
                        # ] if support_gpu else [],
                        # "nodeSelector": {
                        #     "cloud.google.com/gke-accelerator": gpu_sku
                        # } if support_gpu and gpu_sku else {},
                        "volumes": [
                            {
                                "name": "ray",
                                "volumeSource": {
                                    "hostPath": {
                                        "path": "/tmp/ray"
                                    }
                                },
                            },
                        ],
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
                                "volumeMounts": [
                                    {
                                        "name": "ray",
                                        "mountPath": "/tmp/ray",
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
                            # TODO: Uncomment below to enable GPU node scheduling
                            # "tolerations": [
                            #     {"key": "nvidia.com/gpu", "operator": "Exists", "effect": "NoSchedule"}
                            # ] if support_gpu else [],
                            # "nodeSelector": {
                            #     "cloud.google.com/gke-accelerator": gpu_sku
                            # } if support_gpu and gpu_sku else {},
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

def ray_config(
        head_cpu = None,
        head_memory = None,
        head_disk = None,
        head_gpu = None,
        head_object_store_memory = None,
        worker_cpu = None,
        worker_memory = None,
        worker_disk = None,
        worker_gpu = None,
        worker_object_store_memory = None,
        worker_instances = None,
        breakpoint = None, 
        runtime_env = None):
    config_overrides = {
        "head_cpu": head_cpu,
        "head_memory": head_memory,
        "head_disk": head_disk,
        "head_gpu": head_gpu,
        "head_object_store_memory": head_object_store_memory,
        "worker_cpu": worker_cpu,
        "worker_memory": worker_memory,
        "worker_disk": worker_disk,
        "worker_gpu": worker_gpu,
        "worker_object_store_memory": worker_object_store_memory,
        "worker_instances": worker_instances,
        "breakpoint": breakpoint,
        "runtime_env": runtime_env,
    }
    return {key: value for key, value in config_overrides.items() if value != None}