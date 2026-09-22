load("@plugin", "atexit", "cachedoutput", "hashlib", "json", "os", "progress", "spark", "storage", "time", "uuid", "workflow")

ENV = {
    "UF_REMOTE_RUN": "1",
}

TASK_STATE_PENDING = progress.task_state_pending
TASK_STATE_RUNNING = progress.task_state_running
TASK_STATE_SUCCEEDED = progress.task_state_succeeded
TASK_STATE_FAILED = progress.task_state_failed
TASK_STATE_KILLED = progress.task_state_killed
TASK_STATE_SKIPPED = progress.task_state_skipped
TIME_FOMART = "%Y-%m-%d %H:%M:%S"

CACHE_KEY_TASK_PATH = "michelangelo/uniflow-task-path"
CACHE_KEY_INPUT_HASH = "michelangelo/uniflow-input-hash"
CACHE_KEY_CACHE_VERSION = "michelangelo/uniflow-cache-version"
LABEL_VALUE_SIZE_LIMIT = 60
CACHE_KEY_RANDOM_PREFIX_DIGEST_SIZE = 4
CACHE_ENABLED_ENV = "CACHE_ENABLED"
CACHE_VERSION_ENV = "CACHE_VERSION"
CACHE_OPERATION_PUT = "PUT"
CACHE_OPERATION_GET = "GET"
CACHE_ENABLED_TRUE = "true"
CACHE_ENABLED_FALSE = "false"

DEFAULT_RETRY_ATTEMPTS = 0

def get_result_url():
    """
    Get the url for the result.json
    """
    metadata_storage_url = os.environ.get("UF_METADATA_STORAGE_URL", os.environ["UF_STORAGE_URL"])
    result_url = "{}/{}.json".format(metadata_storage_url, uuid.uuid4().hex)
    return result_url

# The url is expected to be in format of scheme://host/path
def io_read_json(url):
    return storage.read(url)

# Get the task image for the task.
# Args:
#    task_name: the name of the task
# Returns:
#    task_image: the task image for the task

def get_task_image(task_name):
    global_image = os.environ.get("UF_TASK_IMAGE", "")
    task_image = os.environ.get("UF_TASK_IMAGE_" + task_name, global_image)
    if task_image == "":
        fail("failed to get task image:", task_name)
    return task_image

# Get the task name for the task.
# Args:
#    task_path: the path of the task
#    alias: the alias of the task
# Returns:
#    task_name: the name of the task
def get_task_name(task_path, alias):
    if alias != None:
        return alias
    return task_path.split(".")[-1]

def resource_dict(cpu, memory, disk = None, gpu = None, gpu_sku = ""):
    res = {
        "cpu": cpu,
        "memory": memory,
    }
    if disk:
        res["diskSize"] = disk
    if gpu:
        res["gpu"] = gpu
    if gpu_sku:
        res["gpu_sku"] = gpu_sku
    return res

def report_progress(task_path, task_name, task_log = "", task_message = "", task_state = "", start_time = "", end_time = "", output = "", retry_attempt_id = "", first_activity_id = "", activity_id = "", input = ""):
    if type(retry_attempt_id) != "str":
        retry_attempt_id = str(retry_attempt_id)
    state_dict = {
        "task_path": task_path,
        "task_name": task_name,
        "task_log": task_log,
        "task_message": task_message,
        "task_state": task_state,
        "start_time": start_time,
        "end_time": end_time,
        "output": output,
        "retry_attempt_id": retry_attempt_id,
        "first_activity_id": first_activity_id,
        "current_activity_id": activity_id,
        "input": input,
    }
    progress.report(str(state_dict))

def get_input_hash(args, kwargs):
    """
    Get the input hash for the task.

    Args:
        args: input arguments
        kwargs: input keyword arguments
    Returns:
        input_hash: the input hash
    """
    args = json.dumps(args) if args else "[]"
    kwargs = json.dumps(kwargs) if kwargs else "{}"
    input_hash = hashlib.blake2b_hex(args + kwargs, digest_size = 16)
    return input_hash

