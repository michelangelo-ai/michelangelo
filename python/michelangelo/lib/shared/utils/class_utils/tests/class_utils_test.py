"""Tests for class_utils."""

# ruff: noqa: I001
from unittest import TestCase
from michelangelo.lib.shared.utils.class_utils import (
    get_full_class_name,
    is_instance_of,
)

_MODULE = "michelangelo.lib.shared.utils.class_utils.tests.class_utils_test"


class Dummy:
    """Test class."""


class Dummy1(Dummy):
    """Test subclass."""


class ClassUtilsTest(TestCase):
    """Tests for get_full_class_name and is_instance_of."""

    def test_get_builtin_full_class_name(self):
        """A builtin resolves to its builtins path."""
        self.assertEqual(get_full_class_name("example"), "builtins.str")

    def test_get_full_class_name(self):
        """A user class resolves to its module path."""
        self.assertEqual(get_full_class_name(Dummy()), f"{_MODULE}.Dummy")

    def test_is_instance_of(self):
        """Instances and subclasses match; unknown names do not."""
        self.assertTrue(is_instance_of(Dummy(), f"{_MODULE}.Dummy"))
        self.assertTrue(is_instance_of(Dummy1(), f"{_MODULE}.Dummy"))

        self.assertFalse(is_instance_of(Dummy(), "foo.Dummy"))
        self.assertFalse(is_instance_of(Dummy(), f"{_MODULE}.Dummy2"))
        self.assertFalse(is_instance_of(1, f"{_MODULE}.Dummy"))

        with self.assertRaises(ValueError):
            is_instance_of(Dummy(), "Dummy")
