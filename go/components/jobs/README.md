# Local Development (Mac)
The set of jobs controllers process requests by forwarding requests to Compute clusters. The kubeconfig for the Compute clusters is specified in the configuration. For local development, you can use your local kubernetes installation as a proxy for the Compute clusters. To do this, add the following to the `secrets.yaml` in project's `config` folder.
```
k8s-batch01-kubeconfig:
  k8s-batch01-admin-kubeconfig_dot_conf: <path to kubeconfig>
```

For example, this is the path to kubeconfig on the Mac laptop.
```
k8s-batch01-kubeconfig:
  k8s-batch01-admin-kubeconfig_dot_conf: /Users/bharatj/.kube/config
```

After that, you can run the service as usual.
```
JENKINS_OFFLINE_TOKEN="" JENKINS_PASSWORD="" JENKINS_USERNAME="" CONTROLLERS="ray" UBER_CONFIG_DIR=src/code.uber.internal/uberai/michelangelo/controllermgr/config bazel debug cmd/controllermgr:controllermgr
```

To simulate the creation workflow in Compute on local laptop, we change the namespace of the job request to `kuberay-operator` to avoid getting a `already exists` error from local K8s. This is the namespace in which the ray operator run by default. Therefore it is able to pick up crd request and spin up the corresponding pod.

There are few prerequisites to running the local setup:
1. Create the `RayCluster` CRD by running
```
kubectl create -f ray.io_rayclusters.yaml
```
2. Check that the CRD is installed
```
kubectl get crds

NAME                                           CREATED AT
rayclusters.ray.io                             2022-02-03T22:05:16Z
```
3. Install the `kuberay` operator by following the instructions at github using helm chart.

Once the ray operator pod is created, the operator logs can be inspected to see creation logs similar to the following.
```
kubectl logs kubray-operator-7bb9f747f8-wlh6h -n kuberay-operator
2022-01-28T02:21:45.149Z	INFO	setup	the operator	{"version:": ""}
I0128 02:21:46.523899       1 request.go:645] Throttling request took 1.1096326s, request: GET:https://10.96.0.1:443/apis/flowcontrol.apiserver.k8s.io/v1beta1?timeout=32s
2022-01-28T02:21:46.530Z	INFO	controller-runtime.metrics	metrics server is starting to listen	{"addr": ":8080"}
2022-01-28T02:21:46.530Z	INFO	setup	starting manager
2022-01-28T02:21:46.531Z	INFO	controller-runtime.manager	starting metrics server	{"path": "/metrics"}
2022-01-28T02:21:46.531Z	INFO	controller-runtime.manager.controller.raycluster-controller	Starting EventSource	{"reconciler group": "ray.io", "reconciler kind": "RayCluster", "source": "kind source: /, Kind="}
2022-01-28T02:21:47.012Z	INFO	controller-runtime.manager.controller.raycluster-controller	Starting EventSource	{"reconciler group": "ray.io", "reconciler kind": "RayCluster", "source": "kind source: /, Kind="}
2022-01-28T02:21:47.314Z	INFO	controller-runtime.manager.controller.raycluster-controller	Starting EventSource	{"reconciler group": "ray.io", "reconciler kind": "RayCluster", "source": "kind source: /, Kind="}
2022-01-28T02:21:47.415Z	INFO	controller-runtime.manager.controller.raycluster-controller	Starting Controller	{"reconciler group": "ray.io", "reconciler kind": "RayCluster"}
2022-01-28T02:21:47.415Z	INFO	controller-runtime.manager.controller.raycluster-controller	Starting workers	{"reconciler group": "ray.io", "reconciler kind": "RayCluster", "worker count": 1}
2022-01-28T21:07:16.309Z	INFO	raycluster-controller	reconciling RayCluster	{"cluster name": "rayjob-sample"}
2022-01-28T21:07:16.426Z	INFO	raycluster-controller	Pod Service created successfully	{"service name": "rayjob-sample-head-svc"}
2022-01-28T21:07:16.426Z	DEBUG	controller-runtime.manager.events	Normal	{"object": {"kind":"RayCluster","namespace":"kuberay-operator","name":"rayjob-sample","uid":"8a98eb0e-6621-429e-88e7-7b055628a3bd","apiVersion":"ray.io/v1alpha1","resourceVersion":"14094537"}, "reason": "Created", "message": "Created service rayjob-sample-head-svc"}
2022-01-28T21:07:16.507Z	INFO	raycluster-controller	reconcilePods 	{"creating head pod for cluster": "rayjob-sample"}
2022-01-28T21:07:16.507Z	INFO	RayCluster-Controller	Setting pod namespaces	{"namespace": "kuberay-operator"}
2022-01-28T21:07:16.507Z	INFO	raycluster-controller	createHeadPod	{"head pod with name": "rayjob-sample-head-"}
2022-01-28T21:07:16.519Z	DEBUG	controller-runtime.manager.events	Normal	{"object": {"kind":"RayCluster","namespace":"kuberay-operator","name":"rayjob-sample","uid":"8a98eb0e-6621-429e-88e7-7b055628a3bd","apiVersion":"ray.io/v1alpha1","resourceVersion":"14094537"}, "reason": "Created", "message": "Created head pod head"}
2022-01-28T21:07:16.620Z	INFO	raycluster-controller	reconciling RayCluster	{"cluster name": "rayjob-sample"}
2022-01-28T21:07:16.620Z	INFO	controllers.RayCluster	reconcileServices 	{"head service found": "rayjob-sample-head-svc"}
2022-01-28T21:07:16.621Z	INFO	raycluster-controller	reconcilePods 	{"head pod found": "head"}
2022-01-28T21:07:16.621Z	INFO	raycluster-controller	reconcilePods	{"head pod is up and running... checking workers": "head"}
2022-01-28T21:09:49.329Z	INFO	raycluster-controller	reconciling RayCluster	{"cluster name": "rayjob-sample"}
2022-01-28T21:09:49.329Z	INFO	controllers.RayCluster	reconcileServices 	{"head service found": "rayjob-sample-head-svc"}
2022-01-28T21:09:49.329Z	INFO	raycluster-controller	reconcilePods 	{"head pod found": "head"}
2022-01-28T21:09:49.329Z	INFO	raycluster-controller	reconcilePods	{"head pod is up and running... checking workers": "head"}
2022-01-28T21:09:49.341Z	INFO	raycluster-controller	reconciling RayCluster	{"cluster name": "rayjob-sample"}
2022-01-28T21:09:49.342Z	INFO	controllers.RayCluster	reconcileServices 	{"head service found": "rayjob-sample-head-svc"}
2022-01-28T21:09:49.343Z	INFO	raycluster-controller	reconcilePods 	{"head pod found": "head"}
2022-01-28T21:09:49.344Z	INFO	raycluster-controller	reconcilePods	{"head pod is up and running... checking workers": "head"}
```

