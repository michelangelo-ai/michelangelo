"""Tests for the Uniflow task runner entrypoint."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
import uuid
from dataclasses import dataclass
from pathlib import Path
from unittest import mock

import fsspec

from michelangelo.uniflow.core import task
from michelangelo.uniflow.core.decorator import task_context
from michelangelo.uniflow.core.run_task import main as run_task_main
from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig
from michelangelo.uniflow.core.utils import dot_path


@dataclass
class RunTaskConfig(TaskConfig):  # noqa: D101
    def pre_run(self):  # noqa: D102
        pass

    def post_run(self):  # noqa: D102
        pass

    def get_binding(self) -> TaskBinding:  # noqa: D102
        raise NotImplementedError  # Not called in this test

    @classmethod
    def get_config_binding(cls) -> TaskBinding:  # noqa: D102
        raise NotImplementedError  # Not called in this test


@task(config=RunTaskConfig(), alias="echo")
def echo_task(x) -> dict:  # noqa: D103
    return {
        "input": x,
        "alias": task_context.alias,
    }


@task(config=RunTaskConfig(), alias="capture_kwargs")
def capture_kwargs_task(**kwargs) -> dict:  # noqa: D103
    return kwargs


@task(config=RunTaskConfig(), alias="kwargs_size")
def kwargs_size_task(payload) -> dict:  # noqa: D103
    return {"size": len(payload)}


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

    def test_local_kwargs_file(self):  # noqa: D102
        result_url = _random_test_result_url()
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as kwargs_file:
            json.dump({"source": "local"}, kwargs_file)
            kwargs_file.flush()

            test_args = _task_args(
                capture_kwargs_task,
                result_url,
                "--kwargs-file",
                kwargs_file.name,
            )
            with mock.patch("sys.argv", test_args):
                run_task_main()

        with fsspec.open(result_url) as f:
            result = json.load(f)

        self.assertEqual({"source": "local"}, result)

    def test_memory_kwargs_file(self):  # noqa: D102
        kwargs_url = f"memory://{uuid.uuid4()}.json"
        result_url = _random_test_result_url()
        with fsspec.open(kwargs_url, mode="wt", encoding="utf-8") as f:
            json.dump({"source": "memory"}, f)

        test_args = _task_args(
            capture_kwargs_task,
            result_url,
            "--kwargs-file",
            kwargs_url,
        )
        with mock.patch("sys.argv", test_args):
            run_task_main()

        with fsspec.open(result_url) as f:
            result = json.load(f)

        self.assertEqual({"source": "memory"}, result)

    def test_kwargs_inputs_are_required_and_mutually_exclusive(self):  # noqa: D102
        result_url = _random_test_result_url()
        without_kwargs = [
            "test",
            "--task",
            dot_path(capture_kwargs_task),
            "--args",
            "[]",
            "--result-url",
            result_url,
        ]
        with (
            mock.patch("sys.argv", without_kwargs),
            self.assertRaisesRegex(SystemExit, "2"),
        ):
            run_task_main()

        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as kwargs_file:
            kwargs_file.write("{}")
            kwargs_file.flush()
            both_kwargs = [
                *without_kwargs,
                "--kwargs",
                "{}",
                "--kwargs-file",
                kwargs_file.name,
            ]
            with (
                mock.patch("sys.argv", both_kwargs),
                self.assertRaisesRegex(SystemExit, "2"),
            ):
                run_task_main()

    def test_kwargs_file_must_decode_to_dict(self):  # noqa: D102
        result_url = _random_test_result_url()
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as kwargs_file:
            kwargs_file.write("[]")
            kwargs_file.flush()

            test_args = _task_args(
                capture_kwargs_task,
                result_url,
                "--kwargs-file",
                kwargs_file.name,
            )
            with (
                mock.patch("sys.argv", test_args),
                self.assertRaisesRegex(
                    AssertionError,
                    "Expected kwargs to be a dict",
                ),
            ):
                run_task_main()

    def test_large_kwargs_file_in_subprocess(self):  # noqa: D102
        payload = "x" * (256 * 1024)
        encoded_kwargs = json.dumps({"payload": payload})
        self.assertGreater(len(encoded_kwargs.encode("utf-8")), 128 * 1024)

        with tempfile.TemporaryDirectory() as temp_dir:
            kwargs_path = Path(temp_dir) / "kwargs.json"
            result_path = Path(temp_dir) / "result.json"
            kwargs_path.write_text(encoded_kwargs, encoding="utf-8")

            process = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "michelangelo.uniflow.core.run_task",
                    "--task",
                    dot_path(kwargs_size_task),
                    "--args",
                    "[]",
                    "--kwargs-file",
                    str(kwargs_path),
                    "--result-url",
                    str(result_path),
                ],
                cwd=Path(__file__).parents[3],
                capture_output=True,
                check=False,
                env={
                    **os.environ,
                    "PYTHONPATH": os.pathsep.join(
                        [
                            str(Path(__file__).parent),
                            str(Path(__file__).parents[3]),
                        ]
                    ),
                },
                text=True,
            )

            self.assertEqual(0, process.returncode, process.stderr)
            self.assertEqual(
                {"size": len(payload)},
                json.loads(result_path.read_text(encoding="utf-8")),
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


def _task_args(task_function, result_url, kwargs_flag, kwargs_value):
    return [
        "test",
        "--task",
        dot_path(task_function),
        "--args",
        "[]",
        kwargs_flag,
        kwargs_value,
        "--result-url",
        result_url,
    ]
