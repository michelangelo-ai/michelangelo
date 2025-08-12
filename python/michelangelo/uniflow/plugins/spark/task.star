load("@plugin", "atexit", "json", "os", "sparkhttp", "time", "workflow")
load("../../commons.star", "CACHE_OPERATION_GET", "CACHE_OPERATION_PUT", "DEFAULT_RETRY_ATTEMPTS", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "create_cached_output", "get_cache_enabled", "get_cache_keys", "get_cached_output", "get_result_url", "get_task_image", "get_task_iam_role", "get_task_architecture", "get_task_pipeline", "get_task_name", "io_read_json", "normalize_task_name", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

SPARK_ENV = {
}

SPARK_DEFAULT_DRIVER_CPU = os.environ.get("SPARK_DEFAULT_DRIVER_CPU", "4")
SPARK_DEFAULT_DRIVER_MEMORY = os.environ.get("SPARK_DEFAULT_DRIVER_MEMORY", "16G")
SPARK_DEFAULT_DRIVER_DISK = os.environ.get("SPARK_DEFAULT_DRIVER_DISK", "512G")
SPARK_DEFAULT_DRIVER_GPU = os.environ.get("SPARK_DEFAULT_DRIVER_GPU", "0")

SPARK_DEFAULT_EXECUTOR_CPU = os.environ.get("SPARK_DEFAULT_EXECUTOR_CPU", "4")
SPARK_DEFAULT_EXECUTOR_MEMORY = os.environ.get("SPARK_DEFAULT_EXECUTOR_MEMORY", "16G")
SPARK_DEFAULT_EXECUTOR_DISK = os.environ.get("SPARK_DEFAULT_EXECUTOR_DISK", "512G")
SPARK_DEFAULT_EXECUTOR_GPU = os.environ.get("SPARK_DEFAULT_EXECUTOR_GPU", "0")
SPARK_DEFAULT_EXECUTOR_INSTANCES = os.environ.get("SPARK_DEFAULT_EXECUTOR_INSTANCES", "1")

def spark_task(
        task_path,
        alias = None,
        cache_version = None,
        cache_enabled = False,
        retry_attempts = DEFAULT_RETRY_ATTEMPTS,
        driver_cpu = SPARK_DEFAULT_DRIVER_CPU,
        driver_memory = SPARK_DEFAULT_DRIVER_MEMORY,
        driver_disk = SPARK_DEFAULT_DRIVER_DISK,
        driver_gpu = SPARK_DEFAULT_DRIVER_GPU,
        executor_cpu = SPARK_DEFAULT_EXECUTOR_CPU,
        executor_memory = SPARK_DEFAULT_EXECUTOR_MEMORY,
        executor_disk = SPARK_DEFAULT_EXECUTOR_DISK,
        executor_gpu = SPARK_DEFAULT_EXECUTOR_GPU,
        executor_instances = SPARK_DEFAULT_EXECUTOR_INSTANCES):
    def callable(*args, **kwargs):
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get("MA_NAMESPACE", "default")
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
        final_cache_enabled = get_cache_enabled(cache_enabled, task_name)

        if final_cache_enabled:  # Check if the result is cached
            cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_GET)
            cached_output = get_cached_output(namespace, cache_keys)
            if cached_output != None:
                cached_result_json_url = cached_output.get("spec", {}).get("storageUri", "")
                if cached_result_json_url != "":
                    end_time_seconds = time.time()
                    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
                    report_progress(
                        task_name = task_name,
                        task_path = task_path,
                        task_state = TASK_STATE_SKIPPED,
                        start_time = start_time_formated_str,
                        task_message = "Spark Task skipped due to Cache Hit",
                        task_log = "",
                        end_time = end_time_formated_str,
                        output = cached_output.get("metadata", {}).get("name", ""),
                    )
                    result = io_read_json(cached_result_json_url)
                    print("spark | cached", "result:", result)
                    return result

        # Apply resource overrides
        _driver_cpu = os.environ.get("SPARK_OVERRIDE_DRIVER_CPU." + task_path, driver_cpu)
        _driver_memory = os.environ.get("SPARK_OVERRIDE_DRIVER_MEMORY." + task_path, driver_memory)
        _driver_disk = os.environ.get("SPARK_OVERRIDE_DRIVER_DISK." + task_path, driver_disk)
        _driver_gpu = os.environ.get("SPARK_OVERRIDE_DRIVER_GPU." + task_path, driver_gpu)

        _executor_cpu = os.environ.get("SPARK_OVERRIDE_EXECUTOR_CPU." + task_path, executor_cpu)
        _executor_memory = os.environ.get("SPARK_OVERRIDE_EXECUTOR_MEMORY." + task_path, executor_memory)
        _executor_disk = os.environ.get("SPARK_OVERRIDE_EXECUTOR_DISK." + task_path, executor_disk)
        _executor_gpu = os.environ.get("SPARK_OVERRIDE_EXECUTOR_GPU." + task_path, executor_gpu)
        _executor_instances = os.environ.get("SPARK_OVERRIDE_EXECUTOR_INSTANCES." + task_path, executor_instances)

        _retry_attempts = retry_attempts

        # Apply resource types
        _driver_cpu = int(_driver_cpu)
        _driver_gpu = int(_driver_gpu)
        _executor_cpu = int(_executor_cpu)
        _executor_gpu = int(_executor_gpu)
        _executor_instances = int(_executor_instances)

        result_url = get_result_url()
        _args = json.dumps(args) if args else "[]"
        _kwargs = "\'" + json.dumps(kwargs) + "\'" if kwargs else "'{}'"

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):

            job_state, terminated_job = execute_spark_task(
                namespace=namespace,
                task_name=task_name,
                task_path=task_path,
                start_time_formated_str=start_time_formated_str,
                retry_attempt_id=retry_attempt_id,
                total_retry_attempt=total_retry_attempt,
                result_url=result_url,
                args=_args,
                kwargs=_kwargs,
            )

            retryable = process_terminated_spark_job(
                job_state,
                terminated_job,
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
            )

            if retryable == False:
                break

        result = io_read_json(result_url)
        print("spark | caching", "result:", result)
        return result

    def with_overrides(alias = alias, config = spark_config(), retry_attempts = DEFAULT_RETRY_ATTEMPTS):
        return spark_task(
            task_path = task_path,
            alias = alias,
            cache_version = cache_version,
            cache_enabled = cache_enabled,
            retry_attempts = retry_attempts,
            driver_cpu = driver_cpu if "driver_cpu" not in config else config["driver_cpu"],
            driver_memory = driver_memory if "driver_memory" not in config else config["driver_memory"],
            driver_disk = driver_disk if "driver_disk" not in config else config["driver_disk"],
            driver_gpu = driver_gpu if "driver_gpu" not in config else config["driver_gpu"],
            executor_cpu = executor_cpu if "executor_cpu" not in config else config["executor_cpu"],
            executor_memory = executor_memory if "executor_memory" not in config else config["executor_memory"],
            executor_disk = executor_disk if "executor_disk" not in config else config["executor_disk"],
            executor_gpu = executor_gpu if "executor_gpu" not in config else config["executor_gpu"],
            executor_instances = executor_instances if "executor_instances" not in config else config["executor_instances"],
        )

    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable

