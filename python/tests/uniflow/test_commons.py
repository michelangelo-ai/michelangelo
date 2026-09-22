"""Unit tests for the shared SparkJob-CRD helpers in commons.star.

Uses the same AST-extraction technique as
michelangelo/uniflow/plugins/ray/tests/ray_cluster_spec_test.py to unit test
individual Starlark functions without needing the full Starlark runtime.
"""

import ast
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock

COMMONS_PATH = (
    Path(__file__).resolve().parents[2] / "michelangelo" / "uniflow" / "commons.star"
)


class StarlarkFailError(Exception):
    """Raised by the stubbed Starlark fail() builtin used in these tests."""


def _fail(*args):
    raise StarlarkFailError(" ".join(str(a) for a in args))


def _starlark_type(value):
    """Mimic Starlark's type(), which returns type names as strings."""
    if value is None:
        return "NoneType"
    if isinstance(value, bool):
        return "bool"
    if isinstance(value, dict):
        return "dict"
    if isinstance(value, list):
        return "list"
    if isinstance(value, str):
        return "string"
    if isinstance(value, int):
        return "int"
    return type(value).__name__


def _base_globals():
    """Stub globals for the Starlark builtins commons.star's job functions use."""
    return {
        "fail": _fail,
        "type": _starlark_type,
        "print": lambda *a, **k: None,
        "TASK_STATE_PENDING": "PENDING",
        "TASK_STATE_RUNNING": "RUNNING",
        "TASK_STATE_SUCCEEDED": "SUCCEEDED",
        "TASK_STATE_FAILED": "FAILED",
        "TASK_STATE_KILLED": "KILLED",
        "TIME_FOMART": "%Y-%m-%d %H:%M:%S",
        "time": SimpleNamespace(
            time=lambda: 0,
            utc_format_seconds=lambda fmt, secs: "2024-01-01 00:00:00",
        ),
        "spark": SimpleNamespace(
            succeeded_condition_type="Succeeded",
            killed_condition_type="Killed",
            running_condition_type="Running",
        ),
        "atexit": SimpleNamespace(register=MagicMock(), unregister=MagicMock()),
        "report_progress": MagicMock(),
    }


def _load_commons_functions(*names, globals_=None):
    """Load one or more top-level functions from commons.star by name.

    Functions are compiled together so cross-calls between them (e.g.
    execute_spark_crd_job calling report_spark_crd_job_terminated) resolve
    against the same namespace.
    """
    tree = ast.parse(COMMONS_PATH.read_text(), filename=str(COMMONS_PATH))
    nodes = [
        node
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name in names
    ]
    module = ast.fix_missing_locations(ast.Module(body=nodes, type_ignores=[]))
    globals_ = dict(globals_ or _base_globals())
    exec(compile(module, str(COMMONS_PATH), "exec"), globals_)
    return globals_


def _load_commons_function(name):
    """Load a single top-level function from commons.star by name."""
    return _load_commons_functions(name)[name]


class TestGetJobLogUrl(unittest.TestCase):
    """Tests for get_job_log_url()."""

    def setUp(self):
        """Load the function under test fresh for each test."""
        self.get_job_log_url = _load_commons_function("get_job_log_url")

    def test_builds_url_when_prefix_and_job_name_present(self):
        """A configured prefix and job name produce a full log URL."""
        url = self.get_job_log_url(
            "https://logs.example.com/spark", "uniflow-sp-abc123"
        )

        self.assertEqual(url, "https://logs.example.com/spark/uniflow-sp-abc123.log")

    def test_empty_when_prefix_missing(self):
        """No prefix means no generated log URL."""
        self.assertEqual(self.get_job_log_url("", "uniflow-sp-abc123"), "")
        self.assertEqual(self.get_job_log_url(None, "uniflow-sp-abc123"), "")

    def test_empty_when_job_name_missing(self):
        """No job name means no generated log URL."""
        self.assertEqual(self.get_job_log_url("https://logs.example.com/spark", ""), "")


