load("@plugin", "atexit", "json", "os", "ray", "time")

RAY_DEFAULT_HEAD_CPU = os.environ.get("RAY_DEFAULT_HEAD_CPU", "8")
RAY_DEFAULT_HEAD_MEMORY = os.environ.get("RAY_DEFAULT_HEAD_MEMORY", "32Gi")
RAY_DEFAULT_HEAD_DISK = os.environ.get("RAY_DEFAULT_HEAD_DISK", "512Gi")
RAY_DEFAULT_HEAD_GPU = os.environ.get("RAY_DEFAULT_HEAD_GPU", "0")

RAY_DEFAULT_WORKER_CPU = os.environ.get("RAY_DEFAULT_WORKER_CPU", "8")
RAY_DEFAULT_WORKER_MEMORY = os.environ.get("RAY_DEFAULT_WORKER_MEMORY", "32Gi")
RAY_DEFAULT_WORKER_DISK = os.environ.get("RAY_DEFAULT_WORKER_DISK", "512Gi")
RAY_DEFAULT_WORKER_GPU = os.environ.get("RAY_DEFAULT_WORKER_GPU", "0")
RAY_DEFAULT_WORKER_INSTANCES = os.environ.get("RAY_DEFAULT_WORKER_INSTANCES", "1")

def task(
        task_path,
        alias = None,
        cache_version = None,
        cache_enabled = False,
        head_cpu = RAY_DEFAULT_HEAD_CPU,
        head_memory = RAY_DEFAULT_HEAD_MEMORY,
        head_disk = RAY_DEFAULT_HEAD_DISK,
        head_gpu = RAY_DEFAULT_HEAD_GPU,
        head_object_store_memory = None,
        worker_cpu = RAY_DEFAULT_WORKER_CPU,
        worker_memory = RAY_DEFAULT_WORKER_MEMORY,
        worker_disk = RAY_DEFAULT_WORKER_DISK,
        worker_gpu = RAY_DEFAULT_WORKER_GPU,
        worker_object_store_memory = None,
        worker_instances = RAY_DEFAULT_WORKER_INSTANCES,
        breakpoint = False):
    def callable(*args, **kwargs):
        # TODO: andrii: Implement Ray task orchestration
        return ...

    def with_overrides(alias = alias):
        return ray_task(
            task_path = task_path,
            alias = alias,
            cache_version = cache_version,
            cache_enabled = cache_enabled,
            head_cpu = head_cpu,
            head_memory = head_memory,
            head_disk = head_disk,
            head_gpu = head_gpu,
            head_object_store_memory = head_object_store_memory,
            worker_cpu = worker_cpu,
            worker_memory = worker_memory,
            worker_disk = worker_disk,
            worker_gpu = worker_gpu,
            worker_object_store_memory = worker_object_store_memory,
            worker_instances = worker_instances,
            breakpoint = breakpoint,
        )

    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable
