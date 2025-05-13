import os
from uber.ai.michelangelo.shared.testing.env import EnvTestCase
from michelangelo.lib.model_manager._private.utils.env_utils import is_local


class LocalTest(EnvTestCase):
    def setUp(self):
        super().setUp()

    def tearDown(self):
        super().tearDown()

    def test_get_terrablob_auth_mode(self):
        os.environ["UBER_LDAP_UID"] = "test"
        os.environ["UBER_OWNER"] = "test@uber.com"
        os.environ["UF_TASK_IMAGE"] = "image"
        self.assertFalse(is_local())

        del os.environ["UF_TASK_IMAGE"]
        self.assertTrue(is_local())

        del os.environ["UBER_LDAP_UID"]
        self.assertFalse(is_local())

        del os.environ["UBER_OWNER"]
        self.assertFalse(is_local())

        os.environ["UBER_LDAP_UID"] = "test"
        self.assertFalse(is_local())
