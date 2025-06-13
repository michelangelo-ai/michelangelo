import fsspec
import ast
import io
import gzip
import random
from dataclasses import dataclass
from typing import Optional
import unittest
from unittest import mock
from michelangelo.uniflow.core.decorator import task, TaskFunction, workflow
from michelangelo.uniflow.core.task_config import Dependencies
from michelangelo.uniflow.core.ref import Ref
from tests.uniflow.core.test_task_config import TaskA, TaskB, a_environ, b_environ


@dataclass
class RandomTextSpec:
    n: int
    vocabulary: list[str]
    encoding: str = "utf-8"
    seed: Optional[int] = None


@dataclass
class Data:
    encoding: str
    size: int
    bytes: io.BytesIO


@task(config=TaskA(cpu=2))
def generate_random_text(spec: RandomTextSpec) -> Data:
    """
    Generates random text based on the given spec. Returns the generated text as bytes.
    """
    # Ensure that the task decorator has called the TaskA.pre_run hook which initializes the global a_environ.
    assert a_environ
    assert isinstance(a_environ["config"], TaskA)
    assert a_environ["config"].cpu == 2

    # Ensure that the task decorator didn't change the input object in an unexpected way.
    assert isinstance(spec, RandomTextSpec)
    assert spec.n > 0
    assert isinstance(spec.vocabulary, list)
    assert spec.vocabulary

    # Generate random text.
    r = random.Random(spec.seed)
    text = " ".join(r.choice(spec.vocabulary) for _ in range(spec.n))
    text_b = text.encode(spec.encoding)
    return Data(
        encoding=spec.encoding,
        bytes=io.BytesIO(text_b),
        size=len(text_b),
    )


@task(config=TaskA(cpu=4), alias="gzip")
def gzip_compress(data: io.BytesIO) -> Data:
    # Ensure that the task decorator has called the TaskA.pre_run hook which initializes the global a_environ.
    assert a_environ
    assert isinstance(a_environ["config"], TaskA)
    assert a_environ["config"].cpu == 4

    # Ensure that the task decorator didn't change the input object in an unexpected way.
    assert isinstance(data, io.BytesIO)

    # Gzip compress the data.
    res = io.BytesIO()
    with gzip.GzipFile(fileobj=res, mode="wb") as f:
        f.write(data.getvalue())

    return Data(
        encoding="gzip",
        bytes=res,
        size=len(res.getvalue()),
    )


@task(config=TaskA())
def echo_task(x):
    # Verify that the task decorator has invoked the pre_run hook of TaskA, which is responsible for initializing
    # the global a_environ.
    assert a_environ
    assert isinstance(a_environ["config"], TaskA)

    # Delegate call to the internal task function _echo_task to process the input x. It ensures that nested task calls work as expected.
    return _echo_task(x)


@task(config=TaskB())
def _echo_task(x):
    # This function is intended to be a nested task, meaning it should only be invoked by another task.
    # Calling it directly will result in an error.

    # Check that the 'b_environ' variable is not initialized. This ensures that the task is being called by 'echo_task',
    # in which case the 'pre_run' hook of TaskB should not be triggered.
    assert not b_environ
    return x


@workflow()
def generate_random_text_workflow(
    spec: RandomTextSpec,
    compress: bool = False,
) -> Data:
    data = generate_random_text(spec)
    if compress:
        data = gzip_compress(data.bytes)
    return data


@workflow()
def with_overrides_workflow(a, b) -> tuple:
    # Override task config
    echo_a = echo_task.with_overrides(alias="echo_a", config=TaskA(cpu=1))
    a = echo_a(a)

    # Override task config and call in-line.
    b = echo_task.with_overrides(alias="echo_b")(b)

    return a, b


class TaskTest(unittest.TestCase):
    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_task_ref_unref(self):
        # Call the generate_random_text task function.
        r_text = generate_random_text(
            RandomTextSpec(
                n=1000,
                vocabulary=["foo", "bar", "fiz", "buz"],
                encoding="utf-8",
            )
        )

        # Assert the task result types:
        #   - JSON-compatible types and dataclasses are not Ref'ed.
        #   - Dataclasses are not Ref'ed.
        #   - BytesIO is Ref'ed.

        self.assertIsInstance(r_text, Data)
        self.assertIsInstance(r_text.bytes, Ref)
        self.assertIsInstance(r_text.size, int)
        self.assertIsInstance(r_text.encoding, str)

        # Call the gzip_compress task function passing the Ref of BytesIO.
        gziped_text = gzip_compress(r_text.bytes)

        # Sanity check: The gziped text size must be less than the original text size.
        self.assertIsInstance(gziped_text.bytes, Ref)
        self.assertLess(gziped_text.size, r_text.size)

    def test_nested_task_call(self):
        # This test checks that the task function can call another task function directly.
        # In this case, the nested task call should be behave as if it wasn't decorated with the @task decorator.
        self.assertEqual("test", echo_task("test"))

    def test_task_transpile(self):
        self.assertIsInstance(generate_random_text, TaskFunction)

        deps = Dependencies()
        exp = generate_random_text._transpile(deps)

        # Ensure that the @task "config" properties are included in the transpiled expression.
        task_params = "cpu=2, memory='1g', spot_instance=False, cache_enabled=False, cache_version=None"
        expected_str = f"__task_a('tests.uniflow.core.test_decorator.generate_random_text', {task_params})"
        self.assertEqual(
            expected_str,
            ast.unparse(exp),
        )

        # Ensure that dependencies contain a single dependency for the "__task_a" attribute.
        self.assertEqual(0, len(deps.star_plugins))
        self.assertEqual(0, len(deps.py_functions))
        self.assertEqual(1, len(deps.star_attributes))
        self.assertIn("__task_a", deps.star_attributes)

    def test_task_transpile_with_alias(self):
        self.assertIsInstance(gzip_compress, TaskFunction)

        # Transpile gzip_compress task function which has the "alias" property set to "gzip".
        deps = Dependencies()
        exp = gzip_compress._transpile(deps)

        # Ensure that the "alias" property is included in the transpiled expression.
        task_params = "alias='gzip', cpu=4, memory='1g', spot_instance=False, cache_enabled=False, cache_version=None"
        expected_str = f"__task_a('tests.uniflow.core.test_decorator.gzip_compress', {task_params})"
        self.assertEqual(
            expected_str,
            ast.unparse(exp),
        )


class TestWorkflow(unittest.TestCase):
    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_local_run(self):
        spec = RandomTextSpec(
            n=100,
            vocabulary=[
                "foo",
                "bar",
            ],
            encoding="utf-8",
        )
        data = generate_random_text_workflow(
            spec=spec,
            compress=True,
        )
        self.assertIsNotNone(data)
        self.assertEqual("gzip", data.encoding)
        self.assertIsInstance(data.size, int)
        self.assertGreater(data.size, 0)

        assert isinstance(data.bytes, Ref)

        with fsspec.open(data.bytes.url, mode="rb") as f:
            actual_data = f.read()

        self.assertEqual(data.size, len(actual_data))

    def test_local_run_with_overrides(self):
        a, b = with_overrides_workflow("foo", "bar")
        self.assertEqual(a, "foo")
        self.assertEqual(b, "bar")
