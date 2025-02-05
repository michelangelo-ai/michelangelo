load("@plugin", "cad", "hashlib", "json", "os", "progress", "uuid")

ENV = {
    "UF_HACK_MA_33362": "1",
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

RESOURCE_ENV_LABEL_PROD = "resourcepool.michelangelo/support-env-prod"
RESOURCE_ENV_LABEL_DEV = "resourcepool.michelangelo/support-env-dev"
RESOURCE_ENV_LABEL_TEST = "resourcepool.michelangelo/support-env-test"

RESOURCE_AFFINITY_LABEL_CLOUD_ZONE = "resourcepool.michelangelo/cloud-zone"
RESOURCE_AFFINITY_LABEL_ZONE = "resourcepool.michelangelo/zone"
RESOURCE_AFFINITY_LABEL_CLUSTER = "resourcepool.michelangelo/cluster"
RESOURCE_AFFINITY_LABEL_CLUSTER_TYPE = "resourcepool.michelangelo/cluster_type"
RESOURCE_AFFINITY_LABEL_NAME = "resourcepool.michelangelo/name"
RESOURCE_AFFINITY_LABEL_PATH = "resourcepool.michelangelo/path"
RESOURCE_AFFINITY_LABEL_SUPPORT_GPU = "resourcepool.michelangelo/support-resource-type-gpu"

def get_env_label():
    """
    Get the env label for the jobs.

    Returns:
        A string can be used for the env label in spark/ray jobs.
    """
    env = os.environ.get("ENV", "development").lower()
    if env == "production":
        return RESOURCE_ENV_LABEL_PROD
    elif env == "testing":
        return RESOURCE_ENV_LABEL_TEST
    return RESOURCE_ENV_LABEL_DEV

def get_result_url():
    """
    Get the url for the result.json
    """
    metadata_storage_url = os.environ.get("UF_METADATA_STORAGE_URL", os.environ["UF_STORAGE_URL"])
    result_url = "{}/{}.json".format(metadata_storage_url, uuid.uuid4().hex)
    return result_url

def io_read_json(file_path):
    if not os.exists(file_path):
        fail("400: file not found: " + file_path)

    f = os.open(file_path, "r")
    data = json.decode(f.read())
    f.close()

    return data

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

# Get the resource annotation for the project.
# Parameters:
#    project_namespace: str, the namespace of the project
#    project_name: str, the name of the project
# Returns:
#    resource_annotations: dict[str, str] the resource affinity for the project
def get_project_resource_annotation(project_namespace, project_name):
    response = cad.execute_activity(
        "code.uber.internal/uberai/michelangelo/starlark/activity/uapi.(*activities).GetProject",
        {"name": project_name, "namespace": project_namespace},
    )
    if type(response) != "dict" or response.get("project") == None:
        fail("failed to get project:", project_name)

    project = response["project"]
    return project.get("metadata", {}).get("annotations", {})

def resource_dict(cpu, memory):
    res = {
        "cpu": cpu,
        "memory": memory,
    }
    return res

def report_progress(task_path, task_name, task_log = "", task_message = "", task_state = "", start_time = "", end_time = "", output = ""):
    state_dict = {
        "task_path": task_path,
        "task_name": task_name,
        "task_log": task_log,
        "task_message": task_message,
        "task_state": task_state,
        "start_time": start_time,
        "end_time": end_time,
        "output": output,
    }
    progress.report(str(state_dict))

def get_task_path(task_path):
    """
    Get the task path for the task.

    Args:
        task_path: the path of the task
    Returns:
        final_task_path: the final path of the task.
    """
    return get_label_value(task_path)

#Get the cache version for the task.
#   Args:
#       cache_version: the version of the cache
#       task_name: the name of the task
#   Returns:
#       final_cache_version: the final version of the cache
def get_cache_version(cache_version, task_name):
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
#   Returns:
#      cache_keys: the cache keys
def get_cache_keys(task_path, task_name, args, kwargs, cache_version):
    final_task_path = get_task_path(task_path)
    final_cache_version = get_cache_version(cache_version, task_name)
    final_input_hash = get_input_hash(args, kwargs)

    cache_keys = {
        CACHE_KEY_TASK_PATH: final_task_path,
        CACHE_KEY_INPUT_HASH: final_input_hash,
        CACHE_KEY_CACHE_VERSION: final_cache_version,
    }
    return cache_keys

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

def get_storage_type(result_json_url):
    """
    Get the storage type for the task.

    Args:
        result_json_url: the dir url of the result.json
    Returns:
        storage_type: the storage type for CachedOutput
    """
    if result_json_url.startswith("file://"):
        storage_type = "STORAGE_TYPE_LOCAL"
    elif result_json_url.startswith("hdfs://"):
        storage_type = "STORAGE_TYPE_HDFS"
    elif result_json_url.startswith("cfs://"):
        storage_type = "STORAGE_TYPE_CFS"
    else:
        storage_type = "STORAGE_TYPE_INVALID"
    return storage_type

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
    return result_json_url
