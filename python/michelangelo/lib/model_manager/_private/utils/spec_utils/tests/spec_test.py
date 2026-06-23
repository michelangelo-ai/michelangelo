"""Tests for nested object-specification utilities."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.spec_utils.spec import (
    collect_nested_class_paths,
)


class CollectNestedClassPathsTest(TestCase):
    """Test cases for ``collect_nested_class_paths``."""

    def test_flat_dict_without_target(self):
        """A flat dict with no ``_target_`` yields no class paths."""
        spec = {"lr": 0.1, "epochs": 10}

        self.assertEqual(collect_nested_class_paths(spec), set())

    def test_flat_dict_with_target(self):
        """A flat dict with a ``_target_`` yields that single class path."""
        spec = {"_target_": "my.module.Model", "lr": 0.1}

        self.assertEqual(collect_nested_class_paths(spec), {"my.module.Model"})

    def test_nested_target(self):
        """Targets nested inside argument values are collected recursively."""
        spec = {
            "_target_": "my.module.Model",
            "optimizer": {"_target_": "my.module.Adam", "lr": 0.1},
        }

        self.assertEqual(
            collect_nested_class_paths(spec),
            {"my.module.Model", "my.module.Adam"},
        )

    def test_list_with_targets(self):
        """Targets inside a list value are collected."""
        spec = {
            "layers": [
                {"_target_": "my.module.LayerA"},
                {"_target_": "my.module.LayerB"},
            ]
        }

        self.assertEqual(
            collect_nested_class_paths(spec),
            {"my.module.LayerA", "my.module.LayerB"},
        )

    def test_empty_dict(self):
        """An empty dict yields no class paths."""
        self.assertEqual(collect_nested_class_paths({}), set())

    def test_scalar_value(self):
        """A scalar value yields no class paths."""
        self.assertEqual(collect_nested_class_paths("just a string"), set())
