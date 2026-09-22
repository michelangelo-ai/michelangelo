load("@plugin", "os", "time")
load("../../commons.star", "DEFAULT_RETRY_ATTEMPTS", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "build_spark_crd_job", "execute_spark_crd_job", "get_pythonpath", "get_task_image", "get_task_name", "resource_dict", COMMONS_ENV = "ENV")

# scala_task runs a pre-compiled Scala/JVM Spark job (a JAR + main class) as a
# SparkJob CRD, the same submission mechanism spark_task uses (submission,
# sensing, and reporting is shared in commons.star's
# execute_spark_crd_job/build_spark_crd_job). Unlike spark_task, the JAR is
# not run_task.py + a Python function the driver calls back into - it is a
# self-contained program, so there is no --task/--args/--kwargs/--result-url
# contract and no result caching: success or failure is purely the SparkJob's
# terminal condition.

SCALA_ENV = {
    "PYTHONPATH": get_pythonpath(),
}

SCALA_DEFAULT_DRIVER_CPU = os.environ.get("SCALA_DEFAULT_DRIVER_CPU", "4")
SCALA_DEFAULT_DRIVER_MEMORY = os.environ.get("SCALA_DEFAULT_DRIVER_MEMORY", "16G")
SCALA_DEFAULT_DRIVER_DISK = os.environ.get("SCALA_DEFAULT_DRIVER_DISK", "512G")
SCALA_DEFAULT_DRIVER_GPU = os.environ.get("SCALA_DEFAULT_DRIVER_GPU", "0")

SCALA_DEFAULT_EXECUTOR_CPU = os.environ.get("SCALA_DEFAULT_EXECUTOR_CPU", "4")
SCALA_DEFAULT_EXECUTOR_MEMORY = os.environ.get("SCALA_DEFAULT_EXECUTOR_MEMORY", "16G")
SCALA_DEFAULT_EXECUTOR_DISK = os.environ.get("SCALA_DEFAULT_EXECUTOR_DISK", "512G")
SCALA_DEFAULT_EXECUTOR_GPU = os.environ.get("SCALA_DEFAULT_EXECUTOR_GPU", "0")
SCALA_DEFAULT_EXECUTOR_INSTANCES = os.environ.get("SCALA_DEFAULT_EXECUTOR_INSTANCES", "1")

SCALA_LOG_URL_PREFIX = os.environ.get("SCALA_LOG_URL_PREFIX")

