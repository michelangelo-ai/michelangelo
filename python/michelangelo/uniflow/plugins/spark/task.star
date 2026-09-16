load("@plugin", "json", "os", "time", "workflow")
load("../../commons.star", "CACHE_OPERATION_GET", "CACHE_OPERATION_PUT", "DEFAULT_RETRY_ATTEMPTS", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "build_spark_crd_job", "execute_spark_crd_job", "get_cache_enabled", "get_cache_keys", "get_cached_output", "get_job_log_url", "get_pythonpath", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "process_terminated_job", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

SPARK_ENV = {
    "PYTHONPATH": get_pythonpath(),
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

SPARK_LOG_URL_PREFIX = os.environ.get("SPARK_LOG_URL_PREFIX")

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
        start_time_formatted_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
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
                        task_path = task_path,
                        task_name = task_name,
                        task_log = "",
                        task_message = "Spark Task skipped due to Cache Hit",
                        task_state = TASK_STATE_SKIPPED,
                        start_time = start_time_formatted_str,
                        end_time = end_time_formated_str,
                        output = cached_output.get("metadata", {}).get("name", ""),
                        retry_attempt_id = "",
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
        _kwargs = json.dumps(kwargs) if kwargs else "{}"

        env = dict(COMMONS_ENV.items())
        env.update(SPARK_ENV)
        env.update(os.environ)
        env = [
            {"name": k, "value": v}
            for k, v in env.items()
        ]

        spark_job = build_spark_crd_job(
            image = get_task_image(task_name),
            main_file = "local:///app/michelangelo/uniflow/core/run_task.py",
            main_class = "org.apache.spark.deploy.PythonRunner",
            # TODO: andrii: set --overrides
            main_args = ["--task", task_path, "--args", _args, "--kwargs", _kwargs, "--result-url", result_url],
            driver_resource = resource_dict(
                cpu = _driver_cpu,
                memory = _driver_memory,
                disk = _driver_disk,
                gpu = _driver_gpu,
            ),
            executor_resource = resource_dict(
                cpu = _executor_cpu,
                memory = _executor_memory,
                disk = _executor_disk,
                gpu = _executor_gpu,
            ),
            executor_instances = _executor_instances,
            generate_name_prefix = "uniflow-sp-",
            env = env,
        )

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):
            job_state, terminated_job = execute_spark_crd_job(
                namespace = namespace,
                task_name = task_name,
                task_path = task_path,
                spark_crd_job = spark_job,
                start_time_formatted_str = start_time_formatted_str,
                retry_attempt_id = retry_attempt_id,
                total_retry_attempt = total_retry_attempt,
                job_label = "Spark",
                log_url_prefix = SPARK_LOG_URL_PREFIX,
            )

            # Extract log URL from terminated job
            driver_log_url = ""
            spark_job_name = ""
            if type(terminated_job) == "dict":
                driver_log_url = terminated_job.get("status", {}).get("jobUrl", "")
                spark_job_name = terminated_job.get("metadata", {}).get("name", "")

            generated_log_url = get_job_log_url(SPARK_LOG_URL_PREFIX, spark_job_name)
            log_url = generated_log_url if generated_log_url else driver_log_url

            retryable = process_terminated_job(
                job_state = job_state,
                task_name = task_name,
                task_path = task_path,
                args = args,
                kwargs = kwargs,
                cache_version = cache_version,
                namespace = namespace,
                result_url = result_url,
                start_time_formatted_str = start_time_formatted_str,
                retry_attempt_id = retry_attempt_id,
                total_retry_attempt = total_retry_attempt,
                job_type = "Spark",
                log_url = log_url,
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
