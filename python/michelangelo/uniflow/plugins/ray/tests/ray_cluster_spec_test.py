"""Tests for the Ray cluster Starlark specification builder."""

from __future__ import annotations

import ast
import json
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


def _load_star_constants(*names: str, environ: dict | None = None):
    """Load the named task.star module-level constants against a real os.environ.

    Unlike _load_star_functions, which extracts only function bodies, this
    execs the matching top-level Assign statements so the os.environ.get(...)
    calls that define these constants actually run.
    """
    task_path = Path(__file__).resolve().parents[1] / "task.star"
    tree = ast.parse(task_path.read_text(), filename=str(task_path))
    assigns = [
        node
        for node in tree.body
        if isinstance(node, ast.Assign)
        and any(
            isinstance(target, ast.Name) and target.id in names
            for target in node.targets
        )
    ]
    assert len(assigns) == len(names), f"missing one of {names} in task.star"
    module = ast.fix_missing_locations(ast.Module(body=assigns, type_ignores=[]))
    globals_ = {"os": SimpleNamespace(environ=dict(environ or {}))}
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


class _TaskHarness:
    """Runs the real task() from task.star with the plugin runtime stubbed out.

    Captures the cluster spec handed to execute_ray_task and the kwargs handed
    to process_terminated_job, so tests can assert on what task() plumbs
    downstream without standing up a Ray cluster.
    """

    # The shipped default: empty string means "no explicit disk request".
    _DEFAULT_DISK = ""

    def _run_task(
        self,
        environ: dict | None = None,
        cluster_log_url: str = "",
        **task_kwargs,
    ):
        captured = {}
        terminated = {}

        def process_terminated_job(**kwargs):
            terminated.update(kwargs)
            return False

        def execute_ray_task(**kwargs):
            captured.update(kwargs)
            return "SUCCEEDED", None, "http://cluster", cluster_log_url

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
            "process_terminated_job": process_terminated_job,
            "io_read_json": lambda url: {"ok": True},
            "report_progress": lambda **kwargs: None,
            "callable_object": lambda func: func,
        }
        task, _, _, _ = _load_star_functions(
            "task",
            "ray_cluster_spec",
            "container_resources",
            "ray_config",
            extra_globals=stubs,
        )
        result = task(task_path="examples.demo.train", **task_kwargs)()
        self.assertEqual(result, {"ok": True})
        self._terminated = terminated
        return captured["cluster"]


class TestTaskResourcePlumbing(_TaskHarness, TestCase):
    """End-to-end tests that task() forwards resources into the cluster spec.

    This is the exact regression surface of the silently-dropped gpu/disk/
    object-store settings.
    """

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


class TestDiskDefaultConstants(TestCase):
    """Tests for the RAY_DEFAULT_*_DISK module constants themselves.

    TestTaskResourcePlumbing's _run_task stubs these names directly, so its
    tests never execute the os.environ.get(...) lines that actually define
    them. These tests load just those two Assign statements instead.
    """

    def test_defaults_to_empty_when_unset(self):
        """No RAY_DEFAULT_*_DISK env var means no explicit disk request."""
        head, worker = _load_star_constants(
            "RAY_DEFAULT_HEAD_DISK", "RAY_DEFAULT_WORKER_DISK"
        )

        self.assertEqual(head, "")
        self.assertEqual(worker, "")

    def test_reads_from_environment_when_set(self):
        """A deployment-level RAY_DEFAULT_*_DISK env var is picked up."""
        head, worker = _load_star_constants(
            "RAY_DEFAULT_HEAD_DISK",
            "RAY_DEFAULT_WORKER_DISK",
            environ={
                "RAY_DEFAULT_HEAD_DISK": "256Gi",
                "RAY_DEFAULT_WORKER_DISK": "1Ti",
            },
        )

        self.assertEqual(head, "256Gi")
        self.assertEqual(worker, "1Ti")