def get_cache_enabled(cache_enabled, task_name):
    if cache_enabled:
        return cache_enabled
    cache_enabled = os.environ.get("{}_{}".format(CACHE_ENABLED_ENV, task_name), os.environ.get(CACHE_ENABLED_ENV, CACHE_ENABLED_FALSE))
    return cache_enabled == CACHE_ENABLED_TRUE

#Get the cache version for the task.
#   Args:
#       cache_version: the version of the cache
#       task_name: the name of the task
#       operation: PUT or GET
#   Returns:
#       final_cache_version: the final version of the cache
def get_cache_version(cache_version, task_name, operation):
    if cache_version == None:
        # try to get cache_version from envs
        cache_version = os.environ.get(
            "{}_{}_{}".format(CACHE_VERSION_ENV, operation, task_name),  # task-level override
            os.environ.get(CACHE_VERSION_ENV, None),  # workflow-level override
        )

    if cache_version == None:
        image = get_task_image(task_name)
        return hashlib.blake2b_hex(image, digest_size = 16)

    return get_label_value(cache_version)

# Get the cache keys for the task.
#   Args:
#       task_path: the path of the task
#       task_name: the name of the task
#       args: input arguments
#       kwargs: input keyword arguments
#       cache_version: the version of the cache
#       operation: PUT or GET
#   Returns:
#      cache_keys: the cache keys
def get_cache_keys(task_path, task_name, args, kwargs, cache_version, operation):
    final_task_path = get_task_path(task_path)
    final_cache_version = get_cache_version(cache_version, task_name, operation)
    final_input_hash = get_input_hash(args, kwargs)

    cache_keys = {
        CACHE_KEY_TASK_PATH: final_task_path,
        CACHE_KEY_INPUT_HASH: final_input_hash,
        CACHE_KEY_CACHE_VERSION: final_cache_version,
    }
    return cache_keys

def get_label_value(value):
    """
    Get the label value for the task. The label will be saved as CachedOutput label.

    If the value is longer than 63, it will be shortened.

    Args:
        value: the value of the label
    Returns:
        value: the shortened value of the label
    """
    if len(value) > LABEL_VALUE_SIZE_LIMIT:
        value_hash = hashlib.blake2b_hex(value, digest_size = CACHE_KEY_RANDOM_PREFIX_DIGEST_SIZE)
        value = value_hash + "-" + value[-(LABEL_VALUE_SIZE_LIMIT - CACHE_KEY_RANDOM_PREFIX_DIGEST_SIZE * 2 - 1):]
    return value

def create_cached_output(namespace, task_name, cache_keys, zone, ttl_in_days, result_json_url):
    """
    Build the cache output for the task.

    Args:
        namespace: the namespace of the task
        task_name: the name of the task
        cache_keys: a dictionary of the cache keys
        zone: the zone of the cache
        ttl_in_days: the ttl of the cache
        result_json_url: the dir url of the result.json
    Returns:
        cached_output: the created cached output
    """
    new_cachedoutput = {
        "metadata": {
            "namespace": namespace,
            "generateName": "uf-vars-",
            "labels": cache_keys,
            "annotations": {
                "michelangelo/Immutable": "true",  # cachedoutputs are created as immutable
            },
        },
        "spec": {
            "storage_uri": result_json_url,
            "type": "CACHED_OUTPUT_TYPE_VARIABLE",
            "zone": zone,
            "ttl_in_days": ttl_in_days,
            "storage_type": get_storage_type(result_json_url),
            # TODO: add source_pipeline_run resource identifier
            "source_pipeline_run_step": task_name,
            "variable_spec": {
                "type": "VARIABLE_TYPE_CUSTOM",
            },
        },
    }
    created_cached_output = cachedoutput.put(cachedoutput = new_cachedoutput)
    return created_cached_output

def get_cached_output(namespace, cache_keys, lookback_days = 28):
    """
    Get the cached result json url for the task.

    Args:
        namespace: the namespace of the task
        cache_keys: a dictionary of the cache keys
        lookback_days: the look back days for the cache
    Returns:
        cached_output: the cached output returned based on the cache keys
    """

    match_criterion = {}
    for cache_key_name, cache_key_value in cache_keys.items():
        match_criterion["cached_output.label.{}".format(cache_key_name)] = cache_key_value

    order_by = [
        {
            "field": "metadata.update_timestamp",
            "dir": 2,
        },
    ]
    response = cachedoutput.query(
        namespace = namespace,
        match_criterion = match_criterion,
        order_by = order_by,
        lookback_days = lookback_days,
        limit = 1,
    )
    cached_output_list = response.get("cachedOutputList", {})
    cached_outputs = cached_output_list.get("items", [])
    if cached_outputs == None or len(cached_outputs) == 0:
        return None
    return cached_outputs[0]

