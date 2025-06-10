import os
from unittest.mock import patch
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.utils.env_utils import is_local


class LocalTest(EnvTestCase):
    def setUp(self):
        super().setUp()

    def tearDown(self):
        super().tearDown()

    @patch.dict(os.environ, {"_LOCAL_RUN": "1"})
    def test_is_local_true_in_local_run(self):
        self.assertTrue(is_local())

    @patch.dict(os.environ, {"UF_TASK_IMAGE": "image"})
    def test_is_local_false_with_task_image(self):
        self.assertFalse(is_local())

    @patch.dict(os.environ, {})
    def test_is_local_true(self):
        self.assertTrue(is_local())