class TestTaskLogURL(_TaskHarness, TestCase):
    """task() must report the log URL published on the RayCluster CR.

    RayClusterStatus.log_url is rendered by the controller from the platform's
    logPersistence.logURLFormat and points at durable, post-mortem logs. The
    dashboard URL in status.job_url dies with the cluster, so it is only a
    fallback for when log persistence is switched off.
    """

    _CR_LOG_URL = "http://localhost:9090/browser/ma/logs/uf-ray-abc_ray/"

    def test_cr_log_url_is_reported(self):
        """The CR's logUrl reaches process_terminated_job verbatim."""
        self._run_task(cluster_log_url=self._CR_LOG_URL)

        self.assertEqual(self._terminated["log_url"], self._CR_LOG_URL)

    def test_falls_back_to_dashboard_when_log_persistence_disabled(self):
        """An empty logUrl (persistence off) falls back to the cluster URL."""
        self._run_task(cluster_log_url="")

        self.assertEqual(self._terminated["log_url"], "http://cluster")


class TestReportRayTaskResultLogURL(TestCase):
    """report_ray_task_result() stamps the persisted log URL on every report.

    task_log is what the control plane records as the substep's logUrl
    (executeworkflow.go copies TaskProgress.TaskLog into StepInfo.LogUrl), so a
    terminal report carrying the live dashboard URL leaves operators with a link
    that stops resolving the moment the cluster is reclaimed.
    """

    _LOG_URL = "http://localhost:9090/browser/ray-history/log/uf-ray-abc_default/"

    def _report(self, job_state: str):
        """Run report_ray_task_result for a job state, returning (state, report)."""
        reports = []

        stubs = {
            "TIME_FOMART": "%Y-%m-%dT%H:%M:%S",
            "TASK_STATE_SUCCEEDED": "SUCCEEDED",
            "TASK_STATE_KILLED": "KILLED",
            "TASK_STATE_FAILED": "FAILED",
            "CACHE_OPERATION_PUT": "PUT",
            "json": json,
            "time": SimpleNamespace(
                time=lambda: 0.0,
                utc_format_seconds=lambda fmt, seconds: "1970-01-01T00:00:00",
            ),
            "get_cache_keys": lambda *args: [],
            "create_cached_output": lambda **kwargs: {"metadata": {"name": "cached-1"}},
            "report_progress": lambda **kwargs: reports.append(kwargs),
        }
        (report_ray_task_result,) = _load_star_functions(
            "report_ray_task_result",
            extra_globals=stubs,
        )
        state = report_ray_task_result(
            {"status": {"state": job_state}},
            "examples.demo.train",
            "train",
            self._LOG_URL,
            "1970-01-01T00:00:00",
            (),
            {},
            1,
            "v1",
            "demo",
            "s3://bucket/result.json",
        )
        self.assertEqual(len(reports), 1)
        return state, reports[0]

    def test_succeeded_reports_persisted_log_url(self):
        """A successful job's terminal report carries the CR log URL."""
        state, report = self._report("RAY_JOB_STATE_SUCCEEDED")

        self.assertEqual(state, "SUCCEEDED")
        self.assertEqual(report["task_log"], self._LOG_URL)

    def test_killed_reports_persisted_log_url(self):
        """A killed job's terminal report carries the CR log URL."""
        state, report = self._report("RAY_JOB_STATE_KILLED")

        self.assertEqual(state, "KILLED")
        self.assertEqual(report["task_log"], self._LOG_URL)

    def test_failed_reports_persisted_log_url(self):
        """A failed job's terminal report carries the CR log URL.

        This is the branch that matters most: a failure is exactly when someone
        follows the link, and by then the cluster has already been torn down.
        """
        state, report = self._report("RAY_JOB_STATE_ERROR")

        self.assertEqual(state, "FAILED")
        self.assertEqual(report["task_log"], self._LOG_URL)