def process_terminated_spark_job(job_state, terminated_job, task_name, task_path, args, kwargs, cache_version, namespace, result_url, start_time_formated_str, retry_attempt_id, total_retry_attempt):

    retryable = False

    if job_state == TASK_STATE_SUCCEEDED:

        cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_PUT)
        print("spark | caching with key", "key:", cache_keys)
        created_cached_output = create_cached_output(
            namespace = namespace,
            cache_keys = cache_keys,
            zone = "",
            ttl_in_days = 0,
            task_name = task_name,
            result_json_url = result_url,
        )
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)

        # driver_log_url = ""
        # if type(terminated_job) == "dict":
        #     driver_log_url = terminated_job.get("status", {}).get("jobUrl", "")

        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formated_str,
            # task_log = driver_log_url,
            end_time = end_time_formated_str,
            task_message = "Spark job succeeded",
            output = created_cached_output.get("metadata", {}).get("name", ""),
            retry_attempt_id = retry_attempt_id,
        )
        print("Spark job succeeded, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") succeeded")

    elif job_state == TASK_STATE_KILLED:
        print("Spark job killed, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + "). no retry should be performed")
        fail("Spark job killed, no retry should be performed")

    # If job failed or was killed, check if we have retries left
    elif job_state == TASK_STATE_FAILED:
        print("Spark job failed, attemp (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") failed")
        if retry_attempt_id < total_retry_attempt:
            retryable = True
        else:
            print("Spark job failed after all (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") attempts were exhausted")
            fail("Spark job failed after all attempts were exhausted")

    return retryable

