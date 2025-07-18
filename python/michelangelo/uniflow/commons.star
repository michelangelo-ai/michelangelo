load("@plugin", "cachedoutput", "hashlib", "json", "os", "progress", "storage", "uuid", "workflow")

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

def get_task_iam_role():
    return os.environ.get("UF_TASK_IAM_ROLE", "")

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

def normalize_task_name(task_name, max_length = 45):
    """
    Normalize task name for Kubernetes resource naming conventions.
    
    This function:
    1. Replaces underscores with hyphens (Kubernetes prefers hyphens)
    2. Truncates the name if it exceeds max_length characters
    3. Ensures the result follows Kubernetes naming rules
    
    Args:
        task_name: the original task name
        max_length: maximum allowed length (default: 45 chars)
    Returns:
        normalized_task_name: the normalized task name suitable for Kubernetes resources
    
    Example:
        normalize_task_name("my_very_long_task_name_that_exceeds_limits") 
        -> "my-very-long-task-name-that-exceeds-limits"[:45]
    """
    if task_name == None:
        return ""
    
    # Replace underscores with hyphens for Kubernetes compatibility
    normalized = task_name.replace("_", "-")
    
    # Truncate if exceeds max length
    if len(normalized) > max_length:
        normalized = normalized[:max_length]
    
    return normalized

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

def report_progress(task_path, task_name, task_log = "", task_message = "", task_state = "", start_time = "", end_time = "", output = "", retry_attempt_id = ""):
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
