"""Unit tests for the shared SparkJob-CRD helpers in commons.star.

Uses the same AST-extraction technique as
michelangelo/uniflow/plugins/ray/tests/ray_cluster_spec_test.py to unit test
individual Starlark functions without needing the full Starlark runtime.
"""

import ast
import unittest
from pathlib import Path

COMMONS_PATH = (
    Path(__file__).resolve().parents[2] / "michelangelo" / "uniflow" / "commons.star"
)


def _load_commons_function(name):
    """Load a single top-level function from commons.star by name."""
    tree = ast.parse(COMMONS_PATH.read_text(), filename=str(COMMONS_PATH))
    function = next(
        node
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name == name
    )
    module = ast.fix_missing_locations(ast.Module(body=[function], type_ignores=[]))
    globals_ = {}
    exec(compile(module, str(COMMONS_PATH), "exec"), globals_)
    return globals_[name]


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


if __name__ == "__main__":
    unittest.main()
