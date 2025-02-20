load("@plugin", "cad", "hashlib", "json", "os", "progress", "s3", "uuid")

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

LABEL_VALUE_SIZE_LIMIT = 60

def get_result_url():
    """
    Get the url for the result.json
    """
    metadata_storage_url = os.environ.get("UF_METADATA_STORAGE_URL", os.environ["UF_STORAGE_URL"])
    result_url = "{}/{}.json".format(metadata_storage_url, uuid.uuid4().hex)
    return result_url

def s3_read_json(path):
    # the format of the path is <bucket>/<file_path>
    parts = path.split("/")
    if len(parts) != 2:
        fail("400: unsupported path:", path)
    b = s3.read(parts[0], parts[1])
    return b

def io_read_json(url):
    readers = {
        "s3://": s3_read_json,
    }
    for prefix, reader in readers.items():
        if url.startswith(prefix):
            path = url[len(prefix):]
            return reader(path)

    fail("400: unsupported url:", url)

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
