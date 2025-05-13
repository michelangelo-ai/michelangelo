from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.reflection_utils import get_root_import_path


class RootImportPathTest(TestCase):
    def test_get_root_import_path(self):
        root_path = get_root_import_path()
        self.assertIsNotNone(root_path)

    def test_get_root_import_path_given_file(self):
        root_path = get_root_import_path("uber/ai/michelangelo/sdk/model_manager/_private/utils/reflection_utils/root_import_path.py")
        self.assertIsNotNone(root_path)