def get_task_path(task_path):
    """
    Get the task path for the task.

    Args:
        task_path: the path of the task
    Returns:
        final_task_path: the final path of the task.
    """
    return get_label_value(task_path)

def get_storage_type(result_json_url):
    """
    Get the storage type for the task.

    Args:
        result_json_url: the dir url of the result.json
    Returns:
        storage_type: the storage type for CachedOutput
    """
    if result_json_url.startswith("s3://"):
        storage_type = "STORAGE_TYPE_S3"
    else:
        storage_type = "STORAGE_TYPE_INVALID"
    return storage_type

def get_pythonpath():
    """
    Get PYTHONPATH environment variable with file sync support.

    When file sync is enabled (UF_FILE_SYNC_TARBALL_URL is set), appends the
    sitecustomize.py directory to PYTHONPATH. This ensures sitecustomize.py runs
    automatically on container startup to apply local code changes before task execution.

    Returns:
        PYTHONPATH value, defaulting to "/app" if not set by user.
        If file sync is enabled, appends ":/app/michelangelo/uniflow/core" to the path.
    """
    # Get existing PYTHONPATH from environment, default to /app if not set
    pythonpath = os.environ.get("PYTHONPATH", "/app")
    if os.environ.get("UF_FILE_SYNC_TARBALL_URL", "") != "":
        # Append sitecustomize.py location to enable automatic file sync on container startup
        pythonpath = pythonpath + ":/app/michelangelo/uniflow/core"
    return pythonpath

def process_terminated_job(
        job_state,
        task_name,
        task_path,
        args,
        kwargs,
        cache_version,
        namespace,
        result_url,
        start_time_formatted_str,
        retry_attempt_id,
        total_retry_attempt,
        job_type,
        log_url):
    """
    Process the result of a terminated job (Spark or Ray).

    This function handles the common logic for processing job termination states
    across different job execution engines (Spark, Ray, etc.).

    Args:
        job_state: The final state of the job (SUCCEEDED, FAILED, or KILLED)
        task_name: The name of the task
        task_path: The path of the task
        args: Input arguments to the task
        kwargs: Input keyword arguments to the task
        cache_version: The version of the cache
        namespace: The namespace of the task
        result_url: The URL where the result is stored
        start_time_formatted_str: The formatted start time string
        retry_attempt_id: The current retry attempt number
        total_retry_attempt: The total number of retry attempts
        job_type: The type of job ("Spark" or "Ray") for message customization
        log_url: The URL to the job logs

    Returns:
        retryable: Boolean indicating whether the job should be retried
    """
    retryable = False

    if job_state == TASK_STATE_SUCCEEDED:
        cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_PUT)
        print("{} | caching with key".format(job_type.lower()), "key:", cache_keys)
        created_cached_output = create_cached_output(
            namespace = namespace,
            cache_keys = cache_keys,
            zone = "",
            ttl_in_days = 0,
            task_name = task_name,
            result_json_url = result_url,
        )
        end_time_seconds = time.time()
        end_time_formatted_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)

        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = log_url,
            task_message = "{} job succeeded".format(job_type) if job_type == "Spark" else "{} Task Completed Successfully".format(job_type),
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formatted_str,
            end_time = end_time_formatted_str,
            output = created_cached_output.get("metadata", {}).get("name", ""),
            retry_attempt_id = retry_attempt_id,
            input = json.dumps({"args": args, "kwargs": kwargs}) if (args or kwargs) else "",
        )
        print("{} job succeeded, attempt ({} / {}) succeeded".format(job_type, str(retry_attempt_id), str(total_retry_attempt)))

    elif job_state == TASK_STATE_KILLED:
        print("{} job killed, attempt ({} / {}). no retry should be performed".format(job_type, str(retry_attempt_id), str(total_retry_attempt)))
        fail("{} job killed, no retry should be performed".format(job_type))

    elif job_state == TASK_STATE_FAILED:
        print("{} job failed, attempt ({} / {}) failed".format(job_type, str(retry_attempt_id), str(total_retry_attempt)))
        if retry_attempt_id < total_retry_attempt:
            retryable = True
        else:
            print("{} job failed after all ({} / {}) attempts were exhausted".format(job_type, str(retry_attempt_id), str(total_retry_attempt)))
            fail("{} job failed after all attempts were exhausted".format(job_type))

    return retryable