class TestBuildSparkCrdJob(unittest.TestCase):
    """Tests for build_spark_crd_job(), shared by spark_task and scala_task."""

    def setUp(self):
        """Load the function under test fresh for each test."""
        self.build_spark_crd_job = _load_commons_function("build_spark_crd_job")

    def _build(self, **overrides):
        kwargs = {
            "image": "test-image:latest",
            "main_file": "/app/run_task.py",
            "main_class": "com.uber.Main",
            "main_args": [],
            "driver_resource": {"cpu": 1, "memory": "1Gi"},
            "executor_resource": {"cpu": 1, "memory": "1Gi"},
            "executor_instances": 2,
            "generate_name_prefix": "uniflow-sp-",
            "env": [{"name": "FOO", "value": "bar"}],
        }
        kwargs.update(overrides)
        return self.build_spark_crd_job(**kwargs)

    def test_spark_job_shape(self):
        """spark_task's parameterization produces the expected SparkJob CRD."""
        job = self._build(
            generate_name_prefix="uniflow-sp-",
            main_args=["--result-url", "s3://default/result.json"],
        )

        self.assertEqual(job["kind"], "SparkJob")
        self.assertEqual(job["metadata"]["generateName"], "uniflow-sp-")
        self.assertEqual(job["spec"]["mainApplicationFile"], "/app/run_task.py")
        expected_args = ["--result-url", "s3://default/result.json"]
        self.assertEqual(job["spec"]["mainArgs"], expected_args)
        self.assertEqual(job["spec"]["driver"]["pod"]["image"], "test-image:latest")
        self.assertEqual(job["spec"]["executor"]["pod"]["image"], "test-image:latest")
        self.assertEqual(job["spec"]["executor"]["instances"], 2)

    def test_scala_job_has_no_main_args(self):
        """scala_task's parameterization submits no mainArgs (JAR-only)."""
        job = self._build(generate_name_prefix="uniflow-sc-", main_args=[])

        self.assertEqual(job["metadata"]["generateName"], "uniflow-sc-")
        self.assertEqual(job["spec"]["mainArgs"], [])

    def test_env_and_resources_propagate_to_driver_and_executor(self):
        """Caller-supplied env and resources reach both driver and executor pods."""
        env = [{"name": "FOO", "value": "bar"}]
        driver_resource = {"cpu": 2, "memory": "2Gi"}
        executor_resource = {"cpu": 4, "memory": "4Gi"}

        job = self._build(
            env=env,
            driver_resource=driver_resource,
            executor_resource=executor_resource,
        )

        self.assertEqual(job["spec"]["driver"]["pod"]["env"], env)
        self.assertEqual(job["spec"]["executor"]["pod"]["env"], env)
        self.assertEqual(job["spec"]["driver"]["pod"]["resource"], driver_resource)
        self.assertEqual(job["spec"]["executor"]["pod"]["resource"], executor_resource)

    def test_uses_michelangelo_config_configmap(self):
        """Driver and executor pods both pull env from michelangelo-config."""
        job = self._build()

        driver_pod = job["spec"]["driver"]["pod"]
        executor_pod = job["spec"]["executor"]["pod"]
        driver_ref = driver_pod["envFrom"][0]["configMapRef"]
        executor_ref = executor_pod["envFrom"][0]["configMapRef"]
        driver_ref = driver_ref["localObjectReference"]
        executor_ref = executor_ref["localObjectReference"]

        self.assertEqual(driver_ref["name"], "michelangelo-config")
        self.assertEqual(executor_ref["name"], "michelangelo-config")


def _condition(condition_type, status, message="", reason=""):
    return {
        "type": condition_type,
        "status": status,
        "message": message,
        "reason": reason,
    }


class TestReportSparkCrdJobTerminated(unittest.TestCase):
    """Tests for report_spark_crd_job_terminated()."""

    def setUp(self):
        """Load the function under test fresh for each test."""
        ns = _load_commons_functions(
            "get_job_log_url", "report_spark_crd_job_terminated"
        )
        self.report_terminated = ns["report_spark_crd_job_terminated"]

    def _call(self, job, unexpected_exit=False, job_name=""):
        return self.report_terminated(
            job,
            task_name="my_task",
            task_path="pkg.my_task",
            start_time_formatted_str="2024-01-01 00:00:00",
            retry_attempt_id=0,
            first_activity_id="act-1",
            job_label="Spark",
            log_url_prefix="",
            unexpected_exit=unexpected_exit,
            job_name=job_name,
        )

    def test_not_a_dict_returns_failed(self):
        """A non-dict job (e.g. None) is treated as failed."""
        self.assertEqual(self._call(None), "FAILED")

    def test_no_matching_conditions_returns_empty(self):
        """A job with no succeeded/killed conditions reports no terminal state."""
        job = {"status": {"statusConditions": []}}

        self.assertEqual(self._call(job), "")

    def test_killed_condition_true_returns_killed(self):
        """A true killed condition reports and returns TASK_STATE_KILLED."""
        job = {
            "status": {
                "statusConditions": [_condition("Killed", "CONDITION_STATUS_TRUE")]
            }
        }

        self.assertEqual(self._call(job), "KILLED")

    def test_succeeded_condition_true_returns_succeeded(self):
        """A true succeeded condition reports and returns TASK_STATE_SUCCEEDED."""
        job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_TRUE")]
            }
        }

        self.assertEqual(self._call(job), "SUCCEEDED")

    def test_succeeded_condition_false_returns_failed(self):
        """A false succeeded condition reports and returns TASK_STATE_FAILED."""
        job = {
            "status": {
                "statusConditions": [
                    _condition(
                        "Succeeded",
                        "CONDITION_STATUS_FALSE",
                        message="boom",
                        reason="OOM",
                    )
                ]
            }
        }

        self.assertEqual(self._call(job), "FAILED")

    def test_unexpected_exit_on_failure_raises(self):
        """unexpected_exit=True on a failed job raises via the fail() builtin."""
        job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_FALSE")]
            }
        }

        with self.assertRaises(StarlarkFailError):
            self._call(job, unexpected_exit=True)


