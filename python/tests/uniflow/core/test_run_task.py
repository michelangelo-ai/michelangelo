import json
import unittest
import uuid
from dataclasses import dataclass
from unittest import mock

import fsspec

from michelangelo.uniflow.core import task
from michelangelo.uniflow.core.decorator import task_context
from michelangelo.uniflow.core.run_task import main as run_task_main
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig
from michelangelo.uniflow.core.utils import dot_path


@dataclass
class TestTaskConfig(TaskConfig):  # noqa: D101
    def pre_run(self):  # noqa: D102
        pass

    def post_run(self):  # noqa: D102
        pass

    def get_binding(self) -> TaskBinding:  # noqa: D102
        raise NotImplementedError  # Not called in this test

    @classmethod
    def get_config_binding(cls) -> TaskBinding:  # noqa: D102
        raise NotImplementedError  # Not called in this test


@task(config=TestTaskConfig(), alias="echo")
def echo_task(x) -> dict:  # noqa: D103
    return {
        "input": x,
        "alias": task_context.alias,
    }


class Test(unittest.TestCase):  # noqa: D101
    def test_simple(self):  # noqa: D102
        result_url = _random_test_result_url()
        test_args = [
            "test",
            "--task",
            dot_path(echo_task),
            "--args",
            '["foo"]',
            "--kwargs",
            "{}",
            "--result-url",
            result_url,
        ]
        with mock.patch("sys.argv", test_args):
            run_task_main()

        with fsspec.open(result_url) as f:
            result = json.load(f)

        self.assertEqual(
            {
                "input": "foo",
                "alias": "echo",
            },
            result,
        )

    def test_overrides(self):  # noqa: D102
        result_url = _random_test_result_url()
        test_args = [
            "test",
            "--task",
            dot_path(echo_task),
            "--args",
            "[3.14]",
            "--kwargs",
            "{}",
            "--result-url",
            result_url,
            "--overrides",
            '{"alias": "pi_task"}',
        ]
        with mock.patch("sys.argv", test_args):
            run_task_main()

        with fsspec.open(result_url) as f:
            result = json.load(f)

        self.assertEqual(
            {
                "input": 3.14,
                "alias": "pi_task",
            },
            result,
        )

    def test_result_not_json(self):  # noqa: D102
        result_url = "memory://result.txt"  # Not a *.json file extension
        test_args = [
            "test",
            "--task",
            dot_path(echo_task),
            "--args",
            "[1]",
            "--kwargs",
            "{}",
            "--result-url",
            result_url,
        ]
        with mock.patch("sys.argv", test_args), self.assertRaises(AssertionError):
            run_task_main()


def _random_test_result_url():
    return f"memory://{uuid.uuid4()}.json"
