load("@plugin", "atexit", "cad", "json", "os", "spark", "time")
load("../../commons.star", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

SPARK_ENV = {
    "PYTHONPATH": ".",
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

        # TODO check cache enabled

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
            main_file = "local:///home/udocker/app/uber/ai/uniflow/core/run_task.py",
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

        # submit spark job
        print("spark | submit job. ns:", namespace, "task_name:", task_name)
        created_spark_job = spark.create_job(spark_job)

        # report task as pending
        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_PENDING,
            start_time = start_time_formated_str,
            task_message = "Spark job has been submitted",
            task_log = created_spark_job.get("status", {}).get("jobUrl", ""),
            end_time = "",
        )

        def check_spark_job_final_state_at_workflow_exit():
            """
            Check the final state of the spark job
            """
            final_job = spark.sensor_job(job = created_spark_job)
            report_spark_job_terminated(final_job, task_name, task_path, start_time_formated_str)
            return

        # register the check_spark_job function to be called at the end of the workflow.
        # this is to ensure that the task state is reported correctly even if the workflow is killed
        atexit.register(check_spark_job_final_state_at_workflow_exit)

        # senosr spark job until it is running and driver log url is available
        running_job = spark.sensor_job(job = created_spark_job, assert_condition_type = spark.running_condition_type)

        driver_log_url = ""
        if type(running_job) == "dict":
            driver_log_url = running_job.get("status", {}).get("jobUrl", "")
        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_RUNNING,
            start_time = start_time_formated_str,
            task_message = "Spark job is running",
            task_log = driver_log_url,
            end_time = "",
            output = "",
        )

        # senosr spark job until it is terminated
        terminated_job = spark.sensor_job(job = created_spark_job)
        if report_spark_job_terminated(terminated_job, task_name, task_path, start_time_formated_str) == True:
            atexit.unregister(check_spark_job_final_state_at_workflow_exit)

        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(
            task_name = task_name,
            task_path = task_path,
            task_state = TASK_STATE_SUCCEEDED,
            start_time = start_time_formated_str,
            task_log = driver_log_url,
            end_time = end_time_formated_str,
            task_message = "Spark job succeeded",
        )
        return io_read_json(result_url)

    def with_overrides(alias = alias, config = spark_config()):
        return spark_task(
            task_path = task_path,
            alias = alias,
            cache_version = cache_version,
            cache_enabled = cache_enabled,
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

def report_spark_job_terminated(job, task_name, task_path, start_time_formated_str):
    """
    Report task progress based on the succeeded condition of the spark job

    Args:
        job: the spark job crd
        task_name: the task name
        task_path: the task path
        start_time_formated_str: the UTC formated string of the task start time
    Returns:
        True if the task is done, False otherwise
    """

    if type(job) != "dict":
        return False

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
                start_time = start_time_formated_str,
                end_time = end_time_formated_str,
                task_message = "{}: {}".format(killed_condition.get("message", "Spark job killed"), killed_condition.get("reason", "unknown reason")),
                task_log = driver_log_url,
            )
            return True

    if succeeded_condition != None:
        succeeded_status = succeeded_condition.get("status", "CONDITION_STATUS_UNKNOWN")
        if succeeded_status == "CONDITION_STATUS_TRUE":
            report_progress(
                task_name = task_name,
                task_path = task_path,
                task_state = TASK_STATE_SUCCEEDED,
                start_time = start_time_formated_str,
                end_time = end_time_formated_str,
                task_message = "Spark job succeeded",
                task_log = driver_log_url,
            )
            return True

        if succeeded_status == "CONDITION_STATUS_FALSE":
            message = succeeded_condition.get("message", "Spark job failed")
            reason = succeeded_condition.get("reason", "unknown reason")
            report_progress(
                task_name = task_name,
                task_path = task_path,
                task_state = TASK_STATE_FAILED,
                start_time = start_time_formated_str,
                end_time = end_time_formated_str,
                task_message = "{}:{}".format(reason, message),
                task_log = driver_log_url,
            )
            fail("spark job failed: {} {} driver url: {}".format(reason, message, driver_log_url))

    return False

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
                },
            },
            "executor": {
                "pod": {
                    "resource": executor_resource,
                    "image": image,
                    "imagePullingPolicy": "Never",
                    "env": env,
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
