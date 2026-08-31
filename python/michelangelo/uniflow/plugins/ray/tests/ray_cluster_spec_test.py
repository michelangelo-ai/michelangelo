"""Tests for the Ray cluster Starlark specification builder."""

from __future__ import annotations

import ast
from pathlib import Path
from types import SimpleNamespace
from unittest import TestCase


def _load_star_functions(*names: str, extra_globals: dict | None = None):
    """Load the named task.star functions with controlled globals."""
    task_path = Path(__file__).resolve().parents[1] / "task.star"
    tree = ast.parse(task_path.read_text(), filename=str(task_path))
    functions = [
        node
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name in names
    ]
    assert len(functions) == len(names), f"missing one of {names} in task.star"
    module = ast.fix_missing_locations(ast.Module(body=functions, type_ignores=[]))
    globals_ = {
        "COMMONS_ENV": {},
        "IMAGE_PULL_POLICY": "Never",
        "RAY_ENV": {},
        "USER_ID": "test-user",
        "os": SimpleNamespace(environ={}),
    }
    if extra_globals:
        globals_.update(extra_globals)
    exec(compile(module, str(task_path), "exec"), globals_)
    return tuple(globals_[name] for name in names)


class TestContainerResources(TestCase):
    """Tests for container_resources()."""

    def setUp(self):
        """Load container_resources from task.star."""
        (self.container_resources,) = _load_star_functions("container_resources")

    def test_cpu_memory_only(self):
        """Without disk or gpu the dict has requests only."""
        resources = self.container_resources(cpu=2, memory="4Gi")

        self.assertEqual(resources, {"requests": {"cpu": 2, "memory": "4Gi"}})

    def test_disk_maps_to_ephemeral_storage(self):
        """Disk lands under the real k8s resource name, not diskSize."""
        resources = self.container_resources(cpu=2, memory="4Gi", disk="100Gi")

        self.assertEqual(resources["requests"]["ephemeral-storage"], "100Gi")
        self.assertNotIn("limits", resources)

    def test_gpu_sets_request_and_limit(self):
        """GPU is an extended resource, so request and limit must match."""
        resources = self.container_resources(cpu=2, memory="4Gi", gpu=1)

        self.assertEqual(resources["requests"]["nvidia.com/gpu"], 1)
        self.assertEqual(resources["limits"], {"nvidia.com/gpu": 1})

    def test_gpu_zero_adds_nothing(self):
        """gpu=0 (the default resolved value) must not emit gpu keys."""
        resources = self.container_resources(cpu=2, memory="4Gi", gpu=0)

        self.assertNotIn("nvidia.com/gpu", resources["requests"])
        self.assertNotIn("limits", resources)