You may also inspect the logs for the ray pod.
```
kubectl logs head -n kuberay-operator
--- Logging error ---
Traceback (most recent call last):
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/_private/utils.py", line 493, in get_k8s_cpus
    cpu_shares = int(open(cpu_share_file_name).read())
FileNotFoundError: [Errno 2] No such file or directory: '/sys/fs/cgroup/cpu/cpu.shares'

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/home/ray/anaconda3/lib/python3.7/logging/__init__.py", line 1025, in emit
    msg = self.format(record)
  File "/home/ray/anaconda3/lib/python3.7/logging/__init__.py", line 869, in format
    return fmt.format(record)
  File "/home/ray/anaconda3/lib/python3.7/logging/__init__.py", line 608, in format
    record.message = record.getMessage()
  File "/home/ray/anaconda3/lib/python3.7/logging/__init__.py", line 369, in getMessage
    msg = msg % self.args
TypeError: not all arguments converted during string formatting
Call stack:
  File "/home/ray/anaconda3/bin/ray", line 8, in <module>
    sys.exit(main())
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/scripts/scripts.py", line 1969, in main
    return cli()
  File "/home/ray/anaconda3/lib/python3.7/site-packages/click/core.py", line 1128, in __call__
    return self.main(*args, **kwargs)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/click/core.py", line 1053, in main
    rv = self.invoke(ctx)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/click/core.py", line 1659, in invoke
    return _process_result(sub_ctx.command.invoke(sub_ctx))
  File "/home/ray/anaconda3/lib/python3.7/site-packages/click/core.py", line 1395, in invoke
    return ctx.invoke(self.callback, **ctx.params)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/click/core.py", line 754, in invoke
    return __callback(*args, **kwargs)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/scripts/scripts.py", line 626, in start
    ray_params, head=True, shutdown_at_exit=block, spawn_reaper=block)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/node.py", line 248, in __init__
    self.start_head_processes()
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/node.py", line 905, in start_head_processes
    self.start_redis()
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/node.py", line 715, in start_redis
    self.get_resource_spec(),
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/node.py", line 376, in get_resource_spec
    is_head=self.head, node_ip_address=self.node_ip_address)
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/_private/resource_spec.py", line 136, in resolve
    num_cpus = ray._private.utils.get_num_cpus()
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/_private/utils.py", line 504, in get_num_cpus
    return int(math.ceil(get_k8s_cpus()))
  File "/home/ray/anaconda3/lib/python3.7/site-packages/ray/_private/utils.py", line 497, in get_k8s_cpus
    logger.exception("Error computing CPU limit of Ray Kubernetes pod.", e)
Message: 'Error computing CPU limit of Ray Kubernetes pod.'
Arguments: (FileNotFoundError(2, 'No such file or directory'),)
2022-01-28 13:09:51,036	INFO services.py:1272 -- View the Ray dashboard at http://127.0.0.1:8265
2022-01-28 13:09:49,429	INFO scripts.py:605 -- Local node IP: 10.1.94.146
2022-01-28 13:09:51,180	SUCC scripts.py:644 -- --------------------
2022-01-28 13:09:51,180	SUCC scripts.py:645 -- Ray runtime started.
2022-01-28 13:09:51,180	SUCC scripts.py:646 -- --------------------
2022-01-28 13:09:51,180	INFO scripts.py:648 -- Next steps
2022-01-28 13:09:51,180	INFO scripts.py:650 -- To connect to this Ray runtime from another node, run
2022-01-28 13:09:51,181	INFO scripts.py:656 --   ray start --address='10.1.94.146:6379' --redis-password='5241590000000000'
2022-01-28 13:09:51,181	INFO scripts.py:658 -- Alternatively, use the following Python code:
2022-01-28 13:09:51,181	INFO scripts.py:661 -- import ray
2022-01-28 13:09:51,181	INFO scripts.py:668 -- ray.init(address='auto', _redis_password='5241590000000000')
2022-01-28 13:09:51,181	INFO scripts.py:670 -- To connect to this Ray runtime from outside of the cluster, for example to
2022-01-28 13:09:51,181	INFO scripts.py:672 -- connect to a remote cluster from your laptop directly, use the following
2022-01-28 13:09:51,181	INFO scripts.py:674 -- Python code:
2022-01-28 13:09:51,181	INFO scripts.py:677 -- import ray
2022-01-28 13:09:51,181	INFO scripts.py:681 -- ray.init(address='ray://<head_node_ip_address>:10001')
2022-01-28 13:09:51,182	INFO scripts.py:685 -- If connection fails, check your firewall settings and network configuration.
2022-01-28 13:09:51,182	INFO scripts.py:689 -- To terminate the Ray runtime, run
2022-01-28 13:09:51,182	INFO scripts.py:690 --   ray stop

```