def scala_task(
        task_path,
        main_file,
        main_class,
        alias = None,
        retry_attempts = DEFAULT_RETRY_ATTEMPTS,
        driver_cpu = SCALA_DEFAULT_DRIVER_CPU,
        driver_memory = SCALA_DEFAULT_DRIVER_MEMORY,
        driver_disk = SCALA_DEFAULT_DRIVER_DISK,
        driver_gpu = SCALA_DEFAULT_DRIVER_GPU,
        executor_cpu = SCALA_DEFAULT_EXECUTOR_CPU,
        executor_memory = SCALA_DEFAULT_EXECUTOR_MEMORY,
        executor_disk = SCALA_DEFAULT_EXECUTOR_DISK,
        executor_gpu = SCALA_DEFAULT_EXECUTOR_GPU,
        executor_instances = SCALA_DEFAULT_EXECUTOR_INSTANCES,
        cache_enabled = False,
        cache_version = None):
    def callable(*args, **kwargs):
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get("MA_NAMESPACE", "default")
        start_time_seconds = time.time()
        start_time_formatted_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)

        # Apply resource overrides
        _driver_cpu = os.environ.get("SCALA_OVERRIDE_DRIVER_CPU." + task_path, driver_cpu)
        _driver_memory = os.environ.get("SCALA_OVERRIDE_DRIVER_MEMORY." + task_path, driver_memory)
        _driver_disk = os.environ.get("SCALA_OVERRIDE_DRIVER_DISK." + task_path, driver_disk)
        _driver_gpu = os.environ.get("SCALA_OVERRIDE_DRIVER_GPU." + task_path, driver_gpu)

        _executor_cpu = os.environ.get("SCALA_OVERRIDE_EXECUTOR_CPU." + task_path, executor_cpu)
        _executor_memory = os.environ.get("SCALA_OVERRIDE_EXECUTOR_MEMORY." + task_path, executor_memory)
        _executor_disk = os.environ.get("SCALA_OVERRIDE_EXECUTOR_DISK." + task_path, executor_disk)
        _executor_gpu = os.environ.get("SCALA_OVERRIDE_EXECUTOR_GPU." + task_path, executor_gpu)
        _executor_instances = os.environ.get("SCALA_OVERRIDE_EXECUTOR_INSTANCES." + task_path, executor_instances)

        # Apply resource types
        _driver_cpu = int(_driver_cpu)
        _driver_gpu = int(_driver_gpu)
        _executor_cpu = int(_executor_cpu)
        _executor_gpu = int(_executor_gpu)
        _executor_instances = int(_executor_instances)

        env = dict(COMMONS_ENV.items())
        env.update(SCALA_ENV)
        env.update(os.environ)
        env = [
            {"name": k, "value": v}
            for k, v in env.items()
        ]

        scala_job = build_spark_crd_job(
            image = get_task_image(task_name),
            main_file = main_file,
            main_class = main_class,
            main_args = [],
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
            generate_name_prefix = "uniflow-sc-",
            env = env,
        )

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):
            job_state, terminated_job = execute_spark_crd_job(
                namespace = namespace,
                task_name = task_name,
                task_path = task_path,
                spark_crd_job = scala_job,
                start_time_formatted_str = start_time_formatted_str,
                retry_attempt_id = retry_attempt_id,
                total_retry_attempt = total_retry_attempt,
                job_label = "Scala",
                log_url_prefix = SCALA_LOG_URL_PREFIX,
            )

            retryable = process_scala_terminated_job(
                job_state = job_state,
                task_name = task_name,
                task_path = task_path,
                start_time_formatted_str = start_time_formatted_str,
                retry_attempt_id = retry_attempt_id,
                total_retry_attempt = total_retry_attempt,
            )

            if retryable == False:
                break

        return None

    def with_overrides(alias = alias, config = scala_config(), retry_attempts = DEFAULT_RETRY_ATTEMPTS):
        return scala_task(
            task_path = task_path,
            main_file = main_file if "main_file" not in config else config["main_file"],
            main_class = main_class if "main_class" not in config else config["main_class"],
            alias = alias,
            retry_attempts = retry_attempts,
            cache_enabled = cache_enabled,
            cache_version = cache_version,
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

def process_scala_terminated_job(job_state, task_name, task_path, start_time_formatted_str, retry_attempt_id, total_retry_attempt):
    """
    Decide whether a terminated scala job should be retried.

    Unlike spark_task's process_terminated_job, this does not write a
    CachedOutput on success - a JAR run has no result.json/args/kwargs
    contract to key a cache lookup on, so success/failure is reported via
    commons.star's report_spark_crd_job_terminated (already called by
    execute_spark_crd_job) and this function only carries the retry decision.

    Returns:
        retryable: Boolean indicating whether the job should be retried
    """
    retryable = False

    if job_state == TASK_STATE_SUCCEEDED:
        print("scala job succeeded, attempt ({} / {}) succeeded".format(str(retry_attempt_id), str(total_retry_attempt)))

    elif job_state == TASK_STATE_KILLED:
        print("scala job killed, attempt ({} / {}). no retry should be performed".format(str(retry_attempt_id), str(total_retry_attempt)))
        fail("scala job killed, no retry should be performed")

    elif job_state == TASK_STATE_FAILED:
        print("scala job failed, attempt ({} / {}) failed".format(str(retry_attempt_id), str(total_retry_attempt)))
        if retry_attempt_id < total_retry_attempt:
            retryable = True
        else:
            print("scala job failed after all ({} / {}) attempts were exhausted".format(str(retry_attempt_id), str(total_retry_attempt)))
            fail("scala job failed after all attempts were exhausted")

    return retryable

def scala_config(
        main_file = None,
        main_class = None,
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
        "main_file": main_file,
        "main_class": main_class,
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