class TestRayClusterSpec(TestCase):
    """Tests for ray_cluster_spec()."""

    def _build_spec(self, namespace: str = "default", **kwargs):
        (ray_cluster_spec,) = _load_star_functions("ray_cluster_spec")
        return ray_cluster_spec(
            namespace=namespace,
            image="test-image",
            head_resources=kwargs.pop(
                "head_resources", {"requests": {"cpu": 1, "memory": "1Gi"}}
            ),
            worker_resources=kwargs.pop(
                "worker_resources", {"requests": {"cpu": 1, "memory": "1Gi"}}
            ),
            worker_instances=kwargs.pop("worker_instances", 1),
            **kwargs,
        )

    def _head_container(self, spec):
        return spec["spec"]["head"]["pod"]["spec"]["containers"][0]

    def _worker_container(self, spec):
        return spec["spec"]["workers"][0]["pod"]["spec"]["containers"][0]

    def test_preserves_custom_namespace(self):
        """A project namespace is retained in cluster metadata."""
        spec = self._build_spec("ma-dev-test")

        self.assertEqual(spec["metadata"]["namespace"], "ma-dev-test")

    def test_preserves_default_namespace(self):
        """Default-namespace workflows remain unchanged."""
        spec = self._build_spec("default")

        self.assertEqual(spec["metadata"]["namespace"], "default")

    def test_resources_passed_through_verbatim(self):
        """The ResourceRequirements dicts land on the containers unchanged."""
        head = {
            "requests": {"cpu": 4, "memory": "8Gi", "nvidia.com/gpu": 2},
            "limits": {"nvidia.com/gpu": 2},
        }
        worker = {"requests": {"cpu": 2, "memory": "4Gi", "ephemeral-storage": "100Gi"}}
        spec = self._build_spec(head_resources=head, worker_resources=worker)

        self.assertEqual(self._head_container(spec)["resources"], head)
        self.assertEqual(self._worker_container(spec)["resources"], worker)

    def test_object_store_memory_reaches_ray_start_params(self):
        """Object store bytes are forwarded verbatim as a ray start flag."""
        spec = self._build_spec(
            head_object_store_memory=2_000_000_000,
            worker_object_store_memory=1_000_000_000,
        )

        self.assertEqual(
            spec["spec"]["head"]["rayStartParams"]["object-store-memory"],
            "2000000000",
        )
        self.assertEqual(
            spec["spec"]["workers"][0]["rayStartParams"]["object-store-memory"],
            "1000000000",
        )

    def test_object_store_memory_absent_by_default(self):
        """Without the parameter, rayStartParams stay exactly as before."""
        spec = self._build_spec()

        expected = {"block": "true", "dashboard-host": "0.0.0.0"}
        self.assertEqual(spec["spec"]["head"]["rayStartParams"], expected)
        self.assertEqual(spec["spec"]["workers"][0]["rayStartParams"], expected)