class TestCheckSparkCrdJobFinalStateAtWorkflowExit(unittest.TestCase):
    """Tests for check_spark_crd_job_final_state_at_workflow_exit()."""

    def setUp(self):
        """Load the function under test, with a controllable spark.sensor_job."""
        self.globals_ = _base_globals()
        ns = _load_commons_functions(
            "get_job_log_url",
            "report_spark_crd_job_terminated",
            "check_spark_crd_job_final_state_at_workflow_exit",
            globals_=self.globals_,
        )
        self.check_final_state = ns["check_spark_crd_job_final_state_at_workflow_exit"]

    def _call(self, final_job):
        self.globals_["spark"].sensor_job = MagicMock(return_value=final_job)
        return self.check_final_state(
            {"metadata": {"name": "uniflow-sp-abc"}},
            task_name="my_task",
            task_path="pkg.my_task",
            start_time_formatted_str="2024-01-01 00:00:00",
            retry_attempt_id=0,
            first_activity_id="act-1",
            job_label="Spark",
            log_url_prefix="",
        )

    def test_senses_job_then_reports_success_without_raising(self):
        """A succeeded final state is reported without raising."""
        job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_TRUE")]
            }
        }

        self._call(job)

        self.globals_["spark"].sensor_job.assert_called_once()

    def test_senses_job_then_raises_on_unexpected_failure(self):
        """A failed final state raises, since this runs at workflow exit."""
        job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_FALSE")]
            }
        }

        with self.assertRaises(StarlarkFailError):
            self._call(job)


class TestExecuteSparkCrdJob(unittest.TestCase):
    """Tests for execute_spark_crd_job()."""

    def setUp(self):
        """Load the function under test, with controllable spark.* mocks."""
        self.globals_ = _base_globals()
        ns = _load_commons_functions(
            "get_job_log_url",
            "report_spark_crd_job_terminated",
            "check_spark_crd_job_final_state_at_workflow_exit",
            "execute_spark_crd_job",
            globals_=self.globals_,
        )
        self.execute = ns["execute_spark_crd_job"]

    def _call(self, spark_crd_job):
        return self.execute(
            namespace="ma-dev-test",
            task_name="my_task",
            task_path="pkg.my_task",
            spark_crd_job=spark_crd_job,
            start_time_formatted_str="2024-01-01 00:00:00",
            retry_attempt_id=0,
            total_retry_attempt=3,
            job_label="Spark",
            log_url_prefix="",
        )

    def test_job_creation_failure_raises(self):
        """A None sparkJob from create_job reports failure and raises."""
        spark = self.globals_["spark"]
        spark.create_job = MagicMock(
            return_value={"sparkJob": None, "activityId": "act-1"}
        )

        with self.assertRaises(StarlarkFailError):
            self._call({"kind": "SparkJob"})

    def test_happy_path_submits_senses_and_reports_success(self):
        """A job that runs to completion returns SUCCEEDED and the terminal job."""
        spark = self.globals_["spark"]
        created_job = {"metadata": {"name": "uniflow-sp-abc"}, "status": {}}
        running_job = {"status": {"jobUrl": "https://driver.example.com"}}
        terminated_job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_TRUE")]
            }
        }

        spark.create_job = MagicMock(
            return_value={"sparkJob": created_job, "activityId": "act-1"}
        )
        spark.sensor_job = MagicMock(side_effect=[running_job, terminated_job])

        job_state, final_job = self._call({"kind": "SparkJob"})

        self.assertEqual(job_state, "SUCCEEDED")
        self.assertEqual(final_job, terminated_job)
        self.assertEqual(spark.sensor_job.call_count, 2)
        self.globals_["atexit"].register.assert_called_once()
        self.globals_["atexit"].unregister.assert_called_once()

    def test_failed_job_does_not_unregister_atexit_hook(self):
        """A job that ultimately fails leaves the atexit safety hook registered."""
        spark = self.globals_["spark"]
        created_job = {"metadata": {"name": "uniflow-sp-abc"}, "status": {}}
        running_job = {"status": {"jobUrl": "https://driver.example.com"}}
        terminated_job = {
            "status": {
                "statusConditions": [_condition("Succeeded", "CONDITION_STATUS_FALSE")]
            }
        }

        spark.create_job = MagicMock(
            return_value={"sparkJob": created_job, "activityId": "act-1"}
        )
        spark.sensor_job = MagicMock(side_effect=[running_job, terminated_job])

        job_state, final_job = self._call({"kind": "SparkJob"})

        self.assertEqual(job_state, "FAILED")
        self.assertEqual(final_job, terminated_job)
        self.globals_["atexit"].unregister.assert_not_called()


if __name__ == "__main__":
    unittest.main()