# Shared SparkJob-CRD submission/sensor/report logic for plugins that run work
# via the SparkJob CRD + Spark Operator (spark.create_job / spark.sensor_job) -
# today spark_task and scala_task. Parameterized by job_label ("Spark"/"Scala")
# and log_url_prefix so each caller gets byte-identical messages to before.

def get_job_log_url(log_url_prefix, job_name):
    """
    Generate a log URL for a SparkJob-CRD-based job, based on the job name.
    Only generates a URL when log_url_prefix is provided.
    Expected format: {log_url_prefix}/{job_name}.log

    Args:
        log_url_prefix: the configured log URL prefix, or None/"" if unset
        job_name: the name of the SparkJob (e.g., "uniflow-sp-abc123")

    Returns:
        str: the complete log URL or empty string if prefix not configured
    """
    if log_url_prefix and job_name:
        return "{}/{}.log".format(log_url_prefix, job_name)
    return ""

def build_spark_crd_job(
        image,
        main_file,
        main_class,
        main_args,
        driver_resource,
        executor_resource,
        executor_instances,
        generate_name_prefix,
        env):
    """
    Build the SparkJob CRD dict shared by spark_task and scala_task.

    Args:
        image: the task image for driver and executor pods
        main_file: the Spark mainApplicationFile (a run_task.py wrapper for
            spark_task, or the user's own JAR for scala_task)
        main_class: the Spark mainClass
        main_args: list of args passed to mainClass (empty for scala_task)
        driver_resource: resource_dict() for the driver pod
        executor_resource: resource_dict() for the executor pod
        executor_instances: number of executor instances
        generate_name_prefix: CRD metadata.generateName prefix
            (e.g. "uniflow-sp-" or "uniflow-sc-")
        env: list of {"name":..., "value":...} env var dicts for driver/executor pods
    Returns:
        spark_crd_job: the SparkJob CRD dict
    """
    preemptible = True

    return {
        "kind": "SparkJob",
        "apiVersion": "michelangelo.api.v2",
        "metadata": {
            "namespace": "default",
            "generateName": generate_name_prefix,
        },
        "spec": {
            "user": {
                "name": "test",
            },
            "driver": {
                "pod": {
                    "resource": driver_resource,
                    "image": image,
                    "imagePullingPolicy": "Never",
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
                },
            },
            "executor": {
                "pod": {
                    "resource": executor_resource,
                    "image": image,
                    "imagePullingPolicy": "Never",
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
                },
                "instances": executor_instances,
            },
            "sparkConf": {
                "spark.peloton.run-as-user": "true",
                "spark.peloton.driver.docker.image": image,
                "spark.peloton.executor.docker.image": image,
                "spark.peloton.usecrets.enable": "true",
                "spark.sql.optimizer.excludedRules": "org.apache.spark.sql.catalyst.optimizer.MergeScalarSubqueries",
                "spark.sql.adaptive.enabled": "false",
                "spark.driver.extraJavaOptions": "-Dcontainer.log.enableTerraBlobIntegration=true",
                "spark.executor.extraJavaOptions": "-Dcontainer.log.enableTerraBlobIntegration=true",
            },
            "mainApplicationFile": main_file,
            "mainArgs": main_args,
            "mainClass": main_class,
            "deps": {},
            "scheduling": {
                "preemptible": preemptible,
            },
            "sparkVersion": "3.5.5",
        },
    }