class TestTaskResourcePlumbing(TestCase):
    """End-to-end tests that task() forwards resources into the cluster spec.

    Runs the real task() from task.star with the plugin runtime stubbed out,
    capturing the cluster spec handed to execute_ray_task. This is the exact
    regression surface of the silently-dropped gpu/disk/object-store settings.
    """

    # The shipped default: empty string means "no explicit disk request".
    _DEFAULT_DISK = ""

    def _run_task(self, environ: dict | None = None, **task_kwargs):
        captured = {}

        def execute_ray_task(**kwargs):
            captured.update(kwargs)
            return "SUCCEEDED", None, "http://cluster", "ray-job-1"

        stubs = {
            "DEFAULT_RETRY_ATTEMPTS": 1,
            "RAY_DEFAULT_HEAD_CPU": "8",
            "RAY_DEFAULT_HEAD_MEMORY": "32Gi",
            "RAY_DEFAULT_HEAD_DISK": self._DEFAULT_DISK,
            "RAY_DEFAULT_HEAD_GPU": "0",
            "RAY_DEFAULT_WORKER_CPU": "8",
            "RAY_DEFAULT_WORKER_MEMORY": "32Gi",
            "RAY_DEFAULT_WORKER_DISK": self._DEFAULT_DISK,
            "RAY_DEFAULT_WORKER_GPU": "0",
            "RAY_DEFAULT_WORKER_INSTANCES": "1",
            "RAY_DEFAULT_GPU_SKU": "",
            "RAY_DEFAULT_ZONE": "",
            "RAY_LOG_URL_PREFIX": None,
            "TIME_FOMART": "%Y-%m-%dT%H:%M:%S",
            "TASK_STATE_SKIPPED": "SKIPPED",
            "CACHE_OPERATION_GET": "GET",
            "os": SimpleNamespace(environ=dict(environ or {})),
            "time": SimpleNamespace(
                time=lambda: 0.0,
                utc_format_seconds=lambda fmt, seconds: "2026-01-01T00:00:00",
            ),
            "get_task_name": lambda task_path, alias: "test-task",
            "get_cache_enabled": lambda cache_enabled, task_name: False,
            "get_result_url": lambda: "s3://bucket/result.json",
            "get_task_image": lambda task_name: "test-image",
            "execute_ray_task": execute_ray_task,
            "process_terminated_job": lambda **kwargs: False,
            "io_read_json": lambda url: {"ok": True},
            "report_progress": lambda **kwargs: None,
            "callable_object": lambda func: func,
        }
        task, _, _, _, _ = _load_star_functions(
            "task",
            "ray_cluster_spec",
            "container_resources",
            "ray_config",
            "get_ray_log_url",
            extra_globals=stubs,
        )
        result = task(task_path="examples.demo.train", **task_kwargs)()
        self.assertEqual(result, {"ok": True})
        return captured["cluster"]

    def _containers(self, cluster):
        head = cluster["spec"]["head"]["pod"]["spec"]["containers"][0]
        worker = cluster["spec"]["workers"][0]["pod"]["spec"]["containers"][0]
        return head, worker

    def test_defaults_produce_cpu_memory_requests_only(self):
        """With no resource parameters the spec matches the old behavior."""
        cluster = self._run_task()

        head, worker = self._containers(cluster)
        self.assertEqual(head["resources"], {"requests": {"cpu": 8, "memory": "32Gi"}})
        self.assertEqual(
            worker["resources"], {"requests": {"cpu": 8, "memory": "32Gi"}}
        )
        self.assertNotIn(
            "object-store-memory", cluster["spec"]["head"]["rayStartParams"]
        )

    def test_explicit_resources_reach_the_pods(self):
        """gpu, disk and object store memory land where k8s and Ray read them."""
        cluster = self._run_task(
            head_gpu="1",
            head_disk="100Gi",
            head_object_store_memory=2000000000,
            worker_gpu="2",
            worker_disk="200Gi",
            worker_object_store_memory=1000000000,
        )

        head, worker = self._containers(cluster)
        self.assertEqual(head["resources"]["requests"]["nvidia.com/gpu"], 1)
        self.assertEqual(head["resources"]["limits"], {"nvidia.com/gpu": 1})
        self.assertEqual(head["resources"]["requests"]["ephemeral-storage"], "100Gi")
        self.assertEqual(worker["resources"]["requests"]["nvidia.com/gpu"], 2)
        self.assertEqual(worker["resources"]["limits"], {"nvidia.com/gpu": 2})
        self.assertEqual(worker["resources"]["requests"]["ephemeral-storage"], "200Gi")
        self.assertEqual(
            cluster["spec"]["head"]["rayStartParams"]["object-store-memory"],
            "2000000000",
        )
        self.assertEqual(
            cluster["spec"]["workers"][0]["rayStartParams"]["object-store-memory"],
            "1000000000",
        )

    def test_no_disk_parameter_adds_no_request(self):
        """Without a disk parameter no ephemeral-storage request renders."""
        cluster = self._run_task()

        head, workers = self._containers(cluster)
        self.assertNotIn("ephemeral-storage", head["resources"]["requests"])
        self.assertNotIn("ephemeral-storage", workers["resources"]["requests"])

    def test_explicit_512gi_disk_forwards(self):
        """A user asking for exactly 512Gi gets it.

        Regression test: the old guard compared against a 512Gi shipped
        default and silently dropped an explicit request for that one value.
        """
        cluster = self._run_task(head_disk="512Gi", worker_disk="512Gi")

        head, workers = self._containers(cluster)
        self.assertEqual(head["resources"]["requests"]["ephemeral-storage"], "512Gi")
        self.assertEqual(workers["resources"]["requests"]["ephemeral-storage"], "512Gi")

    def test_env_configured_default_disk_reaches_the_pod(self):
        """A deployment-level RAY_DEFAULT_*_DISK now reaches the pod spec."""
        self._DEFAULT_DISK = "256Gi"
        cluster = self._run_task()

        head, workers = self._containers(cluster)
        self.assertEqual(head["resources"]["requests"]["ephemeral-storage"], "256Gi")
        self.assertEqual(workers["resources"]["requests"]["ephemeral-storage"], "256Gi")

    def test_env_override_gpu_reaches_the_pod(self):
        """A RAY_OVERRIDE_*_GPU env var flows through like the parameter."""
        cluster = self._run_task(
            environ={"RAY_OVERRIDE_HEAD_GPU.examples.demo.train": "2"}
        )

        head, _ = self._containers(cluster)
        self.assertEqual(head["resources"]["requests"]["nvidia.com/gpu"], 2)
        self.assertEqual(head["resources"]["limits"], {"nvidia.com/gpu": 2})
