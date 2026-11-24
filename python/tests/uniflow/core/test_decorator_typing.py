"""Typing regression tests for the uniflow decorator factories.

``task()``, ``workflow()`` and ``star_plugin()`` must keep their return
annotations: without them, mypy's ``disallow_untyped_decorators`` (part of
``--strict``) turns every decorated function into ``Any`` for downstream
users. The runtime tests always run. The static tests type-check a small
consumer module (``typing_samples/decorator_consumer.py``) with mypy and are
skipped when mypy is not installed.
"""

import inspect
import typing
import unittest
from pathlib import Path

import pytest

import michelangelo
from michelangelo.uniflow.core.decorator import (
    TaskFunction,
    star_plugin,
    task,
    workflow,
)
from tests.uniflow.core.test_task_config import TaskA

_CONSUMER = Path(__file__).parent / "typing_samples" / "decorator_consumer.py"
# The directory that contains the michelangelo package, so mypy checks this
# source tree rather than an installed wheel (which ships no py.typed).
_SOURCE_ROOT = Path(michelangelo.__file__).resolve().parent.parent


def _sample(x: int, y: str = "a") -> int:
    """Sample function wrapped by the decorators under test."""
    return x + len(y)


class DecoratorFactoryAnnotationTest(unittest.TestCase):
    """Runtime checks on the decorator factory annotations."""

    def test_factories_declare_return_annotation(self):
        """Each factory declares a return annotation."""
        for factory in (task, workflow, star_plugin):
            with self.subTest(factory=factory.__name__):
                self.assertIn("return", typing.get_type_hints(factory))

    def test_task_factory_returns_task_function_decorator(self):
        """task() is annotated to produce a TaskFunction."""
        hint = typing.get_type_hints(task)["return"]
        _, decorated = typing.get_args(hint)
        self.assertIs(typing.get_origin(decorated), TaskFunction)

    def test_task_function_declares_wrapper_attributes(self):
        """TaskFunction declares the attributes set by update_wrapper()."""
        hints = typing.get_type_hints(TaskFunction)
        for name in ("__name__", "__qualname__", "__wrapped__"):
            with self.subTest(attribute=name):
                self.assertIn(name, hints)

    def test_decorated_functions_keep_signature_and_metadata(self):
        """Decorated functions expose the wrapped function's metadata."""
        decorated = {
            "task": task(config=TaskA(cpu=1))(_sample),
            "workflow": workflow()(_sample),
            "star_plugin": star_plugin(binding="sample.fn")(_sample),
        }
        for name, fn in decorated.items():
            with self.subTest(decorator=name):
                self.assertEqual(inspect.signature(fn), inspect.signature(_sample))
                self.assertEqual(fn.__name__, _sample.__name__)
                self.assertEqual(fn.__qualname__, _sample.__qualname__)
                self.assertEqual(fn.__doc__, _sample.__doc__)
                self.assertIs(fn.__wrapped__, _sample)
        self.assertIsInstance(decorated["task"], TaskFunction)

    def test_overridden_tasks_keep_signature_and_metadata(self):
        """with_overrides() copies expose the wrapped function's metadata."""
        original = task(config=TaskA(cpu=1), alias="sample")(_sample)
        once = original.with_overrides(alias="sample_v2")
        overridden = {
            "once": once,
            "twice": once.with_overrides(config=TaskA(cpu=2)),
        }
        for name, fn in overridden.items():
            with self.subTest(overrides=name):
                self.assertIsInstance(fn, TaskFunction)
                self.assertIsNot(fn, original)
                self.assertIs(fn.__wrapped__, _sample)
                self.assertIs(inspect.unwrap(fn), _sample)
                self.assertEqual(fn.__name__, original.__name__)
                self.assertEqual(fn.__qualname__, original.__qualname__)
                self.assertEqual(fn.__doc__, _sample.__doc__)
                self.assertEqual(inspect.signature(fn), inspect.signature(_sample))
                self.assertEqual(fn._alias, "sample_v2")
                self.assertFalse(hasattr(fn, "_uf_task"))


def _run_mypy(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, *target: str):
    """Run mypy --strict on ``target`` against this source tree."""
    mypy_api = pytest.importorskip("mypy.api")
    monkeypatch.setenv("MYPYPATH", str(_SOURCE_ROOT))
    return mypy_api.run(
        [
            "--strict",
            "--no-incremental",
            # Report errors in the consumer only, not in michelangelo itself.
            "--follow-imports=silent",
            "--ignore-missing-imports",
            "--cache-dir",
            str(tmp_path / ".mypy_cache"),
            *target,
        ]
    )


def test_consumer_type_checks_under_strict(tmp_path, monkeypatch):
    """A consumer of the decorators passes mypy --strict."""
    stdout, stderr, status = _run_mypy(tmp_path, monkeypatch, str(_CONSUMER))
    assert status == 0, stdout + stderr


def test_wrong_argument_type_is_reported(tmp_path, monkeypatch):
    """A wrong argument type to each decorated function is reported."""
    consumer = tmp_path / "bad_consumer.py"
    consumer.write_text(
        _CONSUMER.read_text() + '\nadd("one")\npipeline("one")\nupper(1)\n',
    )
    stdout, stderr, status = _run_mypy(tmp_path, monkeypatch, str(consumer))
    assert status == 1, stdout + stderr
    errors = [line for line in stdout.splitlines() if "[arg-type]" in line]
    assert len(errors) == 3, stdout + stderr
