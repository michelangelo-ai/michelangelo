load("@plugin", "atexit", "json", "os", "deployment", "time")
load("../../commons.star", "CACHE_OPERATION_GET", "CACHE_OPERATION_PUT", "TASK_STATE_FAILED", "TASK_STATE_KILLED", "TASK_STATE_PENDING", "TASK_STATE_RUNNING", "TASK_STATE_SKIPPED", "TASK_STATE_SUCCEEDED", "TIME_FOMART", "create_cached_output", "get_cache_enabled", "get_cache_keys", "get_cached_output", "get_result_url", "get_task_image", "get_task_name", "io_read_json", "report_progress", "resource_dict", COMMONS_ENV = "ENV")

USER_ID = os.environ.get("USER_ID", "default_user")

def task(
        task_path,
        alias = None,
        cache_version = None,
        cache_enabled = False,
        namespace = None,
        name = None,
        model_name = None):
    def callable(*args, **kwargs):
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get("MA_NAMESPACE", "default")
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
        final_cache_enabled = get_cache_enabled(cache_enabled, task_name)
        if final_cache_enabled:  # Check if the result is cached
            cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_GET)
            print("deploy | cache enabled with key", "key:", cache_keys)
            cached_output = get_cached_output(namespace, cache_keys)
            if cached_output != None:
                cached_result_json_url = cached_output.get("spec", {}).get("storageUri", "")
                if cached_result_json_url != "":
                    print("deploy | found cache output", "cached_result_json_url:", cached_result_json_url)
                    end_time_seconds = time.time()
                    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
                    report_progress(
                        task_path = task_path,
                        task_name = task_name,
                        task_log = "",
                        task_message = "Deploy Task Skipped with Cache Hit",
                        task_state = TASK_STATE_SKIPPED,
                        start_time = start_time_formated_str,
                        end_time = end_time_formated_str,
                        output = cached_output.get("metadata", {}).get("name", ""),
                    )
                    result = io_read_json(cached_result_json_url)
                    print("deploy | cached", "result:", result)
                    return result


        deployment.deploy(name=task_name, namespace=namespace, model_name=model_name)

    def with_overrides(alias = alias):
        return task(
            task_path,
            alias = None,
            cache_version = None,
            cache_enabled = False,
            namespace = None,
            name = None,
            model_name = None,
            )

    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable
