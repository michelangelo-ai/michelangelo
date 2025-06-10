import os
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode


class AuthTest(EnvTestCase):
    def setUp(self):
        super().setUp()

    def tearDown(self):
        super().tearDown()

    def test_get_terrablob_auth_mode(self):
        self.assertIsNone(get_terrablob_auth_mode())
