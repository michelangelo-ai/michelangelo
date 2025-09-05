import json
import tempfile
import unittest
from unittest import mock
import fsspec

from michelangelo.uniflow.core.decorator import write_task_result, task
from michelangelo.uniflow.core.ref import ref, unref
from michelangelo.uniflow.core.io_registry import default_io
from tests.uniflow.core.test_task_config import TaskA


class TestNoneHandling(unittest.TestCase):
    """Tests for proper handling of None return values in uniflow tasks."""

    def test_write_task_result_with_none(self):
        """Test that write_task_result converts None to empty dict."""
        with tempfile.NamedTemporaryFile(mode="w+", suffix=".json", delete=False) as f:
            result_url = f"file://{f.name}"

            # Write None value
            write_task_result(result_url, None)

            # Read back and verify it's an empty dict
            with fsspec.open(result_url, mode="rt") as read_f:
                result = json.load(read_f)

            self.assertEqual(result, {})

    def test_write_task_result_with_valid_values(self):
        """Test that write_task_result preserves non-None values."""
        test_cases = [{"key": "value"}, [1, 2, 3], "string_value", 42, True, []]

        for test_value in test_cases:
            with tempfile.NamedTemporaryFile(
                mode="w+", suffix=".json", delete=False
            ) as f:
                result_url = f"file://{f.name}"

                # Write test value
                write_task_result(result_url, test_value)

                # Read back and verify it's preserved
                with fsspec.open(result_url, mode="rt") as read_f:
                    result = json.load(read_f)

                self.assertEqual(result, test_value)

    def test_ref_unref_none_values(self):
        """Test ref/unref with None values in various scenarios."""
        # Test direct None
        self.assertIsNone(ref(None, default_io))
        self.assertIsNone(unref(None, default_io))

        # Test None in nested structures
        complex_data = {
            "none_value": None,
            "list_with_nones": [1, None, "test", None],
            "nested": {"inner_none": None, "inner_list": [None, {"deep_none": None}]},
        }

        # ref should preserve None values
        ref_result = ref(complex_data, default_io)
        self.assertIsNone(ref_result["none_value"])
        self.assertIsNone(ref_result["list_with_nones"][1])
        self.assertIsNone(ref_result["list_with_nones"][3])
        self.assertIsNone(ref_result["nested"]["inner_none"])
        self.assertIsNone(ref_result["nested"]["inner_list"][0])
        self.assertIsNone(ref_result["nested"]["inner_list"][1]["deep_none"])

        # unref should preserve None values
        unref_result = unref(ref_result, default_io)
        self.assertIsNone(unref_result["none_value"])
        self.assertIsNone(unref_result["list_with_nones"][1])
        self.assertIsNone(unref_result["list_with_nones"][3])
        self.assertIsNone(unref_result["nested"]["inner_none"])
        self.assertIsNone(unref_result["nested"]["inner_list"][0])
        self.assertIsNone(unref_result["nested"]["inner_list"][1]["deep_none"])


class TestTaskNoneIntegration(unittest.TestCase):
    """Integration tests for tasks that return None."""

    @task(config=TaskA())
    def task_returns_none(self):
        """A task that doesn't return anything (returns None implicitly)."""
        # Do some work but don't return anything
        x = 1 + 1
        # No return statement - returns None

    @task(config=TaskA())
    def task_returns_explicit_none(self):
        """A task that explicitly returns None."""
        return None

    @task(config=TaskA())
    def task_returns_valid_value(self):
        """A task that returns a valid value for comparison."""
        return {"status": "success", "data": [1, 2, 3]}

    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_task_implicit_none_return(self):
        """Test task that implicitly returns None (no return statement)."""
        # This should not raise an exception
        result = TestTaskNoneIntegration.task_returns_none(self)

        # The task execution should complete successfully and return None
        self.assertIsNone(result)

    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_task_explicit_none_return(self):
        """Test task that explicitly returns None."""
        # This should not raise an exception
        result = TestTaskNoneIntegration.task_returns_explicit_none(self)

        # The task execution should complete successfully and return None
        self.assertIsNone(result)

    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_task_valid_return_still_works(self):
        """Test that tasks with valid returns still work as expected."""
        result = TestTaskNoneIntegration.task_returns_valid_value(self)

        # Should get the expected return value
        self.assertEqual(result, {"status": "success", "data": [1, 2, 3]})

    @mock.patch.dict(
        "os.environ",
        {
            "UF_STORAGE_URL": "memory://test",
        },
    )
    def test_task_none_with_result_url(self):
        """Test task with None return when result_url is provided (like in remote execution)."""
        with tempfile.NamedTemporaryFile(mode="w+", suffix=".json", delete=False) as f:
            result_url = f"file://{f.name}"

            # Call task with _uf_result_url parameter (simulating remote execution)
            result = TestTaskNoneIntegration.task_returns_none(
                self, _uf_result_url=result_url
            )

            # Task should return None
            self.assertIsNone(result)

            # Result file should contain empty dict (not null)
            with fsspec.open(result_url, mode="rt") as read_f:
                stored_result = json.load(read_f)

            self.assertEqual(stored_result, {})


if __name__ == "__main__":
    unittest.main()