"""Tests for the Ray cluster Starlark specification builder."""

from __future__ import annotations

import ast
from pathlib import Path
from types import SimpleNamespace
from unittest import TestCase


def _load_ray_cluster_spec():
    """Load the actual ray_cluster_spec function with controlled globals."""
    task_path = Path(__file__).resolve().parents[1] / "task.star"
    tree = ast.parse(task_path.read_text(), filename=str(task_path))
    function = next(
        node
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name == "ray_cluster_spec"
    )
    module = ast.fix_missing_locations(ast.Module(body=[function], type_ignores=[]))
    globals_ = {
        "COMMONS_ENV": {},
        "IMAGE_PULL_POLICY": "Never",
        "RAY_ENV": {},
        "USER_ID": "test-user",
        "os": SimpleNamespace(environ={}),
    }
    exec(compile(module, str(task_path), "exec"), globals_)
    return globals_["ray_cluster_spec"]


class TestRayClusterSpec(TestCase):
    """Tests for ray_cluster_spec()."""

    def _build_spec(self, namespace: str):
        ray_cluster_spec = _load_ray_cluster_spec()
        return ray_cluster_spec(
            namespace=namespace,
            image="test-image",
            head_resource={"cpu": 1, "memory": "1Gi"},
            worker_resource={"cpu": 1, "memory": "1Gi"},
            worker_instances=1,
        )

    def test_preserves_custom_namespace(self):
        """A project namespace is retained in cluster metadata."""
        spec = self._build_spec("ma-dev-test")

        self.assertEqual(spec["metadata"]["namespace"], "ma-dev-test")

    def test_preserves_default_namespace(self):
        """Default-namespace workflows remain unchanged."""
        spec = self._build_spec("default")

        self.assertEqual(spec["metadata"]["namespace"], "default")