class TestExecuteRayTaskLogURL(TestCase):
    """execute_ray_task() must never report the dashboard URL as the task log.

    Exercises the real execute_ray_task and report_ray_task_result together
    against a RayCluster CR shaped like the sandbox's: status.logUrl populated
    by the controller, status.jobUrl absent. Before the fix every progress
    report carried the jobUrl fallback string, which is what operators saw in
    the pipeline run's substep logUrl.
    """

    _LOG_URL = "http://localhost:9090/browser/ray-history/log/uf-ray-tp8z5_default/"
    _JOB_URL_FALLBACK = "UAPI did not report RayJob URL"

    def _execute(self, status: dict):
        """Run execute_ray_task against a CR status, returning (result, reports)."""
        reports = []
        cluster = {
            "metadata": {"name": "uf-ray-tp8z5", "namespace": "california-housing"},
            "status": status,
        }

        stubs = {
            "DEFAULT_CREATE_CLUSTER_TIMEOUT_SECONDS": 600,
            "TIME_FOMART": "%Y-%m-%dT%H:%M:%S",
            "TASK_STATE_PENDING": "PENDING",
            "TASK_STATE_RUNNING": "RUNNING",
            "TASK_STATE_SUCCEEDED": "SUCCEEDED",
            "TASK_STATE_KILLED": "KILLED",
            "TASK_STATE_FAILED": "FAILED",
            "CACHE_OPERATION_PUT": "PUT",
            "json": json,
            "time": SimpleNamespace(
                time=lambda: 0.0,
                utc_format_seconds=lambda fmt, seconds: "1970-01-01T00:00:00",
                sleep=lambda seconds: None,
            ),
            "atexit": SimpleNamespace(
                register=lambda *args, **kwargs: None,
                unregister=lambda *args, **kwargs: None,
            ),
            "ray": SimpleNamespace(
                create_cluster=lambda cluster, timeout_seconds: {
                    "rayCluster": cluster,
                    "activityId": "activity-1",
                },
                create_job=lambda entrypoint, ray_job_namespace, ray_job_name: {
                    "status": {"state": "RAY_JOB_STATE_SUCCEEDED"}
                },
                terminate_cluster=lambda *args: None,
            ),
            "ray_job_entrypoint": lambda *args: "python -m run_task",
            "fail": lambda message: self.fail(message),
            "get_cache_keys": lambda *args: [],
            "create_cached_output": lambda **kwargs: {"metadata": {"name": "cached-1"}},
            "report_progress": lambda **kwargs: reports.append(kwargs),
        }
        execute_ray_task, _, _ = _load_star_functions(
            "execute_ray_task",
            "report_ray_task_result",
            "terminate_cluster",
            extra_globals=stubs,
        )
        result = execute_ray_task(
            task_path="examples.demo.train",
            task_name="train",
            cluster=cluster,
            cluster_namespace="california-housing",
            runtime_env={},
            start_time_formated_str="1970-01-01T00:00:00",
            result_url="s3://bucket/result.json",
            args=(),
            kwargs={},
            retry_attempt_id=1,
            total_retry_attempt=1,
            cache_version="v1",
            namespace="california-housing",
        )
        return result, reports

    def test_cr_log_url_replaces_the_job_url_everywhere(self):
        """With logUrl set and jobUrl absent, no report carries the fallback."""
        result, reports = self._execute({"logUrl": self._LOG_URL})
        _, _, cluster_url, cluster_log_url = result

        # The CR has no jobUrl, so cluster_url is the fallback string. That is
        # precisely the value that must not reach any progress report.
        self.assertEqual(cluster_url, self._JOB_URL_FALLBACK)
        self.assertEqual(cluster_log_url, self._LOG_URL)

        logs = [report["task_log"] for report in reports]
        self.assertNotIn(self._JOB_URL_FALLBACK, logs)
        self.assertEqual([log for log in logs if log], [self._LOG_URL] * 2)

    def test_falls_back_to_dashboard_when_log_persistence_disabled(self):
        """Without a CR logUrl the reports carry the dashboard URL."""
        _, reports = self._execute({"jobUrl": "http://dashboard:8265"})

        logs = [report["task_log"] for report in reports]
        self.assertEqual([log for log in logs if log], ["http://dashboard:8265"] * 2)
