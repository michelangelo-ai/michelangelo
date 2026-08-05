load("@plugin", "atexit", "os", "spark", "time")
load("../../commons.star", "DEFAULT_RETRY_ATTEMPTS", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "get_pythonpath", "get_task_image", "get_task_name", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

# scala_task runs a pre-compiled Scala/JVM Spark job (a JAR + main class) as a
# SparkJob CRD, the same submission mechanism spark_task uses. Unlike
# spark_task, the JAR is not run_task.py + a Python function the driver calls
# back into - it is a self-contained program, so there is no
# --task/--args/--kwargs/--result-url contract and no result caching: success
# or failure is purely the SparkJob's terminal condition.

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

def get_scala_log_url(spark_job_name):
    """
    Generate a log URL for a Scala job's underlying SparkJob, based on the job name.
    Only generates a URL when SCALA_LOG_URL_PREFIX environment variable is provided.
    Expected format: {SCALA_LOG_URL_PREFIX}/{spark_job_name}.log

    Args:
        spark_job_name: The name of the SparkJob (e.g., "uniflow-sc-abc123")

    Returns:
        str: The complete log URL or empty string if prefix not configured
    """
    if SCALA_LOG_URL_PREFIX and spark_job_name:
        return "{}/{}.log".format(SCALA_LOG_URL_PREFIX, spark_job_name)
    return ""

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
        executor_instances = SCALA_DEFAULT_EXECUTOR_INSTANCES):
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

        scala_job = get_scala_spark_job(
            namespace = namespace,
            image = get_task_image(task_name),
            main_file = main_file,
            main_class = main_class,
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
        )

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):
            job_state, terminated_job = execute_scala_task(
                namespace = namespace,
                task_name = task_name,
                task_path = task_path,
                scala_job = scala_job,
                start_time_formatted_str = start_time_formatted_str,
                retry_attempt_id = retry_attempt_id,
                total_retry_attempt = total_retry_attempt,
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
            main_file = main_file,
            main_class = main_class,
            alias = alias,
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

def execute_scala_task(namespace, task_name, task_path, scala_job, start_time_formatted_str, retry_attempt_id, total_retry_attempt):
    print("Scala job running, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ")")

    driver_log_url = ""

    # submit spark job running the scala JAR
    print("scala | submit job. ns:", namespace, "task_name:", task_name)

    spark_job_response = spark.create_job(scala_job)

    created_spark_job = spark_job_response["sparkJob"]
    first_activity_id = spark_job_response["activityId"]

    print("scala | first activity ID:", first_activity_id)

    if created_spark_job == None:
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = "",
            task_message = "Scala Job Creation Failed",
            task_state = TASK_STATE_FAILED,
            start_time = start_time_formatted_str,
            end_time = end_time_formated_str,
            output = "",
            retry_attempt_id = retry_attempt_id,
            first_activity_id = first_activity_id,
        )
        fail("scala | job creation failed, activityId=" + first_activity_id)

    print("scala | job created:", "ns=" + namespace, "task_name=" + task_name)

    spark_job_name = ""
    if type(created_spark_job) == "dict":
        driver_log_url = created_spark_job.get("status", {}).get("jobUrl", "")
        spark_job_name = created_spark_job.get("metadata", {}).get("name", "")

    generated_log_url = get_scala_log_url(spark_job_name)

    report_progress(
        task_path = task_path,
        task_name = task_name,
        task_log = generated_log_url if generated_log_url else driver_log_url,
        task_message = "Scala job has been submitted",
        task_state = TASK_STATE_PENDING,
        start_time = start_time_formatted_str,
        end_time = "",
        output = "",
        retry_attempt_id = retry_attempt_id,
        first_activity_id = first_activity_id,
    )

    # register the check_scala_job_final_state_at_workflow_exit function to be called at the
    # end of the workflow. this is to ensure that the task state is reported correctly even
    # if the workflow is killed
    atexit.register(check_scala_job_final_state_at_workflow_exit, created_spark_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id)

    # sensor spark job until it is running and driver log url is available
    running_job = spark.sensor_job(job = created_spark_job, assert_condition_type = spark.running_condition_type)
    print("scala | job running. ns:", namespace, "task_name:", task_name)

    if type(running_job) == "dict":
        driver_log_url = running_job.get("status", {}).get("jobUrl", "")

    report_progress(
        task_name = task_name,
        task_path = task_path,
        task_state = TASK_STATE_RUNNING,
        start_time = start_time_formatted_str,
        task_message = "Scala job is running",
        task_log = generated_log_url if generated_log_url else driver_log_url,
        retry_attempt_id = retry_attempt_id,
        activity_id = first_activity_id,
    )

    # sensor spark job until it is terminated
    print("scala | sensor job until terminated. ns:", namespace, "task_name:", task_name)
    terminated_job = spark.sensor_job(job = created_spark_job)
    print("scala | job terminated. ns:", namespace, "task_name:", task_name)
    job_state = report_scala_job_terminated(terminated_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, spark_job_name = spark_job_name)
    if job_state == TASK_STATE_SUCCEEDED:
        atexit.unregister(check_scala_job_final_state_at_workflow_exit)

    return (job_state, terminated_job)

