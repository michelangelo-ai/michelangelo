import ast
import unittest
from dataclasses import dataclass
from pathlib import Path

from michelangelo.uniflow.core.task_config import TaskBinding, TaskConfig

# Binding for TaskA. That's how we associate TaskConfig Python class with a
# corresponding Starlark function.
_task_a_binding = TaskBinding(
    star_file=Path(__file__).parent / "task_a.star",
    function="task_a",
    export="__task_a",
)

# Binding for TaskB.
_task_b_binding = TaskBinding(
    star_file=Path(__file__).parent / "task_b.star",
    function="task_b",
    export="__task_b",
)

# Global environments. They are used to test the global context object init
# and destruction in task's pre_run and post_run hooks.
a_environ = {}
b_environ = {}


# TestA is a TaskConfig class used for testing. It doesn't have any practical
# use, other than testing.
@dataclass  # TaskConfig sub-classes should be dataclasses.
class TaskA(TaskConfig):  # noqa: D101
    # Task-specific configuration properties are defined as dataclass fields.
    cpu: int = 1
    memory: str = "1g"
    spot_instance: bool = False

    def get_binding(self) -> TaskBinding:  # noqa: D102
        return _task_a_binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:  # noqa: D102
        return _task_a_binding

    def pre_run(self):  # noqa: D102
        # Initialize the test environment.
        assert not a_environ
        a_environ["config"] = self

    def post_run(self):  # noqa: D102
        # Clean up the test environment.
        a_environ.clear()


# TestB is a TaskConfig class used for testing. It's similar to TaskA, but has
# different configuration properties.
@dataclass
class TaskB(TaskConfig):  # noqa: D101
    # Task-specific configuration properties are defined as dataclass fields.
    cpu: int = 1
    memory: str = "1g"
    spot_instance: bool = False

    def get_binding(self) -> TaskBinding:  # noqa: D102
        return _task_b_binding

    @classmethod
    def get_config_binding(cls) -> TaskBinding:  # noqa: D102
        return _task_b_binding

    def pre_run(self):  # noqa: D102
        # Initialize the test environment.
        assert not b_environ
        b_environ["config"] = self

    def post_run(self):  # noqa: D102
        # Clean up the test environment.
        b_environ.clear()


class Test(unittest.TestCase):  # noqa: D101
    def test_instantiation(self):  # noqa: D102
        # Just check that the task classes are instantiable
        self.assertIsInstance(TaskA(), TaskConfig)
        self.assertIsInstance(TaskB(), TaskConfig)

    def test_invalid_task_instantiation(self):  # noqa: D102
        # TODO: andrii: validate TaskConfig sub-classes
        # Ideally, we should validate task config classes to ensure that they
        # comply with the rules. Ex:
        # - Task config class is a dataclass
        # - It doesn't contain reserved fields, such as "alias"
        pass

    def test_to_keywords(self):  # noqa: D102
        # 1. Default values
        keywords = TaskA().to_keywords()
        self.assertEqual(3, len(keywords))

        k = keywords[0]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("cpu", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual(1, k.value.value)  # type: ignore[attr-defined]

        k = keywords[1]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("memory", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual("1g", k.value.value)  # type: ignore[attr-defined]

        k = keywords[2]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("spot_instance", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual(False, k.value.value)  # type: ignore[attr-defined]

        # 2. Specified values
        keywords = TaskA(cpu=4, spot_instance=True).to_keywords()
        self.assertEqual(3, len(keywords))

        k = keywords[0]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("cpu", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual(4, k.value.value)  # type: ignore[attr-defined]

        k = keywords[1]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("memory", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual("1g", k.value.value)  # type: ignore[attr-defined]

        k = keywords[2]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("spot_instance", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual(True, k.value.value)  # type: ignore[attr-defined]

    def test_keywords_exclusion(self):  # noqa: D102
        # Task properties of None value are excluded from the keyword list.

        keywords = TaskA(cpu=None, spot_instance=None).to_keywords()  # type: ignore[arg-type]
        self.assertEqual(1, len(keywords))

        k = keywords[0]
        self.assertIsInstance(k, ast.keyword)
        self.assertEqual("memory", k.arg)
        self.assertIsInstance(k.value, ast.Constant)
        self.assertEqual("1g", k.value.value)  # type: ignore[attr-defined]
