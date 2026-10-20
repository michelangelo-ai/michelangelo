"""Shared test case for Variable subclasses, copied from the internal SDK."""

# ruff: noqa: I001
from dataclasses import is_dataclass

from unittest import TestCase
from unittest.mock import patch, MagicMock

from michelangelo.uniflow.core import IO
from michelangelo.workflow.variables._private.base import Variable

from typing import Optional


class BaseVariableTestCase(TestCase):
    """Base test case that checks a variable's save/load contract."""

    def setUp(self):
        """Patch IO creation so saves and loads go through a mock."""
        self.patcher = patch(
            "michelangelo.workflow.variables._private.base._create_io", spec=True
        )
        self.mock_create_io = self.patcher.start()

    def tearDown(self):
        """Stop the IO patch."""
        self.patcher.stop()

    def check_variable(
        self, variable: Variable, load_func: Optional[str] = None, use_io: bool = True
    ):
        """Assert the variable saves once, reloads once, and round-trips."""
        value = variable.value

        mock_io_instance = MagicMock(spec=IO)
        mock_io_instance.read.return_value = value
        self.mock_create_io.return_value = mock_io_instance

        # must be a dataclass for Uniflow compatibility
        self.assertTrue(is_dataclass(variable))
        self.assertIsNotNone(variable.value)
        self.assertIsNotNone(variable.path)
        self.assertFalse(variable._saved)

        # Save the value via the variable
        variable.save()
        variable.save()  # multiple calls should only save once
        if use_io:
            mock_io_instance.write.assert_called_once()
        self.assertTrue(variable._saved)

        # Load value using the variable
        variable._value = None
        variable._saved = False
        if load_func:
            getattr(variable, load_func)()
        variable._load()  # multiple calls should only load once
        loaded_value = variable.value

        if use_io:
            mock_io_instance.read.assert_called_once()
        self.assertEqual(loaded_value, value)
        self.assertTrue(variable._saved)
