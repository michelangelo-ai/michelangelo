import os
from michelangelo._internal.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode


class AuthTest(EnvTestCase):
    def setUp(self):
        super().setUp()

    def tearDown(self):
        super().tearDown()

    def test_get_terrablob_auth_mode(self):
        os.environ["UBER_LDAP_UID"] = "test"
        os.environ["UBER_OWNER"] = "test@uber.com"
        if "UF_TASK_IMAGE" in os.environ:
            del os.environ["UF_TASK_IMAGE"]
        self.assertIsNone(get_terrablob_auth_mode())

        os.environ["UBER_LDAP_UID"] = ""
        os.environ["UBER_OWNER"] = "test@uber.com"
        self.assertIsNone(get_terrablob_auth_mode())

    def test_get_terrablob_auth_mode_legacy(self):
        os.environ["UBER_LDAP_UID"] = "test"
        os.environ["UBER_OWNER"] = "test@uber.com"
        os.environ["UF_TASK_IMAGE"] = "test"
        self.assertEqual(get_terrablob_auth_mode(), "legacy")