def check_scala_job_final_state_at_workflow_exit(created_spark_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id):
    """
    Check the final state of the scala job's underlying SparkJob.
    """
    final_job = spark.sensor_job(job = created_spark_job)
    report_scala_job_terminated(final_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, unexpected_exit = True)
    return

def report_scala_job_terminated(job, task_name, task_path, start_time_formatted_str, retry_attempt_id, first_activity_id, unexpected_exit = False, spark_job_name = ""):
    """
    Report task progress based on the succeeded condition of the underlying SparkJob.

    Args:
        job: the SparkJob crd
        task_name: the task name
        task_path: the task path
        start_time_formatted_str: the UTC formatted string of the task start time
        retry_attempt_id: the attempt id
        unexpected_exit: whether the job failed unexpectedly
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
    generated_log_url = get_scala_log_url(spark_job_name)
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
                task_message = "{}: {}".format(killed_condition.get("message", "Scala job killed"), killed_condition.get("reason", "unknown reason")),
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
                task_message = "Scala job succeeded",
                task_log = log_url,
                retry_attempt_id = retry_attempt_id,
                activity_id = first_activity_id,
            )
            return TASK_STATE_SUCCEEDED

        if succeeded_status == "CONDITION_STATUS_FALSE":
            message = succeeded_condition.get("message", "Scala job failed")
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
                fail("scala job failed: {} {} driver url: {}".format(reason, message, driver_log_url))
            return TASK_STATE_FAILED

    return ""

def process_scala_terminated_job(job_state, task_name, task_path, start_time_formatted_str, retry_attempt_id, total_retry_attempt):
    """
    Decide whether a terminated scala job should be retried.

    Unlike spark_task's process_terminated_job, this does not write a
    CachedOutput on success - a JAR run has no result.json/args/kwargs
    contract to key a cache lookup on, so success/failure is reported via
    report_scala_job_terminated (already called by execute_scala_task) and
    this function only carries the retry decision.

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

def get_scala_spark_job(
        namespace,
        image,
        main_file,
        main_class,
        driver_resource,
        executor_resource,
        executor_instances):
    env = dict(COMMONS_ENV.items())
    env.update(SCALA_ENV)
    env.update(os.environ)
    env = [
        {"name": k, "value": v}
        for k, v in env.items()
    ]

    preemptible = True

    return {
        "kind": "SparkJob",
        "apiVersion": "michelangelo.api.v2",
        "metadata": {
            "namespace": "default",
            "generateName": "uniflow-sc-",
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
            "mainArgs": [],
            "mainClass": main_class,
            "deps": {},
            "scheduling": {
                "preemptible": preemptible,
            },
            "sparkVersion": "3.5.5",
        },
    }

def scala_config(
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