def execute_spark_task(namespace, task_name, task_path, start_time_formated_str, retry_attempt_id, total_retry_attempt, result_url, args, kwargs):

    print("Spark job running, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ")")

    # driver_log_url = ""

    # task_path: uniflow.bert_cola.sparkone_example.uniflow_spark.spark_query
    uniflow_folder = task_path.split(".")[1]
    job_folder = task_path.split(".")[2]
    main_application_file = task_name + ".py"
    pipeline = get_task_pipeline()

    env = dict(COMMONS_ENV.items())
    env.update(SPARK_ENV)
    env.update(os.environ)
    env = " ".join([
        "{}='{}'".format(k, v)
        for k, v in env.items()
    ])

    # submit spark job
    print("spark | submit job. ns:", namespace, "task_name:", task_name)

    spark_one_spec = {
        "kind": "SparkOne",
        "apiVersion": "ml.chimera.kubebuilder.io/v1",
        "metadata": {
            "name": job_folder.replace("_", "-"),
        },
        "spec": {
            "pipeline": pipeline,
            "mainApplicationFile": "s3://chimera-mlpipeline/artifact/cauldron-test/svc.aip.chimeratest/pipelines/uniflow-poc/michelangelo/uniflow/core/run_task.py",
            "arguments": ["--task", task_path, "--args", args, "--kwargs", kwargs, "--result-url", result_url],
            "jobEnv": env,
            "uniflow": uniflow_folder,
        },
    }

    # Read user OIDC token from environment variable
    user_token = os.environ.get("UF_TASK_WORKSPACE_TOKEN", "")

    job = sparkhttp.create_job(spark_one_spec = spark_one_spec, user_token = user_token)

    # report task as pending
    report_progress(
        task_name = task_name,
        task_path = task_path,
        task_state = TASK_STATE_PENDING,
        start_time = start_time_formated_str,
        task_message = "Spark job has been submitted",
        # task_log = driver_log_url,
        retry_attempt_id = retry_attempt_id,
    )

    return report_spark_task_result(job, task_path, task_name, start_time_formated_str, retry_attempt_id), job

def report_spark_task_result(job, task_path, task_name, start_time_formated_str, retry_attempt_id):
    end_time_seconds = time.time()
    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
    if job["status"]["status"] == "SUCCEEDED":
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_message = "SparkOne Succeeded",
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_SUCCEEDED
    elif job["status"]["status"] == "KILLED":
        message = job.get("message", "unknown reason")
        task_message = "SparkOne killed with {}".format(message)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_message = task_message,
            task_state = TASK_STATE_KILLED,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_KILLED
    else:
        error_type = job.get("errorType", "internal")
        message = job.get("message", "unknown error")
        task_message = "SparkOne Failed with {} Error: {}".format(error_type, message)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_message = task_message,
            task_state = TASK_STATE_FAILED,
            start_time = start_time_formated_str,
            end_time = end_time_formated_str,
            retry_attempt_id = retry_attempt_id,
        )
        return TASK_STATE_FAILED

def spark_config(
        driver_cpu = None,
        driver_memory = None,
        driver_disk = None,
        driver_gpu = None,
        executor_cpu = None,
        executor_memory = None,
        executor_disk = None,
        executor_gpu = None,
        executor_instances = None):
    config_overrides = {
        "driver_cpu": driver_cpu,
        "driver_memory": driver_memory,
        "driver_disk": driver_disk,
        "driver_gpu": driver_gpu,
        "executor_cpu": executor_cpu,
        "executor_memory": executor_memory,
        "executor_disk": executor_disk,
        "executor_gpu": executor_gpu,
        "executor_instances": executor_instances,
    }
    return {key: value for key, value in config_overrides.items() if value != None}
