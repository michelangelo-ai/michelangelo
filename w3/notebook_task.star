# Notebook plugin - executes Jupyter notebooks via notebookhttp.
load("@plugin", "notebookhttp", "os", "time", "workflow")
load("../../commons.star", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "get_result_url", "get_task_name", "get_task_pipeline", "report_progress", COMMONS_ENV = "ENV")

# Default notebook configuration - all configurable via environment or parameters
NOTEBOOK_DEFAULT_INSTANCE_TYPE = os.environ.get("NOTEBOOK_DEFAULT_INSTANCE_TYPE", "large-ondemand")
NOTEBOOK_DEFAULT_KERNEL = os.environ.get("NOTEBOOK_DEFAULT_KERNEL", "python3")
NOTEBOOK_DEFAULT_IMAGE = os.environ.get("NOTEBOOK_DEFAULT_IMAGE", "default")

# S3 bucket per environment
NOTEBOOK_S3_BUCKET_STG = "cauldron-uibe-stg"
NOTEBOOK_S3_BUCKET_PRD = "cauldron-uibe-prd"

def task(
        notebook_path,
        alias = None,
        kernel = NOTEBOOK_DEFAULT_KERNEL,
        instance_type = NOTEBOOK_DEFAULT_INSTANCE_TYPE,
        image = NOTEBOOK_DEFAULT_IMAGE,
        git_ref = "HEAD",
        git_branch = None):

    def callable(*args, **kwargs):
        task_name = get_task_name(notebook_path, alias)
        start_time_seconds = time.time()
        start_time_formatted_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)

        result_url = get_result_url()

        # Convert args/kwargs to papermill parameters
        papermill_params = convert_args_to_papermill_params(args, kwargs)

        # Execute notebook task (Go activities handle pending/running progress)
        job_response = execute_notebook_task(
            notebook_path = notebook_path,
            task_name = task_name,
            kernel = kernel,
            instance_type = instance_type,
            image = image,
            git_ref = git_ref,
            git_branch = git_branch,
            papermill_params = papermill_params,
            result_url = result_url,
        )

        # Report success with deterministic time.time() (matches sparkhttp pattern)
        end_time_seconds = time.time()
        end_time_formatted_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        log_url = extract_notebook_log_url(job_response)
        report_progress(
            task_name = task_name,
            task_path = notebook_path,
            task_log = log_url,
            task_message = "Notebook job completed successfully",
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formatted_str,
            end_time = end_time_formatted_str,
        )

        print("notebook | job_response:", job_response)
        return job_response

    callable = callable_object(callable)
    return callable

def create_input_path_suffix(git_branch, git_ref):
    """
    Create input_path_suffix by combining git_branch + git_ref.
    - Empty branch & ref: "main@HEAD"
    - Branch only: "branch@HEAD"
    - Ref only: "main@ref8chars"
    - Branch + ref: "branch@ref8chars"
    """
    branch = git_branch.replace("/", "-") if git_branch else "main"
    ref = git_ref[:8] if git_ref else "HEAD"
    return branch + "@" + ref

def convert_args_to_papermill_params(args, kwargs):
    """
    Convert function arguments to papermill parameters.
    """
    params = {}
    # Add positional args as arg_0, arg_1, etc. (avoid dynamic key for Starlark parser)
    for i in range(len(args)):
        params = dict(params) + {"arg_%s" % i: args[i]}
    # Add keyword arguments
    if kwargs:
        params.update(kwargs)
    return params

def extract_notebook_log_url(job_response):
    """Extract S3 log URL from notebook job response (spec.executorArgs.s3Bucket + outputArtifactPath)."""
    spec = job_response.get("spec") if type(job_response) == "dict" else None
    if not spec:
        return ""
    executor_args = spec.get("executorArgs") if type(spec) == "dict" else None
    if not executor_args:
        return ""
    s3_bucket = executor_args.get("s3Bucket", "")
    output_path = executor_args.get("outputArtifactPath", "")
    if s3_bucket and output_path:
        return "s3://" + s3_bucket + "/" + output_path
    return ""

def execute_notebook_task(
        notebook_path,
        task_name,
        kernel,
        instance_type,
        image,
        git_ref,
        git_branch,
        papermill_params,
        result_url):

    environ = os.environ
    workspace = environ.get("UF_TASK_WORKSPACE", "")
    uf_env = environ.get("UF_TASK_WORKSPACE_ENV", "stg")
    user_email = environ.get("UF_TASK_USER_EMAIL", "")
    user_token = environ.get("UF_TASK_USER_TOKEN", "")
    workspace_env = "prd" if uf_env == "prd" else "stg"
    s3_bucket = NOTEBOOK_S3_BUCKET_PRD if workspace_env == "prd" else NOTEBOOK_S3_BUCKET_STG
    pipeline_id = get_task_pipeline()
    run_id = environ.get("MA_PIPELINE_RUN_NAME", "")

    # Add result_url to papermill params so notebook can save results
    papermill_params["result_url"] = result_url

    # Create input_path_suffix by combining git_branch + first 8 chars of git_ref
    input_path_suffix = create_input_path_suffix(git_branch, git_ref)

    print("notebook | create job:", "notebook_path=" + notebook_path, "input_path_suffix=" + input_path_suffix)

    # Execute notebook using notebookhttp plugin (Go activities handle all progress reporting)
    job_response = notebookhttp.create_job(
        user_token = user_token,
        s3_bucket = s3_bucket,
        workspace = workspace,
        workspace_env = workspace_env,
        pipeline_id = pipeline_id,
        task_id = task_name,
        run_id = run_id,
        notebook_input_path = notebook_path,  # Use notebook_path directly
        input_path_suffix = input_path_suffix,  # Use combined git_branch + first 8 chars of git_ref
        kernel = kernel,
        papermill_params = papermill_params,
        image = image,
        instance_type = instance_type,
        user_email = user_email,
        retry_attempt_id = 1,  # TODO: add starlark-level retry loop like spark/task.star to support configurable retries
    )

    # The Go notebookhttp activity handles job status, completion, and failure logic
    print("notebook | job completed:", job_response)
    return job_response