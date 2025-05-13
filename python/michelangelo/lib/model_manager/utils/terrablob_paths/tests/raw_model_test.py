from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager.utils.terrablob_paths import (
    get_raw_model_base_path,
    get_raw_model_path,
    get_raw_model_main_path,
)


class RawModelTest(TestCase):
    def test_get_raw_model_base_path(self):
        path = get_raw_model_base_path(
            "test_project",
            "test_model",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model",
        )

    def test_get_raw_model_path(self):
        path = get_raw_model_path(
            "test_project",
            "test_model",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0",
        )

    def test_get_raw_model_path_with_revision(self):
        path = get_raw_model_path(
            "test_project",
            "test_model",
            "1",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1",
        )

    def test_get_raw_model_main_path(self):
        path = get_raw_model_main_path(
            "test_project",
            "test_model",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/0/main",
        )

    def test_get_raw_model_main_path_with_revision(self):
        path = get_raw_model_main_path(
            "test_project",
            "test_model",
            "1",
        )
        self.assertEqual(
            path,
            "/prod/michelangelo/raw_models/projects/test_project/models/test_model/revisions/1/main",
        )