def report_spark_crd_job_terminated(
        job,
        task_name,
        task_path,
        start_time_formatted_str,
        retry_attempt_id,
        first_activity_id,
        job_label,
        log_url_prefix,
        unexpected_exit = False,
        job_name = ""):
    """
    Report task progress based on the succeeded/killed conditions of a
    SparkJob-CRD-based job (shared by spark_task and scala_task).

    Args:
        job: the SparkJob crd
        task_name: the task name
        task_path: the task path
        start_time_formatted_str: the UTC formatted string of the task start time
        retry_attempt_id: the attempt id
        first_activity_id: the first activity id
        job_label: "Spark" or "Scala", used verbatim in report messages
        log_url_prefix: the configured log URL prefix for this job type
        unexpected_exit: whether the job failed unexpectedly
        job_name: the SparkJob's generated metadata.name, for log URL generation
    Returns:
        The job state, one of the following:
            - TASK_STATE_SUCCEEDED
            - TASK_STATE_KILLED
            - TASK_STATE_FAILED
    """
    if type(job) != "dict":
        return TASK_STATE_FAILED

    conditions = job.get("status", {}).get("statusConditions", [])
    driver_log_url = job.get("status", {}).get("jobUrl", "")
    generated_log_url = get_job_log_url(log_url_prefix, job_name)
    log_url = generated_log_url if generated_log_url else driver_log_url
    end_time_seconds = time.time()
    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
    killed_condition = None
    succeeded_condition = None

    # we find the succeeded condition and the killed condition
    for condition in conditions:
        if condition == None:
            continue
        if condition["type"] == spark.succeeded_condition_type:
            succeeded_condition = condition
        if condition["type"] == spark.killed_condition_type:
            killed_condition = condition

    if killed_condition != None:
        killed_status = killed_condition.get("status", "CONDITION_STATUS_UNKNOWN")
        if killed_status == "CONDITION_STATUS_TRUE":
            report_progress(
                task_name = task_name,
                task_path = task_path,
                task_state = TASK_STATE_KILLED,
                start_time = start_time_formatted_str,
                end_time = end_time_formated_str,
                task_message = "{}: {}".format(killed_condition.get("message", "{} job killed".format(job_label)), killed_condition.get("reason", "unknown reason")),
                task_log = log_url,
                retry_attempt_id = retry_attempt_id,
                activity_id = first_activity_id,
            )
            return TASK_STATE_KILLED

    if succeeded_condition != None:
        succeeded_status = succeeded_condition.get("status", "CONDITION_STATUS_UNKNOWN")
        if succeeded_status == "CONDITION_STATUS_TRUE":
            report_progress(
                task_name = task_name,
                task_path = task_path,
                task_state = TASK_STATE_SUCCEEDED,
                start_time = start_time_formatted_str,
                end_time = end_time_formated_str,
                task_message = "{} job succeeded".format(job_label),
                task_log = log_url,
                retry_attempt_id = retry_attempt_id,
                activity_id = first_activity_id,
            )
            return TASK_STATE_SUCCEEDED

        if succeeded_status == "CONDITION_STATUS_FALSE":
            message = succeeded_condition.get("message", "{} job failed".format(job_label))
            reason = succeeded_condition.get("reason", "unknown reason")
            report_progress(
                task_name = task_name,
                task_path = task_path,
                task_state = TASK_STATE_FAILED,
                start_time = start_time_formatted_str,
                end_time = end_time_formated_str,
                task_message = "{}:{}".format(reason, message),
                task_log = log_url,
                retry_attempt_id = retry_attempt_id,
                activity_id = first_activity_id,
            )
            if unexpected_exit == True:
                fail("{} job failed: {} {} driver url: {}".format(job_label.lower(), reason, message, driver_log_url))
            return TASK_STATE_FAILED

    return ""

def check_spark_crd_job_final_state_at_workflow_exit(
        created_spark_job,
        task_name,
        task_path,
        start_time_formatted_str,
        retry_attempt_id,
        first_activity_id,
        job_label,
        log_url_prefix):
    """
    Check the final state of a SparkJob-CRD-based job at workflow exit, to
    ensure task state is reported correctly even if the workflow is killed.
    """
    final_job = spark.sensor_job(job = created_spark_job)
    report_spark_crd_job_terminated(final_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, job_label, log_url_prefix, unexpected_exit = True)
    return

