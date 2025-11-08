load("@plugin", "atexit", "json", "os", "spark", "time", "workflow")
load("../../commons.star", "DEFAULT_RETRY_ATTEMPTS", "CACHE_OPERATION_GET", "CACHE_OPERATION_PUT", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "create_cached_output", "get_cache_enabled", "get_cache_keys", "get_cached_output", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

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

        spark_job = get_spark_job(
            namespace = namespace,
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
        )

        total_retry_attempt = retry_attempts + 1
        for retry_attempt_id in range(1, total_retry_attempt + 1):

            job_state, terminated_job = execute_spark_task(
                namespace=namespace,
                task_name=task_name,
                task_path=task_path,
                spark_job=spark_job,
                start_time_formatted_str=start_time_formatted_str,
                retry_attempt_id=retry_attempt_id,
                total_retry_attempt=total_retry_attempt,
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
                start_time_formatted_str,
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

def process_terminated_spark_job(job_state, terminated_job, task_name, task_path, args, kwargs, cache_version, namespace, result_url, start_time_formatted_str, retry_attempt_id, total_retry_attempt):

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

        driver_log_url = ""
        if type(terminated_job) == "dict":
            driver_log_url = terminated_job.get("status", {}).get("jobUrl", "")

        report_progress(
            task_path = task_path,
            task_name = task_name,
            task_log = driver_log_url,
            task_message = "Spark job succeeded",
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formatted_str,
            end_time = end_time_formated_str,
            output = created_cached_output.get("metadata", {}).get("name", ""),
            retry_attempt_id = retry_attempt_id,
        )
        print("Spark job succeeded, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") succeeded")

    elif job_state == TASK_STATE_KILLED:
        print("Spark job killed, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + "). no retry should be performed")
        fail("Spark job killed, no retry should be performed")

    # If job failed or was killed, check if we have retries left
    elif job_state == TASK_STATE_FAILED:
        print("Spark job failed, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") failed")
        if retry_attempt_id < total_retry_attempt:
            retryable = True
        else:
            print("Spark job failed after all (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ") attempts were exhausted")
            fail("Spark job failed after all attempts were exhausted")

    return retryable

def execute_spark_task(namespace, task_name, task_path, spark_job, start_time_formatted_str, retry_attempt_id, total_retry_attempt):

    print("Spark job running, attempt (" + str(retry_attempt_id) + " / " + str(total_retry_attempt) + ")")
    
    driver_log_url = ""

    # submit spark job
    print("spark | submit job. ns:", namespace, "task_name:", task_name)
    created_spark_job = spark.create_job(spark_job)

    if type(created_spark_job) == "dict":
        driver_log_url = created_spark_job.get("status", {}).get("jobUrl", "")

    # report task as pending
    report_progress(
        task_path = task_path,
        task_name = task_name,
        task_log = driver_log_url,
        task_message = "Spark job has been submitted",
        task_state = TASK_STATE_PENDING,
        start_time = start_time_formatted_str,
        end_time = "",
        output = "",
        retry_attempt_id = retry_attempt_id,
    )

    # register the check_spark_job function to be called at the end of the workflow.
    # this is to ensure that the task state is reported correctly even if the workflow is killed
    atexit.register(check_spark_job_final_state_at_workflow_exit, created_spark_job, task_name, task_path, start_time_formatted_str, retry_attempt_id)

    # senosr spark job until it is running and driver log url is available
    running_job = spark.sensor_job(job = created_spark_job, assert_condition_type = spark.running_condition_type)
    print("spark | job running. ns:", namespace, "task_name:", task_name)

    if type(running_job) == "dict":
        driver_log_url = running_job.get("status", {}).get("jobUrl", "")

    report_progress(
        task_name = task_name,
        task_path = task_path,
        task_state = TASK_STATE_RUNNING,
        start_time = start_time_formatted_str,
        task_message = "Spark job is running",
        task_log = driver_log_url,
        retry_attempt_id = retry_attempt_id,
    )

    # sensor spark job until it is terminated
    print("spark | sensor job until terminated. ns:", namespace, "task_name:", task_name)
    terminated_job = spark.sensor_job(job = created_spark_job)
    print("spark | job terminated. ns:", namespace, "task_name:", task_name)
    job_state = report_spark_job_terminated(terminated_job, task_name, task_path, start_time_formatted_str, retry_attempt_id)
    if job_state == TASK_STATE_SUCCEEDED:
        atexit.unregister(check_spark_job_final_state_at_workflow_exit)

    return (job_state, terminated_job)

def check_spark_job_final_state_at_workflow_exit(created_spark_job, task_name, task_path, start_time_formatted_str, retry_attempt_id):
    """
    Check the final state of the spark job
    """
    final_job = spark.sensor_job(job = created_spark_job)
    report_spark_job_terminated(final_job, task_name, task_path, start_time_formatted_str, retry_attempt_id, unexpected_exit=True)
    return

def report_spark_job_terminated(job, task_name, task_path, start_time_formatted_str, retry_attempt_id, unexpected_exit=False):
    """
    Report task progress based on the succeeded condition of the spark job

    Args:
        job: the spark job crd
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
                   task_message = "{}: {}".format(killed_condition.get("message", "Spark job killed"), killed_condition.get("reason", "unknown reason")),
                   task_log = driver_log_url,
                   retry_attempt_id = retry_attempt_id,
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
                   task_message = "Spark job succeeded",
                   task_log = driver_log_url,
                   retry_attempt_id = retry_attempt_id,
               )
               return TASK_STATE_SUCCEEDED

           if succeeded_status == "CONDITION_STATUS_FALSE":
               message = succeeded_condition.get("message", "Spark job failed")
               reason = succeeded_condition.get("reason", "unknown reason")
               report_progress(
                   task_name = task_name,
                   task_path = task_path,
                   task_state = TASK_STATE_FAILED,
                   start_time = start_time_formatted_str,
                   end_time = end_time_formated_str,
                   task_message = "{}:{}".format(reason, message),
                   task_log = driver_log_url,
                   retry_attempt_id = retry_attempt_id,
               )
               if unexpected_exit == True:
                   fail("spark job failed: {} {} driver url: {}".format(reason, message, driver_log_url))
               return TASK_STATE_FAILED

       return ""

# TODO add env label for spark job
# Get the match labels for the spark job.
# The priority of the cluster/pool config is
#     default < project annotation < env
#
# Returns:
#   match_labels: the match labels for the spark job
#def get_spark_job_match_labels():
#    match_labels = {
#        env_label: "true",
#    }
#
#    return match_labels

def get_spark_job(
        namespace,
        image,
        main_file,
        main_class,
        main_args,
        driver_resource,
        executor_resource,
        executor_instances):
    env = dict(COMMONS_ENV.items())
    env.update(SPARK_ENV)
    env.update(os.environ)
    env = [
        {"name": k, "value": v}
        for k, v in env.items()
    ]

    #    match_labels = get_spark_job_match_labels()
    preemptible = True

    # TODO RESOURCE_ENV_LABEL_PROD
    #    if env_label == RESOURCE_ENV_LABEL_PROD:
    #        preemptible = False
    return {
        "kind": "SparkJob",
        "apiVersion": "michelangelo.api.v2",
        "metadata": {
            "namespace": namespace,
            "generateName": "uniflow-sp-",
        },
        "spec": {
            "user": {
                "name": "test",
            },
            #            "affinity": {
            #                "resourceAffinity": {
            #                    "selector": {
            #                        "matchLabels": match_labels,
            #                    },
            #                },
            #            },
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
                    "volumeMounts": [
                        {
                            "name": "spark-logs",
                            "mountPath": "/tmp/spark",
                        },
                    ],
                    "volumes": [
                        {
                            "name": "spark-logs",
                            "hostPath": {
                                "path": "/tmp/spark",
                                "type": "DirectoryOrCreate",
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
                    "volumeMounts": [
                        {
                            "name": "spark-logs",
                            "mountPath": "/tmp/spark",
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
                # Enable event logging for fluent-bit collection
                "spark.eventLog.enabled": "true",
                "spark.eventLog.dir": "/tmp/spark/eventlogs",
                "spark.eventLog.compress": "false",
                "spark.history.fs.logDirectory": "/tmp/spark/eventlogs",
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
