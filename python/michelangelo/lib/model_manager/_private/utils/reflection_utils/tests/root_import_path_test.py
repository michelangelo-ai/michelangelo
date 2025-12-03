from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.reflection_utils import (
    get_root_import_path,
)


class RootImportPathTest(TestCase):
    """Tests utilities that locate the root import path."""

    def test_get_root_import_path(self):
        """It returns a path when no file override is provided."""
        root_path = get_root_import_path()
        self.assertIsNotNone(root_path)

    def test_get_root_import_path_given_file(self):
        """It returns a path when a module file is supplied."""
        root_path = get_root_import_path(
            "michelangelo/lib/model_manager/_private/utils/reflection_utils/root_import_path.py"
        )
        self.assertIsNotNone(root_path)