# Testing against Kube Batch test cluster
It is often helpful to test a scenario E2E against the Batch test cluster setup by the Compute team. This is because your local Kubernetes installation provided by Docker or other agents runs the open source version of the code. On the other hand, the Kubernetes version running on the Compute batch clusters is an Uber fork of the code. This differs in a few ways, most notably in the networking setup. It is also a good way to test issues that are hard to reproduce on your local setup. For example running everything locally could cause namespace or name collisions between global and zonal control planes. Perform the following steps to point to the test batch cluster:
1. Download the kubeconfig for the test cluster from Langley. This secret is managed under the `michelangelo-controllermgr` service. To get the secret, you can either go to the UI https://secrets.uberinternal.com/legacy-secrets?service=michelangelo-controllermgr OR exec into a container for this service using `compute-cli` and then view the secret.

   ```
   compute-cli -s ps --host dca11-yzp <get host for michelangelo-controllermgr in staging/prod from Up>
   compute-cli -s exec --host dca11-yzp --user root michelangelo-controllermgr.staging.compute-0/45f2b7b9-bbae-43fb-8891-7a0433e02063-1-755 <container listed from above command>

   [root@/home/udocker/michelangelo-controllermgr #]cat config/secrets.yaml
   ...
   k8s-batch01-kubeconfig:
   k8s-batch01-admin-kubeconfig_dot_conf: /langley/udocker/michelangelo-controllermgr/current/k8s-batch01-kubeconfig/k8s-batch01-admin-kubeconfig.conf
   k8s_hyphen_batch01_hyphen_admin_hyphen_kubeconfig_dot_conf: /langley/udocker/michelangelo-controllermgr/current/k8s-batch01-kubeconfig/k8s_hyphen_batch01_hyphen_admin_hyphen_kubeconfig.conf
   ...

   [root@/home/udocker/michelangelo-controllermgr #]cat k8s-batch01-admin-kubeconfig_dot_conf: /langley/udocker/michelangelo-controllermgr/current/k8s-batch01-kubeconfig/k8s-batch01-admin-kubeconfig.conf

   ```
   
2. Copy the secret in a local file. e.g. `compute-k8s-poc.conf`. Change the following in this file.
   ```
   server: https://127.0.0.1:16443 <any random port not in use>
   #server: https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal:6443
   ```
3. Setup ssh tunnels from the above random local port to the test batch cluster API server using the bastion host. Make sure this command succeeds.
   ```
   ssh -MfNL 16443:k8s-apiserver-kubernetes-batch01.phx5.uber.internal:6443 hadoopgw01-phx2
   ```
4. Now simply, use this `kubeconfig` in the service `secrets.yaml` file and debug your service.
   ```
   k8s-batch01-kubeconfig:
     k8s-batch01-admin-kubeconfig_dot_conf: /Users/bharatj/compute-k8s-poc.conf
   ```
5. To use with `kubectl`, set the following environment variable. Remember to save the previous value of this variable to revert to your local cluster.
   ```
   export KUBECONFIG=/Users/bharatj/compute-k8s-poc.conf
   ```
6. Now you can run commands against the test batch cluster.
   ```
   ➜  jobs kubectl get nodes
   NAME       STATUS   ROLES    AGE    VERSION
   phx5-tcr   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-tp5   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-v72   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-v9k   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-wka   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-x3a   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-x96   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-xpk   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-yr6   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   phx5-yrr   Ready    <none>   111d   v1.20.4-118+040121ae9af925-dirty
   ```
   