def execute_spark_crd_job(
        namespace,
        task_name,
        task_path,
        spark_crd_job,
        start_time_formatted_str,
        retry_attempt_id,
        total_retry_attempt,
        job_label,
        log_url_prefix):
    """
    Submit a SparkJob CRD and sense it through to a terminal state, shared by
    spark_task and scala_task.

    Args:
        namespace: the namespace to submit the job in
        task_name: the task name
        task_path: the task path
        spark_crd_job: the SparkJob CRD dict, e.g. from build_spark_crd_job()
        start_time_formatted_str: the UTC formatted string of the task start time
        retry_attempt_id: the current attempt number
        total_retry_attempt: the total number of attempts
        job_label: "Spark" or "Scala", used verbatim in print/report messages
        log_url_prefix: the configured log URL prefix for this job type
    Returns:
        (job_state, terminated_job) tuple
    """
    log_prefix = job_label.lower()
    print("{} job running, attempt ({} / {})".format(job_label, str(retry_attempt_id), str(total_retry_attempt)))

    driver_log_url = ""

    # submit spark job
    print("{} | submit job. ns:".format(log_prefix), namespace, "task_name:", task_name)

    spark_job_response = spark.create_job(spark_crd_job)

    created_spark_job = spark_job_response["sparkJob"]
    first_activity_id = spark_job_response["activityId"]

    print("{} | first activity ID:".format(log_prefix), first_activity_id)

    if created_spark_job == None:
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = "",
            task_message = "{} Job Creation Failed".format(job_label),
            task_state = TASK_STATE_FAILED,
            start_time = start_time_formatted_str,
            end_time = end_time_formated_str,
            output = "",
            retry_attempt_id = retry_attempt_id,
            first_activity_id = first_activity_id,
        )
        fail("{} | job creation failed, activityId=".format(log_prefix) + first_activity_id)

    print("{} | job created:".format(log_prefix), "ns=" + namespace, "task_name=" + task_name)

    spark_job_name = ""
    if type(created_spark_job) == "dict":
        driver_log_url = created_spark_job.get("status", {}).get("jobUrl", "")
        spark_job_name = created_spark_job.get("metadata", {}).get("name", "")

    generated_log_url = get_job_log_url(log_url_prefix, spark_job_name)

    report_progress(
        task_path = task_path,
        task_name = task_name,
        task_log = generated_log_url if generated_log_url else driver_log_url,
        task_message = "{} job has been submitted".format(job_label),
        task_state = TASK_STATE_PENDING,
        start_time = start_time_formatted_str,
        end_time = "",
        output = "",
        retry_attempt_id = retry_attempt_id,
        first_activity_id = first_activity_id,
    )

    # register the check function to be called at the end of the workflow.
    # this is to ensure that the task state is reported correctly even if the
    # workflow is killed
    atexit.register(check_spark_crd_job_final_state_at_workflow_exit, created_spark_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, job_label, log_url_prefix)

    # sensor spark job until it is running and driver log url is available
    running_job = spark.sensor_job(job = created_spark_job, assert_condition_type = spark.running_condition_type)
    print("{} | job running. ns:".format(log_prefix), namespace, "task_name:", task_name)

    if type(running_job) == "dict":
        driver_log_url = running_job.get("status", {}).get("jobUrl", "")

    report_progress(
        task_name = task_name,
        task_path = task_path,
        task_state = TASK_STATE_RUNNING,
        start_time = start_time_formatted_str,
        task_message = "{} job is running".format(job_label),
        task_log = generated_log_url if generated_log_url else driver_log_url,
        retry_attempt_id = retry_attempt_id,
        activity_id = first_activity_id,
    )

    # sensor spark job until it is terminated
    print("{} | sensor job until terminated. ns:".format(log_prefix), namespace, "task_name:", task_name)
    terminated_job = spark.sensor_job(job = created_spark_job)
    print("{} | job terminated. ns:".format(log_prefix), namespace, "task_name:", task_name)
    job_state = report_spark_crd_job_terminated(terminated_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, job_label, log_url_prefix, job_name = spark_job_name)
    if job_state == TASK_STATE_SUCCEEDED:
        atexit.unregister(check_spark_crd_job_final_state_at_workflow_exit)

    return (job_state, terminated_job)
