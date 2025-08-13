load('@plugin', 'atexit', 'json', 'os', 'ray', 'time')
load('/Users/baoquan/Documents/dev/oss/michelangelo/python/michelangelo/uniflow/commons.star', 'CACHE_OPERATION_GET', 'CACHE_OPERATION_PUT', 'TASK_STATE_FAILED', 'TASK_STATE_KILLED', 'TASK_STATE_PENDING', 'TASK_STATE_RUNNING', 'TASK_STATE_SKIPPED', 'TASK_STATE_SUCCEEDED', 'TIME_FOMART', 'create_cached_output', 'get_cache_enabled', 'get_cache_keys', 'get_cached_output', 'get_result_url', 'get_task_image', 'get_task_name', 'io_read_json', 'report_progress', 'resource_dict', COMMONS_ENV='ENV')
CREATE_CLUSTER_TIMEOUT_SECONDS = 60 * 30
RAY_ENV = {'RAY_DEDUP_LOGS': '0', 'RAY_NUM_REDIS_GET_RETRIES': str(CREATE_CLUSTER_TIMEOUT_SECONDS // 7 + 1)}
RAY_DEFAULT_HEAD_CPU = os.environ.get('RAY_DEFAULT_HEAD_CPU', '8')
RAY_DEFAULT_HEAD_MEMORY = os.environ.get('RAY_DEFAULT_HEAD_MEMORY', '32Gi')
RAY_DEFAULT_HEAD_DISK = os.environ.get('RAY_DEFAULT_HEAD_DISK', '512Gi')
RAY_DEFAULT_HEAD_GPU = os.environ.get('RAY_DEFAULT_HEAD_GPU', '0')
RAY_DEFAULT_WORKER_CPU = os.environ.get('RAY_DEFAULT_WORKER_CPU', '8')
RAY_DEFAULT_WORKER_MEMORY = os.environ.get('RAY_DEFAULT_WORKER_MEMORY', '32Gi')
RAY_DEFAULT_WORKER_DISK = os.environ.get('RAY_DEFAULT_WORKER_DISK', '512Gi')
RAY_DEFAULT_WORKER_GPU = os.environ.get('RAY_DEFAULT_WORKER_GPU', '0')
RAY_DEFAULT_WORKER_INSTANCES = os.environ.get('RAY_DEFAULT_WORKER_INSTANCES', '1')
RAY_DEFAULT_GPU_SKU = os.environ.get('RAY_DEFAULT_GPU_SKU', '')
RAY_DEFAULT_ZONE = os.environ.get('RAY_DEFAULT_ZONE', '')
USER_ID = os.environ.get('USER_ID', 'default_user')
IMAGE_PULL_POLICY = os.environ.get('IMAGE_PULL_POLICY', 'Never')

def task(task_path, alias=None, cache_version=None, cache_enabled=False, head_cpu=RAY_DEFAULT_HEAD_CPU, head_memory=RAY_DEFAULT_HEAD_MEMORY, head_disk=RAY_DEFAULT_HEAD_DISK, head_gpu=RAY_DEFAULT_HEAD_GPU, head_object_store_memory=None, worker_cpu=RAY_DEFAULT_WORKER_CPU, worker_memory=RAY_DEFAULT_WORKER_MEMORY, worker_disk=RAY_DEFAULT_WORKER_DISK, worker_gpu=RAY_DEFAULT_WORKER_GPU, worker_object_store_memory=None, worker_instances=RAY_DEFAULT_WORKER_INSTANCES, gpu_sku=RAY_DEFAULT_GPU_SKU, zone=RAY_DEFAULT_ZONE, breakpoint=False, runtime_env=None):

    def callable(*args, **kwargs):
        task_name = get_task_name(task_path, alias)
        namespace = os.environ.get('MA_NAMESPACE', 'default')
        start_time_seconds = time.time()
        start_time_formated_str = time.utc_format_seconds(TIME_FOMART, start_time_seconds)
        final_cache_enabled = get_cache_enabled(cache_enabled, task_name)
        if final_cache_enabled:
            cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_GET)
            print('ray | cache enabled with key', 'key:', cache_keys)
            cached_output = get_cached_output(namespace, cache_keys)
            if cached_output != None:
                cached_result_json_url = cached_output.get('spec', {}).get('storageUri', '')
                if cached_result_json_url != '':
                    print('ray | found cache output', 'cached_result_json_url:', cached_result_json_url)
                    end_time_seconds = time.time()
                    end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
                    report_progress(task_path=task_path, task_name=task_name, task_log='', task_message='Ray Task Skipped with Cache Hit', task_state=TASK_STATE_SKIPPED, start_time=start_time_formated_str, end_time=end_time_formated_str, output=cached_output.get('metadata', {}).get('name', ''))
                    result = io_read_json(cached_result_json_url)
                    print('ray | cached', 'result:', result)
                    return result
        _head_cpu = os.environ.get('RAY_OVERRIDE_HEAD_CPU.' + task_path, head_cpu)
        _head_memory = os.environ.get('RAY_OVERRIDE_HEAD_MEMORY.' + task_path, head_memory)
        _head_disk = os.environ.get('RAY_OVERRIDE_HEAD_DISK.' + task_path, head_disk)
        _head_gpu = os.environ.get('RAY_OVERRIDE_HEAD_GPU.' + task_path, head_gpu)
        _worker_cpu = os.environ.get('RAY_OVERRIDE_WORKER_CPU.' + task_path, worker_cpu)
        _worker_memory = os.environ.get('RAY_OVERRIDE_WORKER_MEMORY.' + task_path, worker_memory)
        _worker_disk = os.environ.get('RAY_OVERRIDE_WORKER_DISK.' + task_path, worker_disk)
        _worker_gpu = os.environ.get('RAY_OVERRIDE_WORKER_GPU.' + task_path, worker_gpu)
        _worker_instances = os.environ.get('RAY_OVERRIDE_WORKER_INSTANCES.' + task_path, worker_instances)
        _gpu_sku = os.environ.get('RAY_OVERRIDE_GPU_SKU.' + task_path, gpu_sku)
        _zone = os.environ.get('RAY_OVERRIDE_ZONE.' + task_path, zone)
        _head_cpu = int(_head_cpu)
        _head_gpu = int(_head_gpu)
        _worker_cpu = int(_worker_cpu)
        _worker_gpu = int(_worker_gpu)
        _worker_instances = int(_worker_instances)
        cluster_namespace = namespace
        cluster_image = get_task_image(task_name)
        print('ray | create cluster:', 'ns:', cluster_namespace, 'image:', cluster_image, 'task_name:', task_name)
        cluster = ray_cluster_spec(namespace=cluster_namespace, image=cluster_image, head_resource=resource_dict(cpu=_head_cpu, memory=_head_memory), worker_resource=resource_dict(cpu=_worker_cpu, memory=_worker_memory), worker_instances=_worker_instances, debug_enabled=breakpoint, runtime_env=runtime_env)
        cluster = ray.create_cluster(cluster)
        cluster_name = cluster['metadata']['name']
        cluster_namespace = cluster['metadata']['namespace']
        print('ray | cluster created:', 'ns=' + cluster_namespace, 'n=' + cluster_name)
        report_progress(task_path=task_path, task_name=task_name, task_message='Ray Cluster Created Successfully', task_state=TASK_STATE_RUNNING, start_time=start_time_formated_str, end_time='')

        def terminate_cluster():
            ray.terminate_cluster(cluster_name, cluster_namespace, 'job failed', 'TERMINATION_TYPE_FAILED')
            print('ray | cluster terminated:', 'ns=' + cluster_namespace, 'n=' + cluster_name)
        atexit.register(terminate_cluster)
        result_url = get_result_url()
        entrypoint = ray_job_entrypoint(task_path, result_url, args, kwargs)
        print('ray | run job:', 'task_path=' + task_path)
        job = ray.create_job(entrypoint, ray_job_namespace=cluster_namespace, ray_job_name=cluster_name)
        print('ray | +run job: job=' + str(job))

        def report_ray_task_result():
            end_time_seconds = time.time()
            end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
            if job['status']['state'] == 'RAY_JOB_STATE_SUCCEEDED':
                report_progress(task_path=task_path, task_name=task_name, task_message='Ray Job Succeeded', task_state=TASK_STATE_SUCCEEDED, start_time=start_time_formated_str, end_time=end_time_formated_str)
            elif job['status']['state'] == 'RAY_JOB_STATE_KILLED':
                message = job.get('message', 'unknown reason')
                task_message = 'Ray Job killed with {}'.format(message)
                report_progress(task_path=task_path, task_name=task_name, task_message=task_message, task_state=TASK_STATE_KILLED, start_time=start_time_formated_str, end_time=end_time_formated_str)
            else:
                error_type = job.get('errorType', 'internal')
                message = job.get('message', 'unknown error')
                task_message = 'Ray Job Failed with {} Error: {}'.format(error_type, message)
                report_progress(task_path=task_path, task_name=task_name, task_message=task_message, task_state=TASK_STATE_FAILED, start_time=start_time_formated_str, end_time=end_time_formated_str)
        atexit.register(report_ray_task_result)
        if breakpoint:
            print('ray | breakpoint:', 'ns=' + cluster_namespace, 'n=' + cluster_name)
            time.sleep(seconds=60 * 60 * 24)
            err_message = 'internal: breakpoint timeout'
            print('ray | error:', err_message)
            fail(err_message)
        if job['status']['state'] == 'RAY_JOB_STATE_SUCCEEDED':
            ray.terminate_cluster(cluster_name, cluster_namespace, 'job succeeded', 'TERMINATION_TYPE_SUCCEEDED')
        else:
            ray.terminate_cluster(cluster_name, cluster_namespace, 'job failed', 'TERMINATION_TYPE_FAILED')
        report_ray_task_result()
        atexit.unregister(terminate_cluster)
        atexit.unregister(report_ray_task_result)
        if job['status']['state'] != 'RAY_JOB_STATE_SUCCEEDED':
            fail('internal:', 'message:bad job status:', job['status']['state'], job)
        cache_keys = get_cache_keys(task_path, task_name, args, kwargs, cache_version, CACHE_OPERATION_PUT)
        created_cached_output = create_cached_output(namespace=namespace, cache_keys=cache_keys, zone='', ttl_in_days=0, task_name=task_name, result_json_url=result_url)
        cached_output_name = created_cached_output.get('metadata', {}).get('name', '')
        end_time_seconds = time.time()
        end_time_formated_str = time.utc_format_seconds(TIME_FOMART, end_time_seconds)
        report_progress(task_name=task_name, task_path=task_path, task_state=TASK_STATE_SUCCEEDED, start_time=start_time_formated_str, end_time=end_time_formated_str, task_message='Ray Task Completed Successfully')
        result = io_read_json(result_url)
        print('ray | caching', 'result:', result)
        return result

    def with_overrides(alias=alias, config=ray_config()):
        return task(task_path=task_path, alias=alias, cache_version=cache_version, cache_enabled=cache_enabled, head_cpu=head_cpu if 'head_cpu' not in config else config['head_cpu'], head_memory=head_memory if 'head_memory' not in config else config['head_memory'], head_disk=head_disk if 'head_disk' not in config else config['head_disk'], head_gpu=head_gpu if 'head_gpu' not in config else config['head_gpu'], head_object_store_memory=head_object_store_memory if 'head_object_store_memory' not in config else config['head_object_store_memory'], worker_cpu=worker_cpu if 'worker_cpu' not in config else config['worker_cpu'], worker_memory=worker_memory if 'worker_memory' not in config else config['worker_memory'], worker_disk=worker_disk if 'worker_disk' not in config else config['worker_disk'], worker_gpu=worker_gpu if 'worker_gpu' not in config else config['worker_gpu'], worker_object_store_memory=worker_object_store_memory if 'worker_object_store_memory' not in config else config['worker_object_store_memory'], worker_instances=worker_instances if 'worker_instances' not in config else config['worker_instances'], gpu_sku=gpu_sku if 'gpu_sku' not in config else config['gpu_sku'], zone=zone if 'zone' not in config else config['zone'], breakpoint=breakpoint if 'breakpoint' not in config else config['breakpoint'], runtime_env=runtime_env if 'runtime_env' not in config else config['runtime_env'])
    callable = callable_object(callable)
    callable.with_overrides = with_overrides
    return callable

def ray_job_entrypoint(task_path, result_url, args=None, kwargs=None):
    args = json.dumps(args) if args else '[]'
    kwargs = json.dumps(kwargs) if kwargs else '{}'
    return "python3 -m michelangelo.uniflow.core.run_task --task '" + task_path + "' --args '" + args + "' --kwargs '" + kwargs + "' --result-url '" + result_url + "'"

def ray_cluster_spec(namespace, image, head_resource, worker_resource, worker_instances, debug_enabled=False, runtime_env=None):
    ray_init_kwargs = os.environ.get('_RAY_INIT_KWARGS', {})
    ray_init_kwargs['runtime_env'] = runtime_env
    env = dict(COMMONS_ENV.items())
    env.update(RAY_ENV)
    env.update(os.environ)
    env.update({'_RAY_INIT_KWARGS': str(ray_init_kwargs)})
    env = [{'name': k, 'value': v} for (k, v) in env.items()]
    support_gpu = head_resource.get('gpu', 0) + worker_resource.get('gpu', 0) * worker_instances > 0
    annotations = {}
    if debug_enabled:
        annotations['michelangelo/profiling-ptrace-enabled'] = 'true'
    return {'metadata': {'generateName': 'uf-ray-', 'namespace': 'default', 'annotations': annotations}, 'spec': {'user': {'name': USER_ID}, 'rayVersion': '2.3.1', 'head': {'serviceType': 'ClusterIP', 'rayStartParams': {'block': 'true', 'dashboard-host': '0.0.0.0'}, 'pod': {'spec': {'containers': [{'name': 'head', 'resources': {'requests': head_resource}, 'image': image, 'imagePullPolicy': IMAGE_PULL_POLICY, 'env': env, 'envFrom': [{'configMapRef': {'localObjectReference': {'name': 'michelangelo-config'}}}], 'lifecycle': {'postStart': {'exec': {'command': ['/bin/sh', '-c', 'echo', "'Initializing Ray Head'"]}}}}]}}}, 'workers': [{'minInstances': worker_instances, 'maxInstances': worker_instances, 'nodeType': 'worker-group-1', 'objectStoreMemoryRatio': 0.0, 'rayStartParams': {'block': 'true', 'dashboard-host': '0.0.0.0'}, 'pod': {'spec': {'restartPolicy': 'Never', 'containers': [{'name': 'worker', 'resources': {'requests': worker_resource}, 'image': image, 'imagePullPolicy': IMAGE_PULL_POLICY, 'env': env, 'envFrom': [{'configMapRef': {'localObjectReference': {'name': 'michelangelo-config'}}}], 'lifecycle': {'postStart': {'exec': {'command': ['/bin/sh', '-c', 'echo', "'Initializing Ray Worker'"]}}}}]}}}], 'rayConf': {}}}

def ray_config(head_cpu=None, head_memory=None, head_disk=None, head_gpu=None, head_object_store_memory=None, worker_cpu=None, worker_memory=None, worker_disk=None, worker_gpu=None, worker_object_store_memory=None, worker_instances=None, breakpoint=None, runtime_env=None):
    config_overrides = {'head_cpu': head_cpu, 'head_memory': head_memory, 'head_disk': head_disk, 'head_gpu': head_gpu, 'head_object_store_memory': head_object_store_memory, 'worker_cpu': worker_cpu, 'worker_memory': worker_memory, 'worker_disk': worker_disk, 'worker_gpu': worker_gpu, 'worker_object_store_memory': worker_object_store_memory, 'worker_instances': worker_instances, 'breakpoint': breakpoint, 'runtime_env': runtime_env}
    return {key: value for (key, value) in config_overrides.items() if value != None